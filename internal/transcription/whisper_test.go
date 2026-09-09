package transcription

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWhisperTranscribeUsesWhisperCPPCLI(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.ogg")
	if err := os.WriteFile(audio, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "args")
	binary := filepath.Join(dir, "whisper-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WHISPER_TEST_ARGS\"\nprintf 'hasil transkripsi\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WHISPER_TEST_ARGS", logPath)

	got, err := (Whisper{Binary: binary, Model: "models/ggml-small-q5_1.bin", Language: "id", Threads: 2}).Transcribe(audio)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if got != "hasil transkripsi" {
		t.Fatalf("transcript = %q, want %q", got, "hasil transkripsi")
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "-m\nmodels/ggml-small-q5_1.bin\n-f\n" + audio + "\n-otxt\n-of\n-\n-nt\n-l\nid\n-t\n2\n"
	if string(args) != want {
		t.Fatalf("args = %q, want %q", string(args), want)
	}
}
