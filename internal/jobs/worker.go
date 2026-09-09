package jobs

import (
	"log"
	"os"
	"time"
)

type Transcriber interface{ Transcribe(string) (string, error) }
type Notifier interface {
	Ready(*Note, string) error
	Failed(*Note) error
}
type Worker struct {
	Repo        *Repository
	Transcriber Transcriber
	Notifier    Notifier
	Poll        time.Duration
	Stale       time.Duration
}

func (w *Worker) Run(stop <-chan struct{}) {
	if w.Poll <= 0 {
		w.Poll = time.Second
	}
	if w.Stale <= 0 {
		w.Stale = 30 * time.Minute
	}
	_ = w.Repo.RecoverStale(w.Stale)
	for {
		select {
		case <-stop:
			return
		case <-time.After(w.Poll):
			w.process()
		}
	}
}
func (w *Worker) process() {
	n, e := w.Repo.ClaimOldest()
	if e != nil {
		log.Printf("job claim error: %v", e)
		return
	}
	if n == nil {
		return
	}
	log.Printf("transcription_started note_id=%s", n.ID)
	text, e := w.Transcriber.Transcribe(n.AudioPath)
	if e != nil {
		log.Printf("transcription_failed note_id=%s: %v", n.ID, e)
		_ = w.Repo.Fail(n.ID, e.Error())
		if w.Notifier != nil {
			_ = w.Notifier.Failed(n)
		}
		return
	}
	if err := os.WriteFile(n.TranscriptPath, []byte(text), 0644); err != nil {
		log.Printf("transcription_failed note_id=%s: %v", n.ID, err)
		_ = w.Repo.Fail(n.ID, err.Error())
		return
	}
	if e = w.Repo.Complete(n.ID, n.TranscriptPath); e != nil {
		log.Printf("job complete error note_id=%s: %v", n.ID, e)
		return
	}
	log.Printf("transcription_completed note_id=%s", n.ID)
	if w.Notifier != nil {
		_ = w.Notifier.Ready(n, n.TranscriptPath)
	}
}
