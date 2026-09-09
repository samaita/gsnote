package transcription

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhisperTranscribeUsesLocalWhisperCLI(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "voice.ogg")
	if err := os.WriteFile(audio, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WHISPER_TEST_ARGS\"\nmkdir -p \"$6\"\nprintf 'hasil transkripsi\\n' > \"$6/voice.txt\"\n"
	binary := filepath.Join(binDir, "whisper")
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WHISPER_TEST_ARGS", logPath)

	got, err := (Whisper{Binary: "whisper", Model: "small", Language: "id", Threads: 2}).Transcribe(audio)
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
	wantArgs := "--model\nsmall\n--language\nid\n--output_dir\n" + filepath.Dir(audio) + "\n--output_format\ntxt\n--threads\n2\n" + audio + "\n"
	if string(args) != wantArgs {
		t.Fatalf("args = %q, want %q", string(args), wantArgs)
	}
	if strings.Contains(string(args), "-m\n") {
		t.Fatal("used whisper.cpp arguments")
	}
}
