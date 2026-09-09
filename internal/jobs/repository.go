package jobs

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

type Status string

const (
	Received     Status = "RECEIVED"
	Stored       Status = "STORED"
	Queued       Status = "QUEUED"
	Transcribing Status = "TRANSCRIBING"
	Done         Status = "DONE"
	Failed       Status = "FAILED"
)

type Note struct {
	ID, ChatID, MessageID, FileID, AudioPath, TranscriptPath, ErrorMessage string
	Status                                                                 Status
	CreatedAt, StartedAt, FinishedAt                                       time.Time
}
type Repository struct{ db *sql.DB }

func Open(path string) (*Repository, error) {
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	r := &Repository{db}
	_, e = db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS notes (id TEXT PRIMARY KEY, telegram_chat_id TEXT NOT NULL, telegram_message_id TEXT NOT NULL, telegram_file_id TEXT NOT NULL, audio_path TEXT NOT NULL, transcript_path TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, transcription_started_at TEXT, transcription_finished_at TEXT, error_message TEXT)`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return r, nil
}
func (r *Repository) Close() error { return r.db.Close() }
func (r *Repository) Insert(n Note) error {
	_, e := r.db.Exec(`INSERT INTO notes(id,telegram_chat_id,telegram_message_id,telegram_file_id,audio_path,transcript_path,status,created_at) VALUES(?,?,?,?,?,?,?,?)`, n.ID, n.ChatID, n.MessageID, n.FileID, n.AudioPath, n.TranscriptPath, n.Status, n.CreatedAt.UTC().Format(time.RFC3339Nano))
	return e
}
func (r *Repository) ClaimOldest() (*Note, error) {
	tx, e := r.db.Begin()
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var n Note
	var created string
	row := tx.QueryRow(`SELECT id,telegram_chat_id,telegram_message_id,telegram_file_id,audio_path,transcript_path,created_at FROM notes WHERE status=? ORDER BY created_at LIMIT 1`, Queued)
	if e = row.Scan(&n.ID, &n.ChatID, &n.MessageID, &n.FileID, &n.AudioPath, &n.TranscriptPath, &created); e != nil {
		if e == sql.ErrNoRows {
			return nil, nil
		}
		return nil, e
	}
	now := time.Now().UTC()
	res, e := tx.Exec(`UPDATE notes SET status=?,transcription_started_at=? WHERE id=? AND status=?`, Transcribing, now.Format(time.RFC3339Nano), n.ID, Queued)
	if e != nil {
		return nil, e
	}
	a, _ := res.RowsAffected()
	if a != 1 {
		return nil, nil
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	n.Status = Transcribing
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	n.StartedAt = now
	return &n, nil
}
func (r *Repository) Complete(id, path string) error {
	_, e := r.db.Exec(`UPDATE notes SET status=?,transcript_path=?,transcription_finished_at=? WHERE id=?`, Done, path, time.Now().UTC().Format(time.RFC3339Nano), id)
	return e
}
func (r *Repository) Fail(id, msg string) error {
	_, e := r.db.Exec(`UPDATE notes SET status=?,error_message=?,transcription_finished_at=? WHERE id=?`, Failed, msg, time.Now().UTC().Format(time.RFC3339Nano), id)
	return e
}
func (r *Repository) RecoverStale(age time.Duration) error {
	_, e := r.db.Exec(`UPDATE notes SET status=?,transcription_started_at=NULL,error_message=? WHERE status=? AND transcription_started_at < ?`, Queued, "stale transcription recovered", Transcribing, time.Now().Add(-age).UTC().Format(time.RFC3339Nano))
	return e
}
func (r *Repository) String() string { return fmt.Sprintf("repository") }
