package voice

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/jobs"
	"github.com/axonigma/gsnote/internal/storage"
	"github.com/axonigma/gsnote/internal/transcription"
)

type AudioFetcher func(*tgbotapi.Message) (string, string, error)
type ReplyFunc func(*tgbotapi.Message, string)
type AsyncProcessor struct {
	bot   *tgbotapi.BotAPI
	root  string
	store *storage.Storage
	repo  *jobs.Repository
	fetch AudioFetcher
	reply ReplyFunc
	mu    sync.Mutex
	seen  map[int]struct{}
}

func NewAsyncProcessor(bot *tgbotapi.BotAPI, root string, repo *jobs.Repository) (*AsyncProcessor, error) {
	store, e := storage.New(root)
	if e != nil {
		return nil, e
	}
	p := &AsyncProcessor{bot: bot, root: root, store: store, repo: repo, reply: sendReply(bot), seen: map[int]struct{}{}}
	p.fetch = p.downloadVoice
	return p, nil
}

// SetDependencies replaces network and reply behavior for tests or alternate transports.
func (p *AsyncProcessor) SetDependencies(fetch AudioFetcher, reply ReplyFunc) {
	if fetch != nil {
		p.fetch = fetch
	}
	if reply != nil {
		p.reply = reply
	}
}
func (p *AsyncProcessor) ProcessVoiceMessage(m *tgbotapi.Message) {
	if m == nil || m.Chat == nil || m.Voice == nil {
		return
	}
	p.mu.Lock()
	if _, ok := p.seen[m.MessageID]; ok {
		p.mu.Unlock()
		return
	}
	p.seen[m.MessageID] = struct{}{}
	p.mu.Unlock()
	voiceID, e := NewIDManager(p.root).Next()
	if e != nil {
		p.reply(m, "Could not allocate voice ID; please retry.")
		return
	}
	id := "VN-" + voiceID
	tmp, ext, e := p.fetch(m)
	if e != nil {
		p.reply(m, "Could not download voice; please retry.")
		return
	}
	defer os.Remove(tmp)
	if ext == "" {
		ext = ".ogg"
	}
	now := time.Now().UTC()
	name := fmt.Sprintf("%s-%s%s", id, now.Format("20060102150405"), ext)
	f, e := os.Open(tmp)
	if e != nil {
		p.reply(m, "Could not read downloaded voice.")
		return
	}
	defer f.Close()
	if e = p.store.SaveAudio(name, f); e != nil {
		p.reply(m, "Could not save voice recording.")
		return
	}
	idPath := filepath.Join(p.root, "Inbox", "Voices", name)
	if e = syncVoiceIndex(p.root, id, idPath); e != nil {
		p.reply(m, fmt.Sprintf("Voice saved as %s at %s, but could not update audio index.", id, idPath))
		return
	}
	audio := idPath
	job := jobs.Job{ID: id, ChatID: strconv.FormatInt(m.Chat.ID, 10), MessageID: strconv.Itoa(m.MessageID), FileID: voiceFileID(m), AudioPath: audio, Status: jobs.Queued, CreatedAt: now}
	if e = p.repo.Insert(job); e != nil {
		p.reply(m, fmt.Sprintf("Voice saved as %s at %s, but queueing failed; recording was kept.", id, audio))
		return
	}
	p.reply(m, fmt.Sprintf("Queued and saved %s", id))
}
func syncVoiceIndex(root, id, audioPath string) error {
	index := filepath.Join(root, "Inbox", "Texts", id+".md")
	f, err := os.OpenFile(index, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	fileName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	if _, err = fmt.Fprintf(f, "# %s\n\n- Audio: [[%s]]\n- Status: QUEUED\n", id, fileName); err != nil {
		f.Close()
		os.Remove(index)
		return err
	}
	return f.Close()
}

func voiceFileID(m *tgbotapi.Message) string {
	if m.Voice != nil {
		return m.Voice.FileID
	}
	return ""
}
func (p *AsyncProcessor) downloadVoice(m *tgbotapi.Message) (string, string, error) {
	if p.bot == nil || m.Voice == nil {
		return "", "", fmt.Errorf("voice download unavailable")
	}
	f, e := p.bot.GetFile(tgbotapi.FileConfig{FileID: m.Voice.FileID})
	if e != nil {
		return "", "", e
	}
	resp, e := http.Get(f.Link(p.bot.Token))
	if e != nil {
		return "", "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("download status %d", resp.StatusCode)
	}
	tmp, e := os.CreateTemp("", "gsnote-voice-*.ogg")
	if e != nil {
		return "", "", e
	}
	if _, e = io.Copy(tmp, resp.Body); e != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", e
	}
	if e = tmp.Close(); e != nil {
		os.Remove(tmp.Name())
		return "", "", e
	}
	return tmp.Name(), ".ogg", nil
}
func sendReply(bot *tgbotapi.BotAPI) ReplyFunc {
	return func(m *tgbotapi.Message, text string) {
		if bot == nil || m == nil || m.Chat == nil {
			return
		}
		reply := tgbotapi.NewMessage(m.Chat.ID, text)
		reply.ReplyToMessageID = m.MessageID
		if _, e := bot.Send(reply); e != nil {
			log.Printf("voice reply: %v", e)
		}
	}
}

