package voice

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/axonigma/gsnote/internal/jobs"
	"github.com/axonigma/gsnote/internal/storage"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type AsyncProcessor struct {
	bot        *tgbotapi.BotAPI
	repo       *jobs.Repository
	store      *storage.Storage
	fetchAudio func(*tgbotapi.Message) (string, string, error)
	send       func(*tgbotapi.Message, string)
}

func NewAsyncProcessor(bot *tgbotapi.BotAPI, dataDir string) (*AsyncProcessor, error) {
	repo, e := jobs.Open(filepath.Join(dataDir, "gsnote.db"))
	if e != nil {
		return nil, e
	}
	st, e := storage.New(dataDir)
	if e != nil {
		repo.Close()
		return nil, e
	}
	p := &AsyncProcessor{bot: bot, repo: repo, store: st}
	p.fetchAudio = p.downloadVoice
	p.send = p.sendToChat
	return p, nil
}
func (p *AsyncProcessor) Repository() *jobs.Repository { return p.repo }
func (p *AsyncProcessor) ProcessVoiceMessage(msg *tgbotapi.Message) {
	if msg == nil {
		return
	}
	tmp, ext, e := p.fetchAudio(msg)
	if e != nil {
		p.send(msg, "❌ Could not save voice note.")
		return
	}
	defer os.Remove(tmp)
	id := newNoteID(time.Now())
	audio, e := p.store.SaveAudio(tmp, id, ext)
	if e != nil {
		p.send(msg, "❌ Could not save voice note.")
		return
	}
	note := jobs.Note{ID: id, ChatID: fmt.Sprint(msg.Chat.ID), MessageID: fmt.Sprint(msg.MessageID), FileID: msg.Voice.FileID, AudioPath: audio, TranscriptPath: p.store.TranscriptPath(id), Status: jobs.Queued, CreatedAt: time.Now()}
	if e = p.repo.Insert(note); e != nil {
		log.Printf("job_queued note_id=%s error=%v", id, e)
		p.send(msg, "❌ Could not queue voice note.")
		return
	}
	log.Printf("audio_saved note_id=%s", id)
	p.send(msg, fmt.Sprintf("🎙 Saved\n\n%s\nStatus: queued", id))
}
func newNoteID(t time.Time) string {
	b := make([]byte, 2)
	if _, e := rand.Read(b); e != nil {
		b = []byte{0, 0}
	}
	return fmt.Sprintf("VN-%s-%s", t.Format("20060102-150405"), hex.EncodeToString(b))
}
func (p *AsyncProcessor) downloadVoice(msg *tgbotapi.Message) (string, string, error) {
	f, e := p.bot.GetFile(tgbotapi.FileConfig{FileID: msg.Voice.FileID})
	if e != nil {
		return "", "", e
	}
	req, e := http.NewRequest(http.MethodGet, f.Link(p.bot.Token), nil)
	if e != nil {
		return "", "", e
	}
	client := p.bot.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, e := client.Do(req)
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
	if _, e = tmp.ReadFrom(resp.Body); e != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", "", e
	}
	tmp.Close()
	return tmp.Name(), ".ogg", nil
}
func (p *AsyncProcessor) sendToChat(msg *tgbotapi.Message, text string) {
	if p.bot == nil {
		return
	}
	_, e := p.bot.Send(tgbotapi.NewMessage(msg.Chat.ID, text))
	if e != nil {
		log.Printf("telegram_notification_failed: %v", e)
	}
}

func (p *AsyncProcessor) Ready(n *jobs.Note, path string) error {
	if p.bot == nil {
		return nil
	}
	if _, err := p.bot.Send(tgbotapi.NewMessage(parseID(n.ChatID), fmt.Sprintf("✅ Transcription ready\n\n%s", n.ID))); err != nil {
		return err
	}
	_, err := p.bot.Send(tgbotapi.NewDocument(parseID(n.ChatID), tgbotapi.FilePath(path)))
	return err
}

func (p *AsyncProcessor) Failed(n *jobs.Note) error {
	if p.bot == nil {
		return nil
	}
	_, err := p.bot.Send(tgbotapi.NewMessage(parseID(n.ChatID), fmt.Sprintf("❌ Transcription failed\n\n%s\nAudio is still safely stored.", n.ID)))
	return err
}

func parseID(s string) int64 { var n int64; fmt.Sscan(s, &n); return n }
