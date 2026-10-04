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
	_, err = db.Exec(`CREATE TABLE notes (
		id TEXT PRIMARY KEY, telegram_chat_id TEXT NOT NULL, telegram_message_id TEXT NOT NULL,
		telegram_file_id TEXT NOT NULL, audio_path TEXT NOT NULL, transcript_path TEXT NOT NULL,
		status TEXT NOT NULL, created_at TEXT NOT NULL, transcription_started_at TEXT,
		transcription_finished_at TEXT, error_message TEXT
	);
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
	defer r.Close()
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
	if err := r.db.QueryRow(`SELECT attempts, next_attempt_at FROM notes WHERE id='job-1'`).Scan(&attempts, &due); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || due.Valid {
		t.Fatalf("migrated retry fields = %d, %v", attempts, due)
	}
}

func TestClaimOldestOnlyClaimsDueQueuedJob(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, job := range []Job{
		{ID: "later", AudioPath: "later.ogg", Status: Queued, CreatedAt: now.Add(-time.Hour)},
		{ID: "eligible", AudioPath: "eligible.ogg", Status: Queued, CreatedAt: now},
	} {
		if err := r.Insert(job); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE notes SET next_attempt_at=? WHERE id='later'`, now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	got, err := r.ClaimOldest(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "eligible" || got.Status != Transcribing {
		t.Fatalf("claim = %+v", got)
	}
	got, err = r.ClaimOldest(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("claimed future job: %+v", got)
	}
}

func TestConcurrentClaimsNeverReturnSameJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	r1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()
	r2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	now := time.Now().UTC()
	if err := r1.Insert(Job{ID: "only", AudioPath: "only.ogg", Status: Queued, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan *Job, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, repo := range []*Repository{r1, r2} {
		wg.Add(1)
		go func(repo *Repository) {
			defer wg.Done()
			<-start
			job, err := repo.ClaimOldest(context.Background(), now.Add(time.Second))
			if err != nil {
				errs <- err
				return
			}
			results <- job
		}(repo)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	claimed := 0
	for job := range results {
		if job != nil {
			if job.ID != "only" {
				t.Fatalf("unexpected claim: %+v", job)
			}
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed %d times, want once", claimed)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	_, err := Open("")
	if err == nil || err.Error() != "database path is required" {
		t.Fatalf("Open(\"\") error = %v", err)
	}
}
