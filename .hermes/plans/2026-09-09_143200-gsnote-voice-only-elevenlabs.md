# gsnote Voice-Only Revamp — Milestone 1: Raw + Transcript (ElevenLabs STT)

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Pivot gsnote to a voice-only note app: Telegram voice note → ElevenLabs Scribe STT → save raw audio + transcript markdown. No LLM, no habits, no git sync, no command surface beyond `/help`.

**Architecture:** Single gsnote root folder holds everything (`<id>-<ts>.ogg` audio, `<id>-<ts>.md` transcript note, `_counter.txt` ID counter). One input path: voice messages. One command: `/help`. STT is a single HTTPS multipart call to ElevenLabs `POST /v1/speech-to-text` (model `scribe_v1`, language auto-detect by default). The `Transcriber` interface stays; `LocalTranscriber` (whisper.cpp + ffmpeg) is deleted.

**Tech Stack:** Go (stdlib `net/http`, `mime/multipart`, `encoding/json`), telegram-bot-api v5, godotenv. No new dependencies.

---

## Milestone definition (from user)

- Only 1 way to input: **voice**.
- Output per capture: **raw audio + transcript**. No summarizer/LLM for now.
- `.env` contains only: **1 gsnote folder root + Telegram-related + ElevenLabs-related** vars.
- GitHub token gone (was only for `/sync`). Single command: `/help`.

## Resulting env shape

```
TELEGRAM_BOT_TOKEN=<bot token>          # required
WHITELIST_TELEGRAM_ID=<telegram id>     # required (comma-separated allowed)
GSNOTE_ROOT=/path/to/notes              # required (single root for audio + md + counter)
ELEVEN_API_KEY=<xi-api-key>             # required for voice pipeline (bot boots without it)
ELEVEN_MODEL=scribe_v1                  # optional, default "scribe_v1"
ELEVEN_LANGUAGE=                        # optional, ISO-639-1 (e.g. "id", "en"); empty = auto-detect
```

Timestamps use system local time — no TIMEZONE var.

## ElevenLabs Scribe API (contract used by this plan)

```
POST https://api.elevenlabs.io/v1/speech-to-text
Headers:
  xi-api-key: <ELEVEN_API_KEY>
Body (multipart/form-data):
  file:        <audio bytes>          # OGG/Opus from Telegram accepted natively — no ffmpeg
  model_id:    scribe_v1
  language_code: <optional>            # omit => auto-detect
  diarize: "false"
  tag_audio_events: "false"
Response 200: {"language_code":"eng","text":"...","words":[...]}
```

We parse only `.text`. Errors: non-200 → surface status + body snippet. Limits: ~1h / 1GB per request — Telegram voice notes are far below this. Auto-detect covers mixed EN/ID speech; leave `ELEVEN_LANGUAGE` empty unless quality demands pinning.

## Files: create / modify / delete

**Delete entirely:**
- `internal/voice/llm.go`, `internal/voice/llm_test.go` (LLM processor)
- `internal/voice/delete.go` (`/voice delete` gone)
- `internal/syncgit/service.go`, `internal/syncgit/service_test.go` (git sync feature)
- `internal/writer/writer.go` (habit writer)
- `internal/parser/` — whole package (`parser.go` habit parser, `voice.go` + `voice_test.go` voice-command parser: no commands left to parse)
- `internal/handler/handler_test.go` — rewritten (see Task 5)

**Rewrite:**
- `internal/voice/stt.go` → `ElevenTranscriber`
- `internal/voice/stt_test.go` → httptest-based tests
- `internal/voice/markdown.go` → slim metadata + verbatim transcript note
- `internal/voice/process.go` → drop LLM + List/Delete; keep dedup, download, audio-first persistence
- `internal/voice/voice_test.go` → trim to new pipeline (most of the 558 lines test the LLM flow)
- `internal/handler/handler.go` → voice message + `/help` only
- `cmd/bot/main.go` → new env loading
- `.env.example`, `README.md`, `install.sh`

**Unchanged:**
- `internal/voice/id.go` (sequential `00001`-style IDs persisted in `_counter.txt`)
- `Makefile` (`make dev` stays)

---

## Task 1: `ElevenTranscriber` (TDD — happy path + language pin)

**Objective:** STT via ElevenLabs HTTP API, replacing whisper.cpp entirely.

**Files:**
- Rewrite: `internal/voice/stt.go`
- Rewrite: `internal/voice/stt_test.go`

**Step 1: Write failing tests** — `internal/voice/stt_test.go` (httptest server asserts `xi-api-key` header, `model_id` field, uploaded audio bytes; a second test pins `language_code` when `Language` is set):

