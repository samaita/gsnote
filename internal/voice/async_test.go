package voice

import (
	"context"
	"errors"
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

func (n *asyncNotifier) send(_ int64, _ int, text string) error { n.calls++; n.text = text; return nil }

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
	fetch := func(*tgbotapi.Message) (string, string, error) {
		f, e := os.CreateTemp(root, "upload-*.ogg")
		if e != nil {
			return "", "", e
		}
		if _, e = f.WriteString("ogg"); e != nil {
			f.Close()
			return "", "", e
		}
		f.Close()
		return f.Name(), ".ogg", nil
	}
	acks := 0
	proc.SetDependencies(fetch, func(_ *tgbotapi.Message, s string) {
		if s != "Queued and saved VN-00001" {
			t.Errorf("ack %q", s)
		}
		acks++
	})
	msg := &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 42}, Voice: &tgbotapi.Voice{FileID: "file"}}
	proc.ProcessVoiceMessage(msg)
	if acks != 1 {
		t.Fatalf("acks=%d", acks)
	}
	queued, e := repo.List(context.Background())
	if e != nil || len(queued) != 1 {
		t.Fatalf("queued=%v err=%v", queued, e)
	}
	job := queued[0]
	if job.Status != jobs.Queued || job.TranscriptPath != "" || job.FileID != "file" || job.ChatID != "42" {
		t.Fatalf("queued row %+v", job)
	}
	if _, e := os.Stat(job.AudioPath); e != nil {
		t.Fatal(e)
	}
	trans := &asyncFakeTranscriber{text: "English transcript"}
	notify := &asyncNotifier{}
	w := Worker{Repo: repo, Transcriber: trans, Root: root, Notify: notify.send}
	claimed, e := repo.ClaimOldest(context.Background(), time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	w.process(context.Background(), claimed)
	if trans.calls != 1 {
		t.Fatalf("STT calls=%d", trans.calls)
	}
	done, e := repo.Get(context.Background(), job.ID)
	if e != nil {
		t.Fatal(e)
	}
	if done.Status != jobs.Done || done.TranscriptPath == "" {
		t.Fatalf("done=%+v", done)
	}
	body, e := os.ReadFile(done.TranscriptPath)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(body), "English transcript") {
		t.Fatalf("note=%s", body)
	}
	if notify.calls != 1 {
		t.Fatalf("notifications=%d", notify.calls)
	}
}

type fakeNotifier struct {
	calls int
	texts []string
}

func (n *fakeNotifier) send(_ int64, _ int, text string) error {
	n.calls++
	n.texts = append(n.texts, text)
	return nil
}
func TestFailureRetryAndTerminalNotification(t *testing.T) {
	root := t.TempDir()
	repo, e := jobs.Open(filepath.Join(root, "queue.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	audio := filepath.Join(root, "v.ogg")
	if e = os.WriteFile(audio, []byte("ogg"), 0600); e != nil {
		t.Fatal(e)
	}
	created := time.Now().UTC()
	if e = repo.Insert(jobs.Job{ID: "VN-retry", AudioPath: audio, CreatedAt: created}); e != nil {
		t.Fatal(e)
	}
	notice := &fakeNotifier{}
	tr := &asyncFakeTranscriber{err: errors.New("offline")}
	w := Worker{Repo: repo, Root: root, Transcriber: tr, Notify: notice.send, MaxAttempts: 1}
	job, e := repo.ClaimOldest(context.Background(), created.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	w.process(context.Background(), job)
	state, e := repo.Get(context.Background(), job.ID)
	if e != nil {
		t.Fatal(e)
	}
	if state.Status != jobs.Failed || state.Attempts != 1 || notice.calls != 1 {
		t.Fatalf("state=%+v notifications=%d", state, notice.calls)
	}
	if _, e = os.Stat(audio); e != nil {
		t.Fatalf("original audio lost: %v", e)
	}
}
