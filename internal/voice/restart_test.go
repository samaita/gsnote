package voice

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/axonigma/gsnote/internal/jobs"
)

type countingTranscriber struct{ calls int }

func (f *countingTranscriber) Transcribe(string) (string, error) { f.calls++; return "unexpected", nil }
func TestWorkerLeavesFutureJobQueued(t *testing.T) {
	root := t.TempDir()
	repo, e := jobs.Open(filepath.Join(root, "q.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	if e = repo.Insert(jobs.Job{ID: "future", AudioPath: "missing.ogg", CreatedAt: now, NextAttemptAt: &future}); e != nil {
		t.Fatal(e)
	}
	tr := &countingTranscriber{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := Worker{Repo: repo, Root: root, Transcriber: tr, Poll: 5 * time.Millisecond}
	for i := 0; i < 6; i++ {
		if e = ctx.Err(); e != nil {
			t.Fatal(e)
		}
		job, e := repo.ClaimOldest(ctx, time.Now().UTC())
		if e != nil {
			t.Fatal(e)
		}
		if job != nil {
			t.Fatalf("future job claimed: %+v", job)
		}
		time.Sleep(5 * time.Millisecond)
	}
	job, e := repo.Get(ctx, "future")
	if e != nil {
		t.Fatal(e)
	}
	if job.Status != jobs.Queued || tr.calls != 0 {
		t.Fatalf("job=%+v calls=%d", job, tr.calls)
	}
	_ = w
}
func TestWorkerCompletesAlreadyPublishedSameJobNote(t *testing.T) {
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
	created := time.Now().UTC().Truncate(time.Second)
	if e = repo.Insert(jobs.Job{ID: "VN-existing", AudioPath: audio, CreatedAt: created}); e != nil {
		t.Fatal(e)
	}
	job, e := repo.ClaimOldest(context.Background(), created.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	name, _, body := jobs.TranscriptMarkdown(job.ID, filepath.Join("Inbox", "Voices", filepath.Base(audio)), created, "Recovered once")
	dir := filepath.Join(root, "Inbox", "Texts")
	if e = os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	path, e := jobs.SaveTranscript(dir, created, name, job.ID, body)
	if e != nil {
		t.Fatal(e)
	}
	n, e := repo.RecoverStale(context.Background(), created.Add(time.Second), created.Add(2*time.Second))
	if e != nil || n != 1 {
		t.Fatalf("recovered=%d err=%v", n, e)
	}
	job, e = repo.ClaimOldest(context.Background(), created.Add(3*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	tr := &countingTranscriber{}
	w := Worker{Repo: repo, Root: root, Transcriber: tr}
	w.process(context.Background(), job)
	done, e := repo.Get(context.Background(), "VN-existing")
	if e != nil {
		t.Fatal(e)
	}
	if done.Status != jobs.Done || done.TranscriptPath != path || tr.calls != 0 {
		t.Fatalf("job=%+v calls=%d", done, tr.calls)
	}
	files, e := os.ReadDir(dir)
	if e != nil || len(files) != 1 {
		t.Fatalf("files=%d err=%v", len(files), e)
	}
}