```go
package voice

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newElevenServer(t *testing.T, handler http.HandlerFunc) *ElevenTranscriber {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &ElevenTranscriber{APIKey: "test-key", BaseURL: srv.URL, Model: "scribe_v1", Client: srv.Client()}
}

func TestElevenTranscriber_HappyPath(t *testing.T) {
	var gotAuth, gotModel string
	var gotFile []byte
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("xi-api-key")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		gotModel = r.FormValue("model_id")
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		gotFile, _ = io.ReadAll(f)
		json.NewEncoder(w).Encode(map[string]any{"language_code": "eng", "text": "hello world"})
	})

	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("FAKEOGG"), 0644)

	text, err := tr.Transcribe(tmp)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if gotAuth != "test-key" {
		t.Errorf("api key header = %q", gotAuth)
	}
	if gotModel != "scribe_v1" {
		t.Errorf("model_id = %q", gotModel)
	}
	if string(gotFile) != "FAKEOGG" {
		t.Errorf("uploaded audio mismatch")
	}
}

func TestElevenTranscriber_LanguageCode(t *testing.T) {
	var gotLang string
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotLang = r.FormValue("language_code")
		json.NewEncoder(w).Encode(map[string]any{"text": "halo"})
	})
	tr.Language = "id"
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	if _, err := tr.Transcribe(tmp); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if gotLang != "id" {
		t.Errorf("language_code = %q, want %q", gotLang, "id")
	}
}
```

**Step 2: Run** `go test ./internal/voice/ -run TestEleven -v` → FAIL (type not defined).

**Step 3: Implement** — new `internal/voice/stt.go`:

```go
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

// ElevenTranscriber calls the ElevenLabs speech-to-text API.
// Telegram OGG/Opus voice notes are uploaded as-is: no local conversion.
type ElevenTranscriber struct {
	APIKey   string // ELEVEN_API_KEY (required)
	BaseURL  string // default "https://api.elevenlabs.io"
	Model    string // default "scribe_v1"
	Language string // ISO-639-1; empty = auto-detect
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
```

**Step 4: Run** `go test ./internal/voice/ -run TestEleven -v` → PASS.
**Step 5: Commit:** `git add internal/voice/stt.go internal/voice/stt_test.go && git commit -m "feat: ElevenLabs Scribe transcriber replaces whisper.cpp"`

## Task 2: STT error paths (TDD)

**Objective:** Config + API failures return clear errors (never panic, never lose audio).

**Files:** Modify `internal/voice/stt_test.go`, `internal/voice/stt.go` (if needed).

Tests to add:

```go
func TestElevenTranscriber_MissingAPIKey(t *testing.T) {
	tr := &ElevenTranscriber{}
	_, err := tr.Transcribe(filepath.Join(t.TempDir(), "v.ogg"))
	if err == nil || !strings.Contains(err.Error(), "ELEVEN_API_KEY") {
		t.Fatalf("want missing-key error, got %v", err)
	}
}

func TestElevenTranscriber_APIError(t *testing.T) {
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":{"status":401,"message":"invalid_api_key"}}`))
	})
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	_, err := tr.Transcribe(tmp)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestElevenTranscriber_EmptyTranscript(t *testing.T) {
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"text": "   "})
	})
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	text, err := tr.Transcribe(tmp)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "" {
		t.Errorf("want trimmed empty, got %q", text)
	}
}
```

**Run:** `go test ./internal/voice/ -run TestEleven -v` → PASS. **Commit:** `test: ElevenLabs transcriber error paths`

## Task 3: Slim markdown format (TDD)

**Objective:** Note file = frontmatter (id/date/audio/source) + verbatim transcript. No title/summary/type/category/project/tags (LLM fields gone).

**Files:**
- Rewrite: `internal/voice/markdown.go`
- Modify affected tests in `internal/voice/voice_test.go`

**New `VoiceMetadata` + `WriteMarkdown`:**

```go
// VoiceMetadata is the data written to the transcript note.
type VoiceMetadata struct {
	ID         string
	Date       time.Time
	Transcript string
	Audio      string // audio filename relative to GSNOTE_ROOT
}

// WriteMarkdown writes the voice note as a markdown file.
func WriteMarkdown(path string, meta VoiceMetadata) error {
	content := fmt.Sprintf(`---
id: "%s"
date: %s
source: telegram-voice
audio: %s
---

%s
`, meta.ID, meta.Date.Format("2006-01-02 15:04"), meta.Audio, meta.Transcript)
	return os.WriteFile(path, []byte(content), 0644)
}
```

`DefaultMDFilename` stays `<id>-YYYYMMDD.md`. Delete `WriteRawTranscript` — the note IS the raw transcript now.

**Run:** `go test ./internal/voice/ -v`. **Commit:** `feat: slim voice note to raw transcript format`

## Task 4: Pipeline without LLM (TDD)

**Objective:** `process.go` flow becomes: dedup → download → save audio → transcribe (ElevenLabs) → write single note md → reply. No LLM, no validateVoiceInfo, no List/Delete surface.

**Files:**
- Modify: `internal/voice/process.go`
- Modify: `internal/voice/voice_test.go` (fake transcriber instead of LLM flow)
- Delete: `internal/voice/llm.go`, `internal/voice/llm_test.go`

**`NewProcessor` signature:**

```go
func NewProcessor(bot *tgbotapi.BotAPI, elevenKey, elevenModel, elevenLang, root string) *Processor
```

`ProcessVoiceMessage` body — keep dedup + `fetchAudio` + audio-persist-before-STT resilience exactly as today, then:

```go
transcript, err := p.transcriber.Transcribe(audioPath)
if err != nil { /* audio already saved */ p.send(msg, "Voice received. STT failed — audio saved for retry."); return }

