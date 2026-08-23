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

// Processor orchestrates the full voice-to-note pipeline.
type Processor struct {
	bot         *tgbotapi.BotAPI
	transcriber *Transcriber
	llm         *LLM
	idMgr       *IDManager
	voicesRoot  string
	syncRoot    string
	lastMsgSeq  map[int64]bool // messageID -> processed
}

// NewProcessor creates a new Processor instance.
func NewProcessor(bot *tgbotapi.BotAPI, sttKey, sttBaseURL, sttModel, llmKey, llmBaseURL, llmModel string, voicesRoot, syncRoot string) *Processor {
	return &Processor{
		bot:         bot,
		transcriber: &Transcriber{APIKey: sttKey, BaseURL: sttBaseURL, Model: sttModel},
		llm:         &LLM{APIKey: llmKey, BaseURL: llmBaseURL, Model: llmModel},
		idMgr:       NewIDManager(voicesRoot),
		voicesRoot:  voicesRoot,
		syncRoot:    syncRoot,
		lastMsgSeq:  make(map[int64]bool),
	}
}

// ProcessVoiceMessage handles the full voice processing pipeline.
func (p *Processor) ProcessVoiceMessage(msg *tgbotapi.Message) {
	if msg == nil {
		return
	}

	// Dedup by message ID (Telegram can retry updates)
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

	voicePath, ext, err := p.downloadVoice(msg)
	if err != nil {
		log.Printf("voice download error: %v", err)
		p.reply(msg, "Failed to download audio.")
		return
	}
	defer os.Remove(voicePath)

	voiceID, err := p.idMgr.Next()
	if err != nil {
		log.Printf("voice ID error: %v", err)
		p.reply(msg, "Failed to generate ID.")
		return
	}

	date := time.Now().In(time.Local)

	audioFilename := fmt.Sprintf("%s-%s%s", voiceID, date.Format("20060102150405"), ext)
	audioPath := filepath.Join(p.voicesRoot, audioFilename)
	if err := os.Rename(voicePath, audioPath); err != nil {
		log.Printf("voice rename error: %v", err)
		p.reply(msg, "Failed to save audio file.")
		return
	}

	rawTranscript, err := p.transcriber.Transcribe(audioPath)
	if err != nil {
		log.Printf("voice STT error: %v", err)
		p.reply(msg, "Voice received. STT failed — audio saved for retry.")
		return
	}

	info, err := p.llm.Process(rawTranscript)
	if err != nil {
		log.Printf("voice LLM error: %v", err)
		p.reply(msg, "Voice received. LLM failed — audio saved for retry.")
		return
	}

	meta := VoiceMetadata{
		ID:         voiceID,
		Date:       date,
		Title:      info.Title,
		Summary:    info.Summary,
		Content:    info.Content,
		Transcript: rawTranscript,
		VoiceType:  info.Type,
		Category:   info.Category,
		Project:    info.Project,
		Tags:       info.Tags,
	}

	mdFilename := DefaultMDFilename(voiceID, date)
	mdPath := filepath.Join(p.voicesRoot, mdFilename)
	if err := WriteMarkdown(mdPath, meta); err != nil {
		log.Printf("voice markdown write error: %v", err)
		p.reply(msg, fmt.Sprintf("Saved %s\n%v", voiceID, info.Title))
		return
	}

	p.reply(msg, fmt.Sprintf("Saved %s\n\n%s", voiceID, info.Title))
}

// ListVoices returns the list of voice captures in the directory.
func ListVoices(voicesRoot string) (string, error) {
	entries, err := os.ReadDir(voicesRoot)
	if err != nil {
		return "", fmt.Errorf("read voices dir: %w", err)
	}

	var voiceNames []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if idx := findDashIdx(name); idx > 0 {
			voiceNames = append(voiceNames, name[:idx])
		}
	}
	if len(voiceNames) == 0 {
		return "No voice captures found.", nil
	}

	result := "Recent voice captures:\n"
	start := len(voiceNames) - 10
	if start < 0 {
		start = 0
	}
	for i := len(voiceNames) - 1; i >= start; i-- {
		result += "  " + voiceNames[i] + "\n"
	}
	return result[:len(result)-1], nil
}

// downloadVoice fetches audio from Telegram and saves it to a temp file.
func (p *Processor) downloadVoice(msg *tgbotapi.Message) (string, string, error) {
	var remoteID string
	ext := ".ogg"

	if msg.Audio != nil {
		remoteID = msg.Audio.FileID
		ext = ".ogg"
	} else if msg.Document != nil && msg.Document.MimeType == "audio/ogg" {
		remoteID = msg.Document.FileID
		ext = ".ogg"
	} else {
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

func (p *Processor) reply(msg *tgbotapi.Message, text string) {
	if msg == nil || p.bot == nil {
		return
	}
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := p.bot.Send(reply); err != nil {
		log.Printf("send voice reply error: %v", err)
	}
}

func findDashIdx(s string) int {
	for i, c := range s {
		if c == '-' {
			return i
		}
	}
	return -1
}
