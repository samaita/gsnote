package voice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeTranscriber struct {
	fn func(audioPath string) (string, error)
}

func (f fakeTranscriber) Transcribe(audioPath string) (string, error) {
	return f.fn(audioPath)
}

type fakeLLM struct {
	fn func(transcript string) (*VoiceInfo, error)
}

func (f fakeLLM) Process(transcript string) (*VoiceInfo, error) {
	return f.fn(transcript)
}

func newTestProcessor(t *testing.T, voicesRoot string, tr Transcriber, llm NoteLLM) (*Processor, *[]string) {
	t.Helper()
	var sent []string
	p := &Processor{
		transcriber: tr,
		llm:         llm,
		idMgr:       NewIDManager(voicesRoot),
		voicesRoot:  voicesRoot,
		fetchAudio: func(msg *tgbotapi.Message) (string, string, error) {
			f, err := os.CreateTemp(t.TempDir(), "gsnote-test-voice-*.ogg")
			if err != nil {
				return "", "", err
			}
			f.Close()
			return f.Name(), ".ogg", nil
		},
		send: func(msg *tgbotapi.Message, text string) {
			sent = append(sent, text)
		},
		lastMsgSeq: make(map[int64]bool),
	}
	return p, &sent
}

func voiceMessage(id int) *tgbotapi.Message {
	return &tgbotapi.Message{
		MessageID: id,
		Chat:      &tgbotapi.Chat{ID: 123},
		From:      &tgbotapi.User{ID: 456},
		Voice:     &tgbotapi.Voice{FileID: "file_id_voice"},
	}
}

func validInfo() *VoiceInfo {
	return &VoiceInfo{
		Title:    "Address Quality sebagai API Audit",
		Summary:  "Address Quality could be positioned as an API audit product for ecommerce systems.",
		Content:  "Address Quality could be positioned as an API audit product for ecommerce systems.",
		Type:     "idea",
		Category: "address-quality",
		Project:  "address-quality",
		Tags:     []string{"address-quality", "api", "business"},
	}
}

func listAudioFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var audio []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ogg") {
			audio = append(audio, e.Name())
		}
	}
	return audio
}

func TestProcessVoiceMessageSuccess(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "Gue kepikiran Address Quality sebenarnya bisa dijual sebagai API audit buat ecommerce", nil
	}}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		if transcript == "" {
			t.Fatal("expected non-empty transcript to reach the LLM")
		}
		return validInfo(), nil
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 {
		t.Fatalf("expected one reply, got %d: %v", len(*sent), *sent)
	}
	if got := (*sent)[0]; got != "Saved 00001\n\nAddress Quality sebagai API Audit" {
		t.Fatalf("unexpected reply: %q", got)
	}

	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 {
		t.Fatalf("expected one audio file, got %v", audio)
	}
	if !strings.HasPrefix(audio[0], "00001-") {
		t.Fatalf("audio filename must start with voice ID: %q", audio[0])
	}

	dateStr := time.Now().In(time.Local).Format("20060102")
	mdPath := filepath.Join(voicesRoot, "00001-"+dateStr+".md")
	data, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown: %v", err)
	}
	md := string(data)
	for _, want := range []string{
		`id: "00001"`,
		"source: telegram-voice",
		"audio: " + audio[0],
		"# Address Quality sebagai API Audit",
		"## Summary",
		"## Transcript",
		"Gue kepikiran Address Quality",
		"      - api",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestProcessVoiceMessageDownloadFailure(t *testing.T) {
	voicesRoot := t.TempDir()
	p, sent := newTestProcessor(t, voicesRoot, fakeTranscriber{}, fakeLLM{})
	p.fetchAudio = func(msg *tgbotapi.Message) (string, string, error) {
		return "", "", errors.New("download boom")
	}

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || (*sent)[0] != "Failed to download audio." {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	entries, err := os.ReadDir(voicesRoot)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files written on download failure, got %v", entries)
	}
}

func TestProcessVoiceMessageSTTFailureKeepsAudio(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "", errors.New("stt boom")
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, fakeLLM{})

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "STT failed") {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 || !strings.HasPrefix(audio[0], "00001-") {
		t.Fatalf("audio must survive STT failure, got %v", audio)
	}
	if _, err := os.Stat(filepath.Join(voicesRoot, "00001-"+time.Now().In(time.Local).Format("20060102")+".md")); !os.IsNotExist(err) {
		t.Fatalf("markdown must not exist after STT failure")
	}
}