meta := VoiceMetadata{ID: voiceID, Date: date, Transcript: transcript, Audio: audioFilename}
mdPath := filepath.Join(p.root, DefaultMDFilename(voiceID, date))
if err := WriteMarkdown(mdPath, meta); err != nil { ... }

p.send(msg, fmt.Sprintf("Saved %s\n\n%s", voiceID, truncate(transcript, 200)))
```

Add tiny helper `truncate(s, n)` so long transcripts don't blow the Telegram 4096-char reply limit. Remove `List`, `ListVoices`, `findDashIdx`. Rename `voicesRoot`/`syncRoot` fields → single `root`.

**Run:** `go test ./internal/voice/ -v` → PASS. **Commit:** `feat: voice pipeline saves raw audio + transcript, LLM step removed`

## Task 5: Handler minimal: voice + /help (TDD)

**Objective:** Handler routes voice messages and `/help` only. Unknown `/command` gets the existing warn text.

**Files:**
- Rewrite: `internal/handler/handler.go`, `internal/handler/handler_test.go`
- Delete: `internal/writer/writer.go`, `internal/parser/` (whole dir), `internal/syncgit/` (whole dir)

`Handler` struct: `bot`, `whitelistTelegramID`, `voiceSvc` (interface with just `ProcessVoiceMessage`). `New(bot *tgbotapi.BotAPI, whitelist map[int64]bool) *Handler`.

```go
const helpText = `gsnote — voice-only notes.

Send a voice message to capture a note (audio + transcript).

Commands:
  /help — this message`

const warnText = `Command not found, use /help for guide`
```

`Handle`: whitelist check → `msg.Voice != nil` → `voiceSvc.ProcessVoiceMessage` → `text == "/help"` → helpText → other `/...` → warnText.

**Run:** `go test ./internal/handler/ -v`. **Commit:** `refactor: handler is voice + /help only; drop habit/sync/voice commands`

## Task 6: `main.go` + env overhaul

**Files:**
- Rewrite config section: `cmd/bot/main.go`
- Rewrite: `.env.example`

`main.go` required vars: `TELEGRAM_BOT_TOKEN`, `GSNOTE_ROOT`, `WHITELIST_TELEGRAM_ID`. `ELEVEN_API_KEY`/`ELEVEN_MODEL`/`ELEVEN_LANGUAGE` read with defaults (`scribe_v1`, empty = auto). No TIMEZONE — system local time. MkdirAll `GSNOTE_ROOT`. Voice processor: `if elevenKey != "" { h.StartVoiceProcessor(voice.NewProcessor(bot, elevenKey, elevenModel, elevenLang, root)) }`.

**Run:** `go build ./... && go vet ./...` → clean. **Commit:** `feat: env reduced to gsnote root + telegram + elevenlabs`

## Task 7: Docs + installer

**Files:**
- Rewrite: `README.md` — voice-only story: send voice → audio + transcript in `GSNOTE_ROOT`; env table (6 vars); zero system deps; single command `/help`.
- Modify: `install.sh` — prompts: bot token, whitelist ID, gsnote root, ElevenLabs API key. Drop habits/github/git-author/timezone prompts. systemd part unchanged.

**Commit:** `docs: voice-only revamp with ElevenLabs STT`

## Task 8: Full verification + smoke run

**Step 1:** `go build ./... && go vet ./... && go test ./...` → all green. `gofmt -l .` → empty.
**Step 2:** Smoke run (needs user creds): `~/.config/gsnote/.env` with real token + ElevenLabs key + scratch `GSNOTE_ROOT`, `make dev`, send a voice note from Telegram, confirm `00001-*.ogg` + `00001-*.md` appear with transcript.
**Step 3:** Commit any fixes, open PR `feat/voice-note` → `main`.

---

## Risks / open questions

1. **ElevenLabs language coverage:** auto-detect covers mixed EN/ID speech. If quality is poor, pin `ELEVEN_LANGUAGE=id` or `en`. (Note in README.)
2. **Cost/limits:** Scribe bills per audio minute; Telegram voice notes are seconds — negligible.
3. **File layout migration:** `GSNOTE_ROOT` may point at the old `Voices/` folder — old notes coexist fine; IDs continue from `_counter.txt`. No migration code.
4. **whisper-cli/ffmpeg** fully unused — removed from requirements entirely.
5. **Dropping List/Delete** means the only way to remove a bad capture is deleting files by hand in the folder — acceptable for this milestone by design (user decision: single command `/help`).

## Verification summary

- `go test ./...` green after each task; full suite at the end.
- Smoke: real Telegram voice note → two files in `GSNOTE_ROOT`, transcript readable.
- `.env.example` matches loaded vars exactly (no dead keys).
