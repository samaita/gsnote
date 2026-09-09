package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Storage struct{ Root, AudioDir, TranscriptDir string }

func New(root string) (*Storage, error) {
	s := &Storage{Root: root, AudioDir: filepath.Join(root, "audio"), TranscriptDir: filepath.Join(root, "transcripts")}
	if e := os.MkdirAll(s.AudioDir, 0755); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(s.TranscriptDir, 0755); e != nil {
		return nil, e
	}
	return s, nil
}
func NoteID(now time.Time, suffix string) string {
	return fmt.Sprintf("VN-%s-%s", now.Format("20060102-150405"), suffix)
}
func (s *Storage) AudioPath(id, ext string) string { return filepath.Join(s.AudioDir, id+ext) }
func (s *Storage) TranscriptPath(id string) string { return filepath.Join(s.TranscriptDir, id+".txt") }
func (s *Storage) SaveAudio(src, id, ext string) (string, error) {
	dst := s.AudioPath(id, ext)
	in, e := os.Open(src)
	if e != nil {
		return "", e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return "", e
	}
	if _, e = io.Copy(out, in); e == nil {
		e = out.Close()
	} else {
		out.Close()
	}
	if e != nil {
		os.Remove(dst)
		return "", e
	}
	return dst, nil
}
func (s *Storage) SaveTranscript(id, text string) (string, error) {
	path := s.TranscriptPath(id)
	tmp, e := os.CreateTemp(s.TranscriptDir, ".transcript-")
	if e != nil {
		return "", e
	}
	name := tmp.Name()
	if _, e = tmp.WriteString(text); e == nil {
		e = tmp.Close()
	} else {
		tmp.Close()
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	if e != nil {
		os.Remove(name)
		return "", e
	}
	return path, nil
}
