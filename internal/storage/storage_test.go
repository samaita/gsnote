package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesInboxDirectories(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Voices", "Texts"} {
		info, err := os.Stat(filepath.Join(root, "Inbox", name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", name)
		}
	}
}

func TestSaveAudioDoesNotOverwriteExistingID(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAudio("00001-20261004170000.ogg", bytes.NewBufferString("first recording")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAudio("00001-20261004170000.ogg", bytes.NewBufferString("replacement")); err == nil {
		t.Fatal("expected duplicate ID to fail")
	}
	got, err := os.ReadFile(filepath.Join(root, "Inbox", "Voices", "00001-20261004170000.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first recording" {
		t.Fatalf("recording overwritten: %q", got)
	}
}
