package voice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatalf("write script %s: %v", name, err)
	}
	return path
}

func newLocalTranscriber(t *testing.T, binDir string, opts map[string]string) *LocalTranscriber {
	t.Helper()
	model := filepath.Join(t.TempDir(), "ggml-model.bin")
	if err := os.WriteFile(model, []byte("model"), 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	tr := &LocalTranscriber{
		Bin:    writeScript(t, binDir, "whisper-cli", `out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-of" ]; then out="$a"; fi
  prev="$a"
done
printf 'Gue kepikiran Address Quality bisa dijual sebagai API audit buat ecommerce' > "${out}.txt"
`),
		Model:    model,
		Language: "auto",
		FFmpeg:   writeScript(t, binDir, "ffmpeg", `out=""
for a in "$@"; do out="$a"; done
touch "$out"
`),
	}
	for k, v := range opts {
		switch k {
		case "bin":
			tr.Bin = v
		case "model":
			tr.Model = v
		case "ffmpeg":
			tr.FFmpeg = v
		}
	}
	return tr
}

func TestLocalTranscriberSuccess(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, nil)
	audio := filepath.Join(t.TempDir(), "voice.ogg")
	if err := os.WriteFile(audio, []byte("not-real-ogg"), 0644); err != nil {
		t.Fatalf("write audio: %v", err)
	}

	got, err := tr.Transcribe(audio)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	want := "Gue kepikiran Address Quality bisa dijual sebagai API audit buat ecommerce"
	if got != want {
		t.Fatalf("unexpected transcript: %q", got)
	}

	// Temp working dirs must be cleaned up after transcription.
	leftovers, err := filepath.Glob(filepath.Join(os.TempDir(), "gsnote-stt-*"))
	if err != nil {
		t.Fatalf("glob temp: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("expected no leftover temp dirs, got %v", leftovers)
	}
}

func TestLocalTranscriberMissingModelConfig(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, map[string]string{"model": ""})
	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); !strings.Contains(err.Error(), "STT_MODEL is not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalTranscriberMissingModelFile(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, map[string]string{"model": filepath.Join(t.TempDir(), "nope.bin")})
	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); !strings.Contains(err.Error(), "STT model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalTranscriberMissingBin(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, map[string]string{"bin": filepath.Join(binDir, "does-not-exist")})
	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); err == nil {
		t.Fatal("expected error for missing whisper binary")
	}
}

func TestLocalTranscriberMissingFFmpeg(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, map[string]string{"ffmpeg": filepath.Join(binDir, "no-ffmpeg")})
	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); err == nil {
		t.Fatal("expected error for missing ffmpeg binary")
	}
}

func TestLocalTranscriberFFmpegFailure(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, nil)
	tr.FFmpeg = writeScript(t, binDir, "ffmpeg-fail", `echo "no audio decoder" >&2
exit 1
`)

	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); err == nil {
		t.Fatal("expected error for ffmpeg failure")
	} else if !strings.Contains(err.Error(), "ffmpeg conversion") {
		t.Fatalf("unexpected error: %v", err)
	} else if !strings.Contains(err.Error(), "no audio decoder") {
		t.Fatalf("expected stderr included in error: %v", err)
	}
}

func TestLocalTranscriberWhisperFailure(t *testing.T) {
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, nil)
	tr.Bin = writeScript(t, binDir, "whisper-cli-fail", `echo "model load failed" >&2
exit 1
`)

	if _, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg")); err == nil {
		t.Fatal("expected error for whisper failure")
	} else if !strings.Contains(err.Error(), "whisper") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalTranscriberDefaultBinaries(t *testing.T) {
	// With default (unset) Bin/FFmpeg the transcriber resolves real binaries.
	// If neither is installed on this machine, Transcribe must fail with a
	// clear "not found" error rather than panicking.
	tr := &LocalTranscriber{Model: filepath.Join(t.TempDir(), "model.bin")}
	if err := os.WriteFile(tr.Model, []byte("m"), 0644); err != nil {
		t.Fatalf("write model: %v", err)
	}
	_, err := tr.Transcribe(filepath.Join(t.TempDir(), "voice.ogg"))
	if err == nil {
		t.Skip("whisper-cli and ffmpeg installed; skipping default-binary check")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected binary not found error, got: %v", err)
	}
}

func TestLocalTranscriberPipelineIntegration(t *testing.T) {
	// End-to-end: audio persisted, LocalTranscriber invoked via fakes,
	// LLM receives the transcript, markdown written, success reply sent.
	binDir := t.TempDir()
	tr := newLocalTranscriber(t, binDir, nil)
	llm := fakeLLM{fn: func(transcript string) (*VoiceInfo, error) {
		if transcript == "" || !strings.Contains(transcript, "Address Quality") {
			t.Fatalf("LLM must receive the real transcript, got %q", transcript)
		}
		return validInfo(), nil
	}}

	voicesRoot := t.TempDir()
	p, sent := newTestProcessor(t, voicesRoot, tr, llm)
	p.ProcessVoiceMessage(voiceMessage(1))

	if len(*sent) != 1 || !strings.Contains((*sent)[0], "Saved 00001") {
		t.Fatalf("unexpected replies: %v", *sent)
	}
	audio := listAudioFiles(t, voicesRoot)
	if len(audio) != 1 {
		t.Fatalf("expected one persisted audio, got %v", audio)
	}
}
