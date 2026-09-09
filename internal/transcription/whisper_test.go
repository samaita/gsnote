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
	ffmpegDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(ffmpegDir, 0700); err != nil {
		t.Fatal(err)
	}
	ffmpeg := filepath.Join(ffmpegDir, "ffmpeg")
	ffmpegScript := "#!/bin/sh\nfor arg in \"$@\"; do out=\"$arg\"; done\nprintf 'wav' > \"$out\"\n"
	if err := os.WriteFile(ffmpeg, []byte(ffmpegScript), 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "whisper-cli")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WHISPER_TEST_ARGS\"\nprintf 'hasil transkripsi\\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WHISPER_TEST_ARGS", logPath)
	t.Setenv("PATH", ffmpegDir+string(os.PathListSeparator)+os.Getenv("PATH"))

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
	wav := filepath.Join(dir, "voice.wav")
	if _, err := os.Stat(wav); err != nil {
		t.Fatalf("converted audio missing: %v", err)
	}
	if _, err := os.Stat(audio); !os.IsNotExist(err) {
		t.Fatalf("source audio still exists, err=%v", err)
	}
	want := "-m\nmodels/ggml-small-q5_1.bin\n-f\n" + wav + "\n-otxt\n-of\n-\n-nt\n-l\nid\n-t\n2\n"
	if string(args) != want {
		t.Fatalf("args = %q, want %q", string(args), want)
	}
}
