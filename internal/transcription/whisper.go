package transcription

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	outputDir := filepath.Dir(audioPath)
	args := []string{"--model", w.Model}
	if w.Language != "" {
		args = append(args, "--language", w.Language)
	}
	args = append(args, "--output_dir", outputDir, "--output_format", "txt")
	if w.Threads > 0 {
		args = append(args, "--threads", fmt.Sprint(w.Threads))
	}
	args = append(args, audioPath)
	out, e := exec.CommandContext(context.Background(), w.Binary, args...).CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("whisper: %w: %s", e, strings.TrimSpace(string(out)))
	}
	transcriptPath := filepath.Join(outputDir, strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))+".txt")
	transcript, e := os.ReadFile(transcriptPath)
	if e != nil {
		return "", fmt.Errorf("whisper: read transcript: %w: %s", e, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(transcript)), nil
}
