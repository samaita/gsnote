package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	Queued             = "QUEUED"
	Transcribing       = "TRANSCRIBING"
	Done               = "DONE"
	Failed             = "FAILED"
	DefaultMaxAttempts = 5
	RetryBase          = 30 * time.Second
	RetryMax           = time.Hour
)

type Job struct {
	ID, ChatID, MessageID, FileID string
	AudioPath, TranscriptPath     string
	Status, ErrorMessage          string
	Attempts                      int
	NextAttemptAt                 *time.Time
	CreatedAt                     time.Time
}

type Repository struct{ db *sql.DB }

func Open(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	r := &Repository{db: db}
	if err := r.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

func (r *Repository) init() error {
	if _, err := r.db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("enable sqlite WAL: %w", err)
	}
	_, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY, telegram_chat_id TEXT NOT NULL, telegram_message_id TEXT NOT NULL,
		telegram_file_id TEXT NOT NULL, audio_path TEXT NOT NULL, transcript_path TEXT,
		status TEXT NOT NULL, created_at TEXT NOT NULL, transcription_started_at TEXT,
		transcription_finished_at TEXT, error_message TEXT, attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TEXT
	)`)
	if err != nil {
		return fmt.Errorf("create notes table: %w", err)
	}
	cols, err := r.columns()
	if err != nil {
		return err
	}
	if !cols["attempts"] {
		if _, err := r.db.Exec(`ALTER TABLE notes ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add attempts column: %w", err)
		}
	}
	if !cols["next_attempt_at"] {
		if _, err := r.db.Exec(`ALTER TABLE notes ADD COLUMN next_attempt_at TEXT`); err != nil {
			return fmt.Errorf("add next_attempt_at column: %w", err)
		}
	}
	return nil
}

func (r *Repository) columns() (map[string]bool, error) {
	rows, err := r.db.Query(`PRAGMA table_info(notes)`)
	if err != nil {
		return nil, fmt.Errorf("inspect notes schema: %w", err)
	}
	defer rows.Close()
	cols := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("read notes schema: %w", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read notes schema: %w", err)
	}
	return cols, nil
}
func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Insert(job Job) error {
	if job.ID == "" || job.AudioPath == "" {
		return errors.New("job ID and audio path are required")
	}
	if job.CreatedAt.IsZero() {
		return errors.New("job creation time is required")
	}
	if job.Status == "" {
		job.Status = Queued
	}
	_, err := r.db.Exec(`INSERT INTO notes (id, telegram_chat_id, telegram_message_id, telegram_file_id, audio_path, transcript_path, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, job.ID, job.ChatID, job.MessageID, job.FileID, job.AudioPath, nullable(job.TranscriptPath), job.Status, job.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("insert job %q: %w", job.ID, err)
	}
	return nil
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// List returns persisted queue rows ordered by creation time.
func (r *Repository) List(ctx context.Context) ([]Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,telegram_chat_id,telegram_message_id,telegram_file_id,audio_path,transcript_path,status,created_at,attempts,next_attempt_at,error_message FROM notes ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var created string
		var transcript, due, failure sql.NullString
		if err := rows.Scan(&j.ID, &j.ChatID, &j.MessageID, &j.FileID, &j.AudioPath, &transcript, &j.Status, &created, &j.Attempts, &due, &failure); err != nil {
			return nil, err
		}
		j.TranscriptPath = transcript.String
		j.ErrorMessage = failure.String
		j.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		if due.Valid {
			d, e := time.Parse(time.RFC3339Nano, due.String)
			if e != nil {
				return nil, e
			}
			j.NextAttemptAt = &d
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) ClaimOldest(ctx context.Context, now time.Time) (*Job, error) {
	stamp := now.UTC().Format(time.RFC3339Nano)
	var job Job
	var transcript sql.NullString
	var created string
	err := r.db.QueryRowContext(ctx, `UPDATE notes SET status = ?, transcription_started_at = ?
		WHERE id = (SELECT id FROM notes WHERE status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		ORDER BY COALESCE(next_attempt_at, created_at), created_at, id LIMIT 1) AND status = ?
		RETURNING id, telegram_chat_id, telegram_message_id, telegram_file_id, audio_path, transcript_path, created_at`,
		Transcribing, stamp, Queued, stamp, Queued).Scan(&job.ID, &job.ChatID, &job.MessageID, &job.FileID, &job.AudioPath, &transcript, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim queued job: %w", err)
	}
	job.TranscriptPath = transcript.String
	job.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, fmt.Errorf("parse job creation time: %w", err)
	}
	job.Status = Transcribing
	return &job, nil
}

func (r *Repository) Get(ctx context.Context, id string) (*Job, error) {
	var job Job
	var created string
	var next, transcript, failure sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, telegram_chat_id, telegram_message_id, telegram_file_id, audio_path, transcript_path, status, error_message, attempts, next_attempt_at, created_at FROM notes WHERE id = ?`, id).Scan(&job.ID, &job.ChatID, &job.MessageID, &job.FileID, &job.AudioPath, &transcript, &job.Status, &failure, &job.Attempts, &next, &created)
	if err != nil {
		return nil, err
	}
	job.TranscriptPath = transcript.String
	job.ErrorMessage = failure.String
	job.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return nil, err
	}
	if next.Valid {
		t, e := time.Parse(time.RFC3339Nano, next.String)
		if e != nil {
			return nil, e
		}
		job.NextAttemptAt = &t
	}
	return &job, nil
}

// Fail records one failed attempt for a claimed job, schedules retry or marks it terminal.
func (r *Repository) Fail(ctx context.Context, id, message string, now time.Time, maxAttempts int) (bool, error) {
	if maxAttempts < 1 {
		return false, errors.New("max attempts must be positive")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin failure transition: %w", err)
	}
	defer tx.Rollback()
	var attempts int
	if err := tx.QueryRowContext(ctx, `SELECT attempts FROM notes WHERE id = ? AND status = ?`, id, Transcribing).Scan(&attempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("read failed job: %w", err)
	}
	attempts++
	if attempts >= maxAttempts {
		_, err = tx.ExecContext(ctx, `UPDATE notes SET attempts=?, status=?, error_message=?, next_attempt_at=NULL, transcription_finished_at=? WHERE id=? AND status=?`, attempts, Failed, message, now.UTC().Format(time.RFC3339Nano), id, Transcribing)
	} else {
		next := now.UTC().Add(RetryDelay(attempts)).Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `UPDATE notes SET attempts=?, status=?, error_message=?, next_attempt_at=?, transcription_started_at=NULL, transcription_finished_at=NULL WHERE id=? AND status=?`, attempts, Queued, message, next, id, Transcribing)
	}
	if err != nil {
		return false, fmt.Errorf("update failed job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit failure transition: %w", err)
	}
	return attempts >= maxAttempts, nil
}

// Complete stores the output path and marks a claimed job done.
func (r *Repository) Complete(ctx context.Context, id, transcriptPath string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE notes SET status=?, transcript_path=?, error_message=NULL, next_attempt_at=NULL, transcription_finished_at=? WHERE id=? AND status=?`, Done, transcriptPath, time.Now().UTC().Format(time.RFC3339Nano), id, Transcribing)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("job %q is not claimed", id)
	}
	return nil
}

func (r *Repository) RecoverStale(ctx context.Context, olderThan time.Time, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE notes SET status=?, transcription_started_at=NULL,
		error_message='stale transcription recovered', next_attempt_at=NULL
		WHERE status=? AND transcription_started_at < ?`, Queued, Transcribing, olderThan.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("recover stale jobs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count recovered jobs: %w", err)
	}
	return count, nil
}
