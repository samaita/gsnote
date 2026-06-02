package cron

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeSpecAcceptsDailyTime(t *testing.T) {
	got, err := NormalizeSpec("06:00")
	if err != nil {
		t.Fatalf("normalize spec: %v", err)
	}
	if got != "0 6 * * *" {
		t.Fatalf("unexpected normalized spec: %q", got)
	}
}

func TestNormalizeSpecAcceptsCronSyntax(t *testing.T) {
	got, err := NormalizeSpec("*/15 9-17 * * 1-5")
	if err != nil {
		t.Fatalf("normalize spec: %v", err)
	}
	if got != "*/15 9-17 * * 1-5" {
		t.Fatalf("unexpected normalized spec: %q", got)
	}
}

func TestNormalizeSpecRejectsInvalid(t *testing.T) {
	if _, err := NormalizeSpec("61 6 * * *"); err == nil {
		t.Fatal("expected invalid spec error")
	}
}

func TestNormalizeCommandAllowlist(t *testing.T) {
	if got, err := NormalizeCommand("/task view"); err != nil || got != "/task view" {
		t.Fatalf("expected /task view, got %q err=%v", got, err)
	}
	if got, err := NormalizeCommand("/journal"); err != nil || got != "/journal" {
		t.Fatalf("expected /journal, got %q err=%v", got, err)
	}
	if _, err := NormalizeCommand("/note https://example.com desc"); !errors.Is(err, ErrUnsupportedCmd) {
		t.Fatalf("expected unsupported command error, got %v", err)
	}
}

func TestStoreCRUDByLineNumber(t *testing.T) {
	store := NewStore(t.TempDir())

	first, err := store.Add("06:00", "/task view", 123)
	if err != nil {
		t.Fatalf("add first: %v", err)
	}
	if first.Spec != "0 6 * * *" {
		t.Fatalf("unexpected first spec: %q", first.Spec)
	}

	if _, err := store.Add("*/5 * * * *", "/sync", 456); err != nil {
		t.Fatalf("add second: %v", err)
	}

	edited, err := store.Edit(2, "07:30", "/sync")
	if err != nil {
		t.Fatalf("edit second: %v", err)
	}
	if edited.ChatID != 456 {
		t.Fatalf("expected existing chat id to be preserved, got %d", edited.ChatID)
	}

	entries, err := store.List()
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].Spec != "30 7 * * *" {
		t.Fatalf("unexpected edited spec: %q", entries[1].Spec)
	}

	deleted, err := store.Delete(1)
	if err != nil {
		t.Fatalf("delete first: %v", err)
	}
	if deleted.ChatID != 123 {
		t.Fatalf("unexpected deleted chat id: %d", deleted.ChatID)
	}

	entries, err = store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestStoreRejectsUnknownLine(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Delete(1); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected entry not found, got %v", err)
	}
}

func TestDueMatchesSchedule(t *testing.T) {
	entries := []Entry{
		{Spec: "0 6 * * *", Cmd: "/task view", ChatID: 1},
		{Spec: "*/5 * * * *", Cmd: "/sync", ChatID: 2},
	}
	at := time.Date(2026, 4, 25, 6, 0, 10, 0, time.UTC)

	due, err := Due(entries, at)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("expected 2 due entries, got %d", len(due))
	}
}

func TestMatchesAcceptsSundayAsSeven(t *testing.T) {
	at := time.Date(2026, 4, 26, 6, 0, 0, 0, time.UTC)
	match, err := Matches("0 6 * * 7", at)
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	if !match {
		t.Fatal("expected Sunday schedule to match")
	}
}

func TestStoreRoundTripsFileFormat(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Add("06:00", "/task view", 999); err != nil {
		t.Fatalf("add: %v", err)
	}

	entries, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if got := filepath.Base(store.Path()); got != FileName {
		t.Fatalf("unexpected file name: %q", got)
	}
}

func TestStoreRejectsAdditionalCronFiles(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	if err := os.WriteFile(filepath.Join(root, "old-cron.txt"), []byte("0 6 * * *\t1\t/task view\n"), 0644); err != nil {
		t.Fatalf("seed extra cron file: %v", err)
	}

	if _, err := store.List(); !errors.Is(err, ErrMultipleFiles) {
		t.Fatalf("expected multiple files error, got %v", err)
	}
}
