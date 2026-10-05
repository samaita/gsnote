package voice

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/axonigma/gsnote/internal/jobs"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type asyncFakeTranscriber struct {
	calls int
	text  string
	err   error
}

func (f *asyncFakeTranscriber) Transcribe(string) (string, error) { f.calls++; return f.text, f.err }

type asyncNotifier struct {
	calls int
	text  string
}

func (n *asyncNotifier) send(_ int64, _ int, s string) error { n.calls++; n.text = s; return nil }
func TestEnqueueDoesNotTranscribeWorkerWritesNote(t *testing.T) {
	root := t.TempDir()
	repo, e := jobs.Open(filepath.Join(root, "queue.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	proc, e := NewAsyncProcessor(nil, root, repo)
	if e != nil {
		t.Fatal(e)
	}
	fetch := func(*tgbotapi.Message) (io.ReadCloser, string, error) {
		return io.NopCloser(strings.NewReader("ogg")), "file", nil
	}
	acks := 0
	proc.SetDependencies(fetch, func(_ *tgbotapi.Message, s string) {
		if s != "Queued and saved VN-file" {
			t.Errorf("ack %q", s)
		}
		acks++
	})
	msg := &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 42}, Voice: &tgbotapi.Voice{FileID: "file"}}
	proc.ProcessVoiceMessage(msg)
	if acks != 1 {
		t.Fatalf("acks=%d", acks)
	}
	rows, e := repo.List(context.Background())
	if e != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, e)
	}
	job := rows[0]
	if job.Status != jobs.Queued || job.TranscriptPath != "" || job.FileID != "file" || job.ChatID != "42" {
		t.Fatalf("job=%+v", job)
	}
	if _, e = os.Stat(job.AudioPath); e != nil {
		t.Fatal(e)
	}
	tr := &asyncFakeTranscriber{text: "English transcript"}
	note := &asyncNotifier{}
	w := Worker{Repo: repo, Root: root, Transcriber: tr, Notify: note.send}
	claim, e := repo.ClaimOldest(context.Background(), time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	w.process(context.Background(), claim)
	done, e := repo.Get(context.Background(), job.ID)
	if e != nil {
		t.Fatal(e)
	}
	name, _, body := jobs.TranscriptMarkdown(job.ID, filepath.Join("Inbox", "Voices", filepath.Base(job.AudioPath)), done.CreatedAt, "English transcript")
	want := filepath.Join(root, "Inbox", "Texts", done.CreatedAt.Format("2006-01-02")+" - "+name+".md")
	if done.Status != jobs.Done || done.TranscriptPath != want {
		t.Fatalf("job=%+v want=%s", done, want)
	}
	content, e := os.ReadFile(want)
	if e != nil || !strings.Contains(string(content), body) {
		t.Fatalf("note=%s err=%v", content, e)
	}
	if tr.calls != 1 || note.calls != 1 {
		t.Fatalf("STT=%d notify=%d", tr.calls, note.calls)
	}
}

type fakeNotifier struct {
	calls int
	texts []string
}

func (n *fakeNotifier) send(_ int64, _ int, s string) error {
	n.calls++
	n.texts = append(n.texts, s)
	return nil
}
func TestWorkerTerminalFailureNotice(t *testing.T) {
	root := t.TempDir()
	repo, e := jobs.Open(filepath.Join(root, "q.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	audio := filepath.Join(root, "voice.ogg")
	if e = os.WriteFile(audio, []byte("ogg"), 0600); e != nil {
		t.Fatal(e)
	}
	created := time.Now().UTC()
	if e = repo.Insert(jobs.Job{ID: "VN-fail", AudioPath: audio, CreatedAt: created}); e != nil {
		t.Fatal(e)
	}
	notify := &fakeNotifier{}
	w := Worker{Repo: repo, Root: root, Transcriber: &asyncFakeTranscriber{err: errors.New("offline")}, Notify: notify.send, MaxAttempts: 1}
	job, e := repo.ClaimOldest(context.Background(), created.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	w.process(context.Background(), job)
	state, e := repo.Get(context.Background(), job.ID)
	if e != nil {
		t.Fatal(e)
	}
	if state.Status != jobs.Failed || state.Attempts != 1 || notify.calls != 1 {
		t.Fatalf("state=%+v notify=%d", state, notify.calls)
	}
	if _, e = os.Stat(audio); e != nil {
		t.Fatalf("OGG removed: %v", e)
	}
}
