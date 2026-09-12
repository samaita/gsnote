package transcription

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	if _, err := os.Stat(audioPath); err != nil {
		return "", fmt.Errorf("open audio: %w", err)
	}

	wavFile, err := os.CreateTemp("", "gsnote-whisper-*.wav")
	if err != nil {
		return "", fmt.Errorf("create temporary wav: %w", err)
	}
	wavPath := wavFile.Name()
	if err := wavFile.Close(); err != nil {
		os.Remove(wavPath)
		return "", fmt.Errorf("close temporary wav: %w", err)
	}
	defer os.Remove(wavPath)

	convertArgs := []string{"-y", "-i", audioPath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath}
	if out, e := exec.CommandContext(context.Background(), "ffmpeg", convertArgs...).CombinedOutput(); e != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", e, strings.TrimSpace(string(out)))
	}
	args := []string{"-m", w.Model, "-f", wavPath, "-nt", "-np"}
	if w.Language != "" {
		args = append(args, "-l", w.Language)
	}
	if w.Threads > 0 {
		args = append(args, "-t", fmt.Sprint(w.Threads))
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(context.Background(), w.Binary, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if e := cmd.Run(); e != nil {
		return "", fmt.Errorf("whisper: %w: %s", e, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
