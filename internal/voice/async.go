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
)

type AudioFetcher func(*tgbotapi.Message) (io.ReadCloser, string, error)
type ReplyFunc func(*tgbotapi.Message, string)

type AsyncProcessor struct {
	bot   *tgbotapi.BotAPI
	root  string
	store *storage.Storage
	repo  *jobs.Repository
	mu    sync.Mutex
	seen  map[int]struct{}
	fetch AudioFetcher
	reply ReplyFunc
}

func NewAsyncProcessor(bot *tgbotapi.BotAPI, root string, repo *jobs.Repository) (*AsyncProcessor, error) {
	store, e := storage.New(root)
	if e != nil {
		return nil, e
	}
	p := &AsyncProcessor{bot: bot, root: root, store: store, repo: repo, seen: map[int]struct{}{}, reply: sendReply(bot)}
	if bot != nil {
		p.fetch = p.downloadVoice
	}
	return p, nil
}
func (p *AsyncProcessor) SetDependencies(f AudioFetcher, r ReplyFunc) {
	if f != nil {
		p.fetch = f
	}
	if r != nil {
		p.reply = r
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
	id := fmt.Sprintf("VN-%05d", m.MessageID)

	name := fmt.Sprintf("%s.ogg", id)
	f, actual, e := p.fetch(m)
	if e != nil {
		p.reply(m, "Could not download voice recording.")
		return
	}
	defer f.Close()
	if e = p.store.SaveAudio(name, f); e != nil {
		p.reply(m, "Could not save voice recording.")
		return
	}
	audio := filepath.Join(p.root, "Inbox", "Voices", name)
	id = "VN-" + actual
	if e = syncVoiceIndex(p.root, id, filepath.Join("Inbox", "Voices", name)); e != nil {
		p.reply(m, fmt.Sprintf("Voice saved as %s at %s, but Inbox index creation failed.", id, audio))
		return
	}
	created := m.Time()
	job := jobs.Job{ID: id, ChatID: strconv.FormatInt(m.Chat.ID, 10), MessageID: strconv.Itoa(m.MessageID), FileID: voiceFileID(m), AudioPath: audio, CreatedAt: created}
	if e = p.repo.Insert(job); e != nil {
		p.reply(m, fmt.Sprintf("Voice saved as %s at %s, but queueing failed; recording was kept.", id, audio))
		return
	}
	p.reply(m, fmt.Sprintf("Queued and saved %s", id))
}
func sendReply(bot *tgbotapi.BotAPI) ReplyFunc {
	return func(m *tgbotapi.Message, text string) {
		if bot == nil || m == nil || m.Chat == nil {
			return
		}
		r := tgbotapi.NewMessage(m.Chat.ID, text)
		r.ReplyToMessageID = m.MessageID
		if _, e := bot.Send(r); e != nil {
			log.Printf("voice reply: %v", e)
		}
	}
}
func (p *AsyncProcessor) downloadVoice(m *tgbotapi.Message) (io.ReadCloser, string, error) {
	f, e := p.bot.GetFile(tgbotapi.FileConfig{FileID: m.Voice.FileID})
	if e != nil {
		return nil, "", e
	}
	u := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", p.bot.Token, f.FilePath)
	res, e := http.Get(u)
	if e != nil {
		return nil, "", e
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, "", fmt.Errorf("telegram file download: %s", res.Status)
	}
	return res.Body, m.Voice.FileID, nil
}
func syncVoiceIndex(root, id, audioPath string) error {
	index := filepath.Join(root, "Inbox", "Texts", id+".md")
	f, e := os.OpenFile(index, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	name := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	if _, e = fmt.Fprintf(f, "# %s\n\n- Audio: [[%s]]\n- Status: QUEUED\n", id, name); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}
func voiceFileID(m *tgbotapi.Message) string {
	if m.Voice != nil {
		return m.Voice.FileID
	}
	return ""
}

type Worker struct {
	Repo             *jobs.Repository
	Transcriber      interface{ Transcribe(string) (string, error) }
	Root             string
	Notify           func(int64, int, string) error
	Poll, StaleAfter time.Duration
	MaxAttempts      int
}

func (w *Worker) Run(ctx context.Context) error {
	if w.Poll <= 0 {
		w.Poll = time.Second
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
	dir := filepath.Join(w.Root, "Inbox", "Texts")
	if e := os.MkdirAll(dir, 0755); e != nil {
		w.fail(ctx, j, e)
		return
	}
	name, title, body := jobs.TranscriptMarkdown(j.ID, filepath.Join("Inbox", "Voices", filepath.Base(j.AudioPath)), j.CreatedAt, "")
	expected := filepath.Join(dir, j.CreatedAt.Format("2006-01-02")+" - "+name+".md")
	var published string
	files, _ := os.ReadDir(dir)
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".md" {
			continue
		}
		candidate := filepath.Join(dir, file.Name())
		if old, err := os.ReadFile(candidate); err == nil && strings.Contains(string(old), "Source: gsnote voice "+j.ID) {
			published = candidate
			break
		}
	}
	if published != "" {
		if e := w.Repo.Complete(ctx, j.ID, published); e != nil {
			log.Printf("complete %s: %v", j.ID, e)
			return
		}
		w.notify(j, fmt.Sprintf("Transcribed %s", j.ID))
		return
	}
	text, e := w.Transcriber.Transcribe(j.AudioPath)
	if e != nil {
		w.fail(ctx, j, e)
		return
	}
	name, title, body = jobs.TranscriptMarkdown(j.ID, filepath.Join("Inbox", "Voices", filepath.Base(j.AudioPath)), j.CreatedAt, text)
	path, e := jobs.SaveTranscript(dir, j.CreatedAt, name, j.ID, body)
	if e != nil {
		old, re := os.ReadFile(expected)
		if re != nil || !strings.Contains(string(old), "Source: gsnote voice "+j.ID) {
			w.fail(ctx, j, e)
			return
		}
		path = expected
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
