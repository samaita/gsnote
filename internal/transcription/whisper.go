package transcription

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Transcriber interface {
	Transcribe(audioPath string) (string, error)
}
type Whisper struct {
	Binary, Model, Language string
	Threads                 int
}

func (w Whisper) Transcribe(audioPath string) (string, error) {
	if w.Binary == "" {
		w.Binary = "whisper-cli"
	}
	if w.Model == "" {
		return "", fmt.Errorf("TRANSCRIBER_MODEL is required")
	}
	args := []string{"-m", w.Model, "-f", audioPath, "-otxt", "-of", "-", "-nt"}
	if w.Language != "" {
		args = append(args, "-l", w.Language)
	}
	if w.Threads > 0 {
		args = append(args, "-t", fmt.Sprint(w.Threads))
	}
	out, e := exec.CommandContext(context.Background(), w.Binary, args...).CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("whisper: %w: %s", e, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