type Worker struct {
	Repo        *jobs.Repository
	Transcriber transcription.Transcriber
	Notify      func(int64, int, string) error
	Poll        time.Duration
	MaxAttempts int
	Root        string
	StaleAfter  time.Duration
}

func (w *Worker) Run(ctx context.Context) error {
	if w.Poll <= 0 {
		w.Poll = time.Second
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = jobs.DefaultMaxAttempts
	}
	if w.StaleAfter <= 0 {
		w.StaleAfter = 5 * time.Minute
	}
	if _, e := w.Repo.RecoverStale(ctx, time.Now().UTC().Add(-w.StaleAfter), time.Now().UTC()); e != nil {
		return e
	}
	for {
		j, e := w.Repo.ClaimOldest(ctx, time.Now().UTC())
		if e != nil {
			return e
		}
		if j != nil {
			w.process(ctx, j)
			continue
		}
		timer := time.NewTimer(w.Poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func (w *Worker) process(ctx context.Context, j *jobs.Job) {
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = jobs.DefaultMaxAttempts
	}
	text, e := w.Transcriber.Transcribe(j.AudioPath)
	if e != nil {
		w.fail(ctx, j, e)
		return
	}
	dir := filepath.Join(w.Root, "Inbox", "Texts")
	if e = os.MkdirAll(dir, 0755); e != nil {
		w.fail(ctx, j, e)
		return
	}
	name, title, body := jobs.TranscriptMarkdown(j.ID, filepath.Join("Inbox", "Voices", filepath.Base(j.AudioPath)), j.CreatedAt, text)
	path := filepath.Join(dir, j.CreatedAt.Format("2006-01-02")+" - "+name+".md")
	if old, readErr := os.ReadFile(path); readErr == nil {
		if string(old) != body {
			w.fail(ctx, j, fmt.Errorf("transcript collision: %s", path))
			return
		}
	} else if !os.IsNotExist(readErr) {
		w.fail(ctx, j, readErr)
		return
	} else {
		savedPath, saveErr := jobs.SaveTranscript(dir, j.CreatedAt, name, j.ID, body)
		if saveErr != nil {
			w.fail(ctx, j, saveErr)
			return
		}
		path = savedPath
	}
	if e = w.Repo.Complete(ctx, j.ID, path); e != nil {
		log.Printf("complete %s: %v", j.ID, e)
		return
	}
	w.notify(j, fmt.Sprintf("Transcribed %s: %s", j.ID, title))
}
func (w *Worker) fail(ctx context.Context, j *jobs.Job, e error) {
	terminal, err := w.Repo.Fail(ctx, j.ID, e.Error(), time.Now().UTC(), w.MaxAttempts)
	if err != nil {
		log.Printf("job failure: %v", err)
		return
	}
	if terminal {
		w.notify(j, "Transcription failed for "+j.ID)
	}
}
func (w *Worker) notify(j *jobs.Job, text string) {
	if w.Notify == nil {
		return
	}
	chat, _ := strconv.ParseInt(j.ChatID, 10, 64)
	msg, _ := strconv.Atoi(j.MessageID)
	if e := w.Notify(chat, msg, text); e != nil {
		log.Printf("notify %s: %v", j.ID, e)
	}
}
