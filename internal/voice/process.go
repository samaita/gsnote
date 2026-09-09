package voice

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Processor orchestrates the voice capture pipeline: download the voice note,
// persist the raw audio, transcribe it, and write the transcript note.
type Processor struct {
	bot         *tgbotapi.BotAPI
	transcriber Transcriber
	idMgr       *IDManager
	root        string
	fetchAudio  func(msg *tgbotapi.Message) (tmpPath, ext string, err error)
	send        func(msg *tgbotapi.Message, text string)
	lastMsgSeq  map[int64]bool // messageID -> processed
}

// NewProcessor creates a new Processor instance. root is the single gsnote
// folder holding audio files, transcript notes, and the ID counter.
func NewProcessor(bot *tgbotapi.BotAPI, elevenKey, elevenModel, elevenLang, root string) *Processor {
	p := &Processor{
		bot: bot,
		transcriber: &ElevenTranscriber{
			APIKey:   elevenKey,
			Model:    elevenModel,
			Language: elevenLang,
		},
		idMgr:      NewIDManager(root),
		root:       root,
		lastMsgSeq: make(map[int64]bool),
	}
	p.fetchAudio = p.downloadVoice
	p.send = p.sendToChat
	return p
}

// ProcessVoiceMessage handles the full voice capture pipeline.
func (p *Processor) ProcessVoiceMessage(msg *tgbotapi.Message) {
	if msg == nil {
		return
	}

	// Dedup by message ID (Telegram can redeliver updates)
	msgID := int64(msg.MessageID)
	if p.lastMsgSeq[msgID] {
		log.Printf("voice: duplicate msg %d, skipping", msgID)
		return
	}
	p.lastMsgSeq[msgID] = true
	const maxDedup = 1000
	if len(p.lastMsgSeq) > maxDedup {
		i := 0
		for k := range p.lastMsgSeq {
			if i >= maxDedup/2 {
				delete(p.lastMsgSeq, k)
			}
			i++
		}
	}

	voicePath, ext, err := p.fetchAudio(msg)
	if err != nil {
		log.Printf("voice download error: %v", err)
		p.send(msg, "Failed to download audio.")
		return
	}
	defer os.Remove(voicePath)

	voiceID, err := p.idMgr.Next()
	if err != nil {
		log.Printf("voice ID error: %v", err)
		p.send(msg, "Failed to generate ID.")
		return
	}

	// Persist the original audio before any STT work so a later failure
	// never destroys the recording.
	date := time.Now()

	audioFilename := fmt.Sprintf("%s-%s%s", voiceID, date.Format("20060102150405"), ext)
	audioPath := filepath.Join(p.root, audioFilename)
	if err := copyFile(voicePath, audioPath); err != nil {
		log.Printf("voice save error: %v", err)
		p.send(msg, "Failed to save audio file.")
		return
	}

	transcript, err := p.transcriber.Transcribe(audioPath)
	if err != nil {
		log.Printf("voice STT error: %v", err)
		p.send(msg, "Voice received. STT failed — audio saved for retry.")
		return
	}

	meta := VoiceMetadata{
		ID:         voiceID,
		Date:       date,
		Transcript: transcript,
		Audio:      audioFilename,
	}
	mdFilename := DefaultMDFilename(voiceID, date)
	mdPath := filepath.Join(p.root, mdFilename)
	if err := WriteMarkdown(mdPath, meta); err != nil {
		log.Printf("voice markdown write error: %v", err)
		p.send(msg, fmt.Sprintf("Voice received. Audio saved as %s, but the note file could not be written.", voiceID))
		return
	}

	p.send(msg, fmt.Sprintf("Saved %s\n\n%s", voiceID, truncate(transcript, 200)))
}

// downloadVoice fetches audio from Telegram and saves it to a temp file.
func (p *Processor) downloadVoice(msg *tgbotapi.Message) (string, string, error) {
	var remoteID string
	ext := ".ogg"

	switch {
	case msg.Voice != nil:
		remoteID = msg.Voice.FileID
	case msg.Audio != nil:
		remoteID = msg.Audio.FileID
	case msg.Document != nil && msg.Document.MimeType == "audio/ogg":
		remoteID = msg.Document.FileID
	default:
		return "", "", fmt.Errorf("no audio found in message")
	}

	cfg := tgbotapi.FileConfig{FileID: remoteID}
	file, err := p.bot.GetFile(cfg)
	if err != nil {
		return "", "", fmt.Errorf("get file: %w", err)
	}

	url := file.Link(p.bot.Token)

	resp, err := http.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("download http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "gsnote-voice-*"+ext)
	if err != nil {
		return "", "", fmt.Errorf("create temp: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", "", fmt.Errorf("download copy: %w", err)
	}
	tmpFile.Close()

	return tmpFile.Name(), ext, nil
}

func (p *Processor) sendToChat(msg *tgbotapi.Message, text string) {
	if msg == nil || p.bot == nil {
		return
	}
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := p.bot.Send(reply); err != nil {
		log.Printf("send voice reply error: %v", err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