func TestProcessVoiceMessageLLMFailureKeepsAudio(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "some transcript", nil
	}}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		return nil, errors.New("llm boom")
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "LLM failed") {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 || !strings.HasPrefix(audio[0], "00001-") {
		t.Fatalf("audio must survive LLM failure, got %v", audio)
	}
}

func TestProcessVoiceMessageInvalidLLMResultKeepsAudio(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "some transcript", nil
	}}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		info := validInfo()
		info.Title = ""
		return info, nil
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "invalid result") {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 {
		t.Fatalf("audio must be kept on invalid LLM result, got %v", audio)
	}
}

func TestProcessVoiceMessageMarkdownFailureKeepsAudio(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "some transcript", nil
	}}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		return validInfo(), nil
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)

	dateStr := time.Now().In(time.Local).Format("20060102")
	mdPath := filepath.Join(voicesRoot, "00001-"+dateStr+".md")
	if err := os.MkdirAll(mdPath, 0755); err != nil {
		t.Fatalf("mkdir blocking md path: %v", err)
	}

	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "but the note file could not be written") {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 {
		t.Fatalf("audio must be kept on markdown failure, got %v", audio)
	}
}

func TestProcessVoiceMessageIdempotent(t *testing.T) {
	voicesRoot := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) {
		return "transcript", nil
	}}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		return validInfo(), nil
	}}
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)

	msg := voiceMessage(42)
	p.ProcessVoiceMessage(msg)
	p.ProcessVoiceMessage(msg)

	if len(*sent) != 1 {
		t.Fatalf("expected a single reply for a duplicate update, got %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 {
		t.Fatalf("duplicate update must not create a duplicate capture, got %v", audio)
	}
	mdCount := 0
	entries, _ := os.ReadDir(voicesRoot)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			mdCount++
		}
	}
	if mdCount != 1 {
		t.Fatalf("expected one markdown file, got %d", mdCount)
	}
}

func TestDeleteByIDExistingDeletesAudioAndMarkdown(t *testing.T) {
	root := t.TempDir()
	audio := filepath.Join(root, "00001-20260823092015.ogg")
	md := filepath.Join(root, "00001-20260823.md")
	other := filepath.Join(root, "00002-20260823.md")
	for _, path := range []string{audio, md, other} {
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	out, err := DeleteByID(root, "00001")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if out != "Deleted voice 00001." {
		t.Fatalf("unexpected output: %q", out)
	}
	for _, path := range []string{audio, md} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s deleted", path)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("other capture must be untouched: %v", err)
	}
}

