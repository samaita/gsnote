package voice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const sttTimeout = 5 * time.Minute

// Transcriber converts audio files to text.
type Transcriber interface {
	Transcribe(audioPath string) (string, error)
}

// LocalTranscriber runs whisper.cpp's whisper-cli locally against the audio
// file. Audio never leaves the machine. The source file is expected to be an
// OGG/Opus voice note; it is converted to a 16kHz mono WAV in a temporary
// directory before being passed to whisper-cli.
type LocalTranscriber struct {
	Bin      string // whisper-cli binary (default "whisper-cli")
	Model    string // path to a ggml whisper model (required)
	Language string // whisper language, e.g. "auto" (default "auto")
	FFmpeg   string // ffmpeg binary (default "ffmpeg")
}

// Transcribe converts the audio to WAV and runs whisper-cli locally.
func (t *LocalTranscriber) Transcribe(audioPath string) (string, error) {
	if strings.TrimSpace(t.Model) == "" {
		return "", errors.New("STT_MODEL is not configured")
	}

	bin := t.Bin
	if bin == "" {
		bin = "whisper-cli"
	}
	ffmpeg := t.FFmpeg
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	lang := t.Language
	if lang == "" {
		lang = "auto"
	}

	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("whisper-cli binary %q not found: %w", bin, err)
	}
	if _, err := exec.LookPath(ffmpeg); err != nil {
		return "", fmt.Errorf("ffmpeg binary %q not found: %w", ffmpeg, err)
	}
	if _, err := os.Stat(t.Model); err != nil {
		return "", fmt.Errorf("STT model %q: %w", t.Model, err)
	}

	tmpDir, err := os.MkdirTemp("", "gsnote-stt-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	wavPath := filepath.Join(tmpDir, "audio.wav")
	if err := runCommand(sttTimeout, ffmpeg, "-y", "-i", audioPath, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath); err != nil {
		return "", fmt.Errorf("ffmpeg conversion: %w", err)
	}

	outPrefix := filepath.Join(tmpDir, "out")
	args := []string{"-m", t.Model, "-f", wavPath, "-l", lang, "-nt", "-otxt", "-of", outPrefix}
	if err := runCommand(sttTimeout, bin, args...); err != nil {
		return "", fmt.Errorf("whisper: %w", err)
	}

	data, err := os.ReadFile(outPrefix + ".txt")
	if err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func runCommand(timeout time.Duration, bin string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}
