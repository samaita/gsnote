package jobs

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpenUpgradesPrototypeSchemaPreservingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE notes (id TEXT PRIMARY KEY, telegram_chat_id TEXT NOT NULL, telegram_message_id TEXT NOT NULL, telegram_file_id TEXT NOT NULL, audio_path TEXT NOT NULL, transcript_path TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, transcription_started_at TEXT, transcription_finished_at TEXT, error_message TEXT);
	INSERT INTO notes VALUES ('job-1','1','2','file','audio.ogg','old.md','QUEUED','2026-10-04T10:00:00Z',NULL,NULL,NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	job, err := r.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != Queued || job.TranscriptPath != "old.md" || job.AudioPath != "audio.ogg" {
		t.Fatalf("legacy row changed: %+v", job)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer r.Close()
	var attempts int
	var due sql.NullString
	if err := r.db.QueryRow(`SELECT attempts,next_attempt_at FROM notes WHERE id='job-1'`).Scan(&attempts, &due); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || due.Valid {
		t.Fatalf("retry columns = %d, %v", attempts, due)
	}
}

func TestInsertNewQueuedJobAllowsEmptyTranscriptPath(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	created := time.Now().UTC()
	job := Job{ID: "VN-queue", AudioPath: "voice.ogg", CreatedAt: created}
	if err := r.Insert(job); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Queued || got.TranscriptPath != "" || got.AudioPath != job.AudioPath {
		t.Fatalf("inserted job = %+v", got)
	}
}

func TestClaimOldestOnlyClaimsDueQueuedJob(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, j := range []Job{{ID: "future", AudioPath: "future.ogg", Status: Queued, CreatedAt: now.Add(-time.Hour)}, {ID: "ready", AudioPath: "ready.ogg", Status: Queued, CreatedAt: now}} {
		if err := r.Insert(j); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE notes SET next_attempt_at=? WHERE id='future'`, now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	got, err := r.ClaimOldest(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "ready" {
		t.Fatalf("claimed %+v", got)
	}
	got, err = r.ClaimOldest(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("claimed not-yet-due job %+v", got)
	}
}

func TestConcurrentClaimsNeverReturnSameJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	a, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	now := time.Now().UTC()
	if e = a.Insert(Job{ID: "one", AudioPath: "one.ogg", CreatedAt: now}); e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	result := make(chan *Job, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, r := range []*Repository{a, b} {
		wg.Add(1)
		go func(r *Repository) {
			defer wg.Done()
			<-start
			j, e := r.ClaimOldest(context.Background(), now.Add(time.Second))
			if e != nil {
				errs <- e
				return
			}
			result <- j
		}(r)
	}
	close(start)
	wg.Wait()
	close(result)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	n := 0
	for j := range result {
		if j != nil {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("claimed %d times", n)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, e := Open(""); e == nil {
		t.Fatal("expected error")
	}
}

func TestRetryDelay(t *testing.T) {
	cases := map[int]time.Duration{1: 30 * time.Second, 2: 60 * time.Second, 5: 480 * time.Second, 6: 960 * time.Second, 7: 1920 * time.Second, 8: time.Hour, 20: time.Hour}
	for n, w := range cases {
		if g := RetryDelay(n); g != w {
			t.Errorf("RetryDelay(%d)=%s want %s", n, g, w)
		}
	}
}

func TestFailRetriesThenMarksTerminal(t *testing.T) {
	r, e := Open(filepath.Join(t.TempDir(), "retry.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if e = r.Insert(Job{ID: "retry", AudioPath: "retry.ogg", CreatedAt: now}); e != nil {
		t.Fatal(e)
	}
	for n := 1; n <= 5; n++ {
		j, e := r.ClaimOldest(ctx, now)
		if e != nil {
			t.Fatal(e)
		}
		if j == nil {
			t.Fatalf("attempt %d not claimed", n)
		}
		terminal, e := r.Fail(ctx, "retry", "failure", now, 5)
		if e != nil {
			t.Fatal(e)
		}
		state, e := r.Get(ctx, "retry")
		if e != nil {
			t.Fatal(e)
		}
		if state.Attempts != n {
			t.Fatalf("attempts=%d want %d", state.Attempts, n)
		}
		if n < 5 {
			if terminal || state.Status != Queued || state.NextAttemptAt == nil || !state.NextAttemptAt.Equal(now.Add(RetryDelay(n))) {
				t.Fatalf("retry state %+v terminal=%v", state, terminal)
			}
			if _, err := r.db.Exec(`UPDATE notes SET next_attempt_at = ? WHERE id = ?`, now.Format(time.RFC3339Nano), "retry"); err != nil {
				t.Fatal(err)
			}
		} else if !terminal || state.Status != Failed || state.NextAttemptAt != nil {
			t.Fatalf("terminal state %+v", state)
		}
	}
}

func TestRecoverStaleDoesNotConsumeAttempt(t *testing.T) {
	r, e := Open(filepath.Join(t.TempDir(), "stale.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	if e = r.Insert(Job{ID: "stale", AudioPath: "stale.ogg", CreatedAt: now}); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ClaimOldest(ctx, now); e != nil {
		t.Fatal(e)
	}
	n, e := r.RecoverStale(ctx, now.Add(time.Second), now.Add(2*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if n != 1 {
		t.Fatalf("recovered %d", n)
	}
	j, e := r.Get(ctx, "stale")
	if e != nil {
		t.Fatal(e)
	}
	if j.Status != Queued || j.Attempts != 0 || j.NextAttemptAt != nil {
		t.Fatalf("state %+v", j)
	}
}
