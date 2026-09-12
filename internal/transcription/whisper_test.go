package transcription

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WHISPER_TEST_ARGS\"\nprintf 'log whisper-cli\\n' >&2\nprintf 'hasil transkripsi\\n'\n"
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
	if _, err := os.Stat(audio); err != nil {
		t.Fatalf("source audio was not preserved: %v", err)
	}
	gotArgs := strings.Split(strings.TrimSpace(string(args)), "\n")
	if len(gotArgs) != 10 {
		t.Fatalf("args = %q", string(args))
	}
	if gotArgs[0] != "-m" || gotArgs[1] != "models/ggml-small-q5_1.bin" || gotArgs[2] != "-f" || !strings.HasPrefix(filepath.Base(gotArgs[3]), "gsnote-whisper-") || filepath.Ext(gotArgs[3]) != ".wav" {
		t.Fatalf("model/audio args = %q", gotArgs[:4])
	}
	wantRest := []string{"-nt", "-np", "-l", "id", "-t", "2"}
	if !reflect.DeepEqual(gotArgs[4:], wantRest) {
		t.Fatalf("args = %q, want suffix %q", gotArgs, wantRest)
	}
	if _, err := os.Stat(gotArgs[3]); !os.IsNotExist(err) {
		t.Fatalf("temporary wav was not removed, err=%v", err)
	}
}
