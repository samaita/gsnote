package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sttTimeout = 5 * time.Minute

// Transcriber converts audio files to text.
type Transcriber interface {
	Transcribe(audioPath string) (string, error)
}

// ElevenTranscriber calls the ElevenLabs speech-to-text API. Telegram
// OGG/Opus voice notes are uploaded as-is: no local conversion is needed.
type ElevenTranscriber struct {
	APIKey   string // ELEVEN_API_KEY (required)
	BaseURL  string // default "https://api.elevenlabs.io"
	Model    string // default "scribe_v1"
	Language string // ISO-639-1 (e.g. "id", "en"); empty = auto-detect
	Client   *http.Client
}

// Transcribe uploads the audio file to ElevenLabs and returns the transcript.
func (t *ElevenTranscriber) Transcribe(audioPath string) (string, error) {
	if strings.TrimSpace(t.APIKey) == "" {
		return "", fmt.Errorf("ELEVEN_API_KEY is not configured")
	}
	base := t.BaseURL
	if base == "" {
		base = "https://api.elevenlabs.io"
	}
	model := t.Model
	if model == "" {
		model = "scribe_v1"
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: sttTimeout}
	}

	f, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("open audio: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("model_id", model); err != nil {
		return "", fmt.Errorf("write model_id: %w", err)
	}
	if t.Language != "" {
		if err := mw.WriteField("language_code", t.Language); err != nil {
			return "", fmt.Errorf("write language_code: %w", err)
		}
	}
	if err := mw.WriteField("diarize", "false"); err != nil {
		return "", fmt.Errorf("write diarize: %w", err)
	}
	if err := mw.WriteField("tag_audio_events", "false"); err != nil {
		return "", fmt.Errorf("write tag_audio_events: %w", err)
	}
	fw, err := mw.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", fmt.Errorf("create file field: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", fmt.Errorf("copy audio: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, base+"/v1/speech-to-text", &body)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("xi-api-key", t.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("elevenlabs request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("elevenlabs status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		LanguageCode string `json:"language_code"`
		Text         string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}
