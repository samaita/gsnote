package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Storage writes voice recordings and reserves the Inbox directory layout.
type Storage struct {
	voices string
}

// New creates the Inbox directories beneath root.
func New(root string) (*Storage, error) {
	voices := filepath.Join(root, "Inbox", "Voices")
	texts := filepath.Join(root, "Inbox", "Texts")
	if err := os.MkdirAll(voices, 0o755); err != nil {
		return nil, fmt.Errorf("create voices directory: %w", err)
	}
	if err := os.MkdirAll(texts, 0o755); err != nil {
		return nil, fmt.Errorf("create texts directory: %w", err)
	}
	return &Storage{voices: voices}, nil
}

// SaveAudio writes audio using exclusive creation so an existing ID is never replaced.
func (s *Storage) SaveAudio(filename string, src io.Reader) error {
	if filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\") {
		return fmt.Errorf("invalid audio filename %q", filename)
	}
	path := filepath.Join(s.voices, filename)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create audio %q: %w", path, err)
	}
	if _, err := io.Copy(f, src); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write audio %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close audio %q: %w", path, err)
	}
	return nil
}
