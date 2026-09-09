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
	text string
	err  error
}

func (f *fakeTranscriber) Transcribe(audioPath string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func newTestProcessor(t *testing.T, root string, tr Transcriber) (*Processor, *[]string) {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	p := &Processor{
		bot:         &tgbotapi.BotAPI{},
		transcriber: tr,
		idMgr:       NewIDManager(root),
		root:        root,
		lastMsgSeq:  make(map[int64]bool),
	}
	sent := []string{}
	p.send = func(msg *tgbotapi.Message, text string) {
		sent = append(sent, text)
	}
	p.fetchAudio = func(msg *tgbotapi.Message) (string, string, error) {
		tmp := filepath.Join(t.TempDir(), "voice.ogg")
		if err := os.WriteFile(tmp, []byte("OGGAUDIO"), 0644); err != nil {
			return "", "", err
		}
		return tmp, ".ogg", nil
	}
	return p, &sent
}

func TestProcessVoiceMessageSuccess(t *testing.T) {
	root := t.TempDir()
	p, sent := newTestProcessor(t, root, &fakeTranscriber{text: "beli kopi dulu sebelum ngoding"})
	msg := &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: 1}}

	p.ProcessVoiceMessage(msg)

	if len(*sent) != 1 {
		t.Fatalf("sent = %v, want 1 message", *sent)
	}
	if !strings.Contains((*sent)[0], "00001") {
		t.Errorf("reply %q missing voice ID", (*sent)[0])
	}
	if !strings.Contains((*sent)[0], "beli kopi") {
		t.Errorf("reply %q missing transcript snippet", (*sent)[0])
	}

	audio := filepath.Join(root, "00001-20060102150405.ogg")
	if _, err := os.Stat(audio); err != nil {
		// Timestamp differs; glob instead.
		matches, _ := filepath.Glob(filepath.Join(root, "00001-*.ogg"))
		if len(matches) != 1 {
			t.Fatalf("expected 1 audio file for 00001, got %v", matches)
		}
	}

	matches, _ := filepath.Glob(filepath.Join(root, "00001-*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 note file for 00001, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if !strings.Contains(string(data), "beli kopi dulu sebelum ngoding") {
		t.Errorf("note missing transcript:\n%s", data)
	}
}

func TestProcessVoiceMessageDownloadFailure(t *testing.T) {
	root := t.TempDir()
	p, sent := newTestProcessor(t, root, &fakeTranscriber{text: "x"})
	p.fetchAudio = func(msg *tgbotapi.Message) (string, string, error) {
		return "", "", errors.New("boom")
	}
	p.ProcessVoiceMessage(&tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 1}})
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Failed to download audio") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestProcessVoiceMessageSTTFailureKeepsAudio(t *testing.T) {
	root := t.TempDir()
	p, sent := newTestProcessor(t, root, &fakeTranscriber{err: errors.New("eleven down")})
	p.ProcessVoiceMessage(&tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 1}})

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "audio saved for retry") {
		t.Fatalf("sent = %v", *sent)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "00001-*.ogg"))
	if len(matches) != 1 {
		t.Fatalf("STT failed but audio must persist, got %v", matches)
	}
	mds, _ := filepath.Glob(filepath.Join(root, "*.md"))
	if len(mds) != 0 {
		t.Fatalf("no note expected on STT failure, got %v", mds)
	}
}

func TestProcessVoiceMessageMarkdownFailureKeepsAudio(t *testing.T) {
	root := t.TempDir()
	// The note filename is <id>-YYYYMMDD.md (day precision), so a directory
	// with that exact name deterministically blocks the write.
	blocker := filepath.Join(root, fmt.Sprintf("00001-%s.md", time.Now().Format("20060102")))
	if err := os.MkdirAll(blocker, 0755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}
	p, sent := newTestProcessor(t, root, &fakeTranscriber{text: "hello"})
	p.ProcessVoiceMessage(&tgbotapi.Message{MessageID: 1, Chat: &tgbotapi.Chat{ID: 1}})

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "could not be written") {
		t.Fatalf("sent = %v", *sent)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "00001-*.ogg"))
	if len(matches) != 1 {
		t.Fatalf("expected audio persisted despite md failure, got %v", matches)
	}
}

func TestProcessVoiceMessageIdempotent(t *testing.T) {
	root := t.TempDir()
	p, sent := newTestProcessor(t, root, &fakeTranscriber{text: "hello"})
	msg := &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 1}}

	p.ProcessVoiceMessage(msg)
	p.ProcessVoiceMessage(msg)

	if len(*sent) != 1 {
		t.Fatalf("duplicate delivery must be skipped, sent = %v", *sent)
	}
}

func TestIDManagerSequential(t *testing.T) {
	m := NewIDManager(t.TempDir())
	first, err := m.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	second, err := m.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if first != "00001" || second != "00002" {
		t.Errorf("ids = %q, %q; want 00001, 00002", first, second)
	}
}

func TestIDManagerPersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	m1 := NewIDManager(root)
	if _, err := m1.Next(); err != nil {
		t.Fatalf("next: %v", err)
	}
	m2 := NewIDManager(root)
	id, err := m2.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if id != "00002" {
		t.Errorf("id = %q, want 00002 (counter persisted)", id)
	}
}

func TestWriteMarkdownIncludesAudioAndTranscript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "00003-20260909.md")
	meta := VoiceMetadata{
		ID:         "00003",
		Date:       dateAt("2026-09-09 14:32"),
		Transcript: "catatan pengingat besok demo",
		Audio:      "00003-20260909143200.ogg",
	}
	if err := WriteMarkdown(path, meta); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(data)
	for _, want := range []string{
		`id: "00003"`,
		"date: 2026-09-09 14:32",
		"source: telegram-voice",
		"audio: 00003-20260909143200.ogg",
		"catatan pengingat besok demo",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("note missing %q:\n%s", want, s)
		}
	}
}

func TestDefaultMDFilename(t *testing.T) {
	got := DefaultMDFilename("00001", dateAt("2026-09-09 14:32"))
	if got != "00001-20260909.md" {
		t.Errorf("DefaultMDFilename = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10s", 10, "exactly10s"},
		{"this is way too long", 10, "this is wa…"},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.n); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.ogg")
	dst := filepath.Join(dir, "dst.ogg")
	if err := os.WriteFile(src, []byte("ABCD"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "ABCD" {
		t.Errorf("copy mismatch: %q", data)
	}
}

func dateAt(s string) time.Time {
	parsed, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return parsed
}