func TestDeleteByIDNonexistent(t *testing.T) {
	root := t.TempDir()
	out, err := DeleteByID(root, "00001")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if out != "Voice 00001 not found." {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDeleteByIDInvalidIDs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "00001-20260823.ogg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	invalid := []string{
		"",
		"abc",
		"1234",
		"123456",
		"00001/../../etc/passwd",
		"../",
		"/",
		`\`,
		"../00001",
		"/etc/passwd",
		"00001 x",
		"00001;rm",
	}
	for _, id := range invalid {
		if _, err := DeleteByID(root, id); !errors.Is(err, ErrInvalidVoiceID) {
			t.Errorf("id %q: expected ErrInvalidVoiceID, got %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "00001-20260823.ogg")); err != nil {
		t.Fatalf("file must not be deleted by invalid IDs: %v", err)
	}
}

func TestDeleteByIDDoesNotTouchCounter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "_counter.txt"), []byte("5"), 0644); err != nil {
		t.Fatalf("write counter: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "00001-20260823.ogg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	DeleteByID(root, "00001")

	if _, err := os.Stat(filepath.Join(root, "_counter.txt")); err != nil {
		t.Fatalf("counter file must be preserved: %v", err)
	}
}

func TestDeleteByIDAmbiguousDoesNotDelete(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"00001-20260823092015.ogg",
		"00001-20260823.md",
		"00001-extra.bak",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	out, err := DeleteByID(root, "00001")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if out != "Unable to delete voice 00001: multiple matching files found." {
		t.Fatalf("unexpected output: %q", out)
	}
	for _, name := range []string{"00001-20260823092015.ogg", "00001-20260823.md", "00001-extra.bak"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("ambiguous delete must not remove %s: %v", name, err)
		}
	}
}

func TestDeleteByIDDoesNotMatchLongerID(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "000010-20260823.ogg"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if out, err := DeleteByID(root, "00001"); err != nil || out != "Voice 00001 not found." {
		t.Fatalf("unexpected result: out=%q err=%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(root, "000010-20260823.ogg")); err != nil {
		t.Fatalf("longer ID must not be matched: %v", err)
	}
}

func TestValidVoiceID(t *testing.T) {
	for id, want := range map[string]bool{
		"00001": true,
		"00002": true,
		"99999": true,
		"":      false,
		"1":     false,
		"0000":  false,
		"000000": false,
		"abcde":  false,
		"00001/": false,
		"../":    false,
	} {
		if got := ValidVoiceID(id); got != want {
			t.Errorf("ValidVoiceID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIDManagerSequential(t *testing.T) {
	root := t.TempDir()
	m := NewIDManager(root)
	for i, want := range []string{"00001", "00002", "00003"} {
		got, err := m.Next()
		if err != nil {
			t.Fatalf("next %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("next %d = %q, want %q", i, got, want)
		}
	}
}

func TestIDManagerPersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	m1 := NewIDManager(root)
	if id, _ := m1.Next(); id != "00001" {
		t.Fatalf("first id: %q", id)
	}
	m2 := NewIDManager(root)
	if id, _ := m2.Next(); id != "00002" {
		t.Fatalf("second id must persist across instances, got %q", id)
	}
}

func TestWriteMarkdownIncludesAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "00001-20260823.md")
	err := WriteMarkdown(path, VoiceMetadata{
		ID:         "00001",
		Date:       time.Date(2026, 8, 23, 10, 0, 0, 0, time.UTC),
		Title:      "Title",
		Summary:    "Summary",
		Transcript: "Transcript",
		VoiceType:  "idea",
		Category:   "cat",
		Project:    "proj",
		Tags:       []string{"a", "b"},
		Audio:      "00001-20260823-xxxx.ogg",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	md := string(data)
	for _, want := range []string{"audio: 00001-20260823-xxxx.ogg", `id: "00001"`} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
}

func TestProcessorDeleteAndList(t *testing.T) {
	root := t.TempDir()
	tr := fakeTranscriber{fn: func(path string) (string, error) { return "t", nil }}
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) { return validInfo(), nil }}
	p, _ := newTestProcessor(t, root, tr, llm)

	p.ProcessVoiceMessage(voiceMessage(1))

	list, err := p.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(list, "00001") {
		t.Fatalf("list missing 00001: %q", list)
	}

	out, err := p.Delete("00001")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if out != "Deleted voice 00001." {
		t.Fatalf("unexpected delete output: %q", out)
	}
	if len(listAudioFiles(t, root)) != 0 {
		t.Fatalf("audio files must be gone after delete")
	}
	if _, err := os.Stat(filepath.Join(root, fmt.Sprintf("00001-%s.md", time.Now().In(time.Local).Format("20060102")))); !os.IsNotExist(err) {
		t.Fatalf("markdown must be gone after delete")
	}
}
