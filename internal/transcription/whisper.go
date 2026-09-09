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
	wavPath := filepath.Join(filepath.Dir(audioPath), strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))+".wav")
	convertArgs := []string{"-y", "-i", audioPath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath}
	if out, e := exec.CommandContext(context.Background(), "ffmpeg", convertArgs...).CombinedOutput(); e != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", e, strings.TrimSpace(string(out)))
	}
	if e := os.Remove(audioPath); e != nil {
		return "", fmt.Errorf("ffmpeg: remove source audio: %w", e)
	}
	args := []string{"-m", w.Model, "-f", wavPath, "-otxt", "-of", "-", "-nt"}
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
