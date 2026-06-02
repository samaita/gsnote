package journal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerCompleteAppendsDailyJournal(t *testing.T) {
	root := t.TempDir()
	mgr := New(root)
	now := time.Date(2026, 6, 3, 21, 15, 0, 0, time.FixedZone("GMT+7", 7*60*60))

	first := Entry{
		Summary:    "Shipped the journal flow",
		Highlights: []string{"Got the state machine working", "Kept the file format simple"},
		Blockers:   []string{"Telegram callback handling needed care"},
		Closing:    "Remember to keep capture quick.",
	}
	if err := mgr.Write(now, first); err != nil {
		t.Fatalf("write first journal: %v", err)
	}

	second := Entry{
		Summary:    "Late addendum",
		Highlights: []string{"Added cron reminder"},
		Blockers:   []string{"None"},
		Closing:    "Second sessions append.",
	}
	if err := mgr.Write(now, second); err != nil {
		t.Fatalf("write second journal: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "2026-06-03.md"))
	if err != nil {
		t.Fatalf("read journal file: %v", err)
	}

	expected := `# Journal - 2026-06-03

## 1. Summary

Shipped the journal flow

## 2. Highlights

1. Got the state machine working
2. Kept the file format simple

## 3. Blockers

1. Telegram callback handling needed care

## 4. Closing Reflection

Remember to keep capture quick.

---

## 1. Summary

Late addendum

## 2. Highlights

1. Added cron reminder

## 3. Blockers

1. None

## 4. Closing Reflection

Second sessions append.
`
	if string(data) != expected {
		t.Fatalf("unexpected journal content:\n%s", data)
	}
}

func TestManagerHasDataAndClearUseLocalDay(t *testing.T) {
	root := t.TempDir()
	mgr := New(root)
	loc := time.FixedZone("GMT+7", 7*60*60)
	now := time.Date(2026, 6, 3, 23, 0, 0, 0, loc)

	if mgr.HasData(now) {
		t.Fatal("did not expect journal data before write")
	}

	if err := mgr.Write(now, Entry{Summary: "x", Highlights: []string{"h"}, Blockers: []string{"b"}, Closing: "c"}); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	if !mgr.HasData(now) {
		t.Fatal("expected journal data after write")
	}

	if err := mgr.Clear(now); err != nil {
		t.Fatalf("clear journal: %v", err)
	}
	if mgr.HasData(now) {
		t.Fatal("did not expect journal data after clear")
	}
}

func TestSessionCollectsRepeatedHighlightsAndBlockers(t *testing.T) {
	session := NewSession()

	if got := session.Prompt(); got != "What happened today?" {
		t.Fatalf("unexpected initial prompt: %q", got)
	}
	if done := session.AcceptText("Built journal capture"); done {
		t.Fatal("session completed too early")
	}
	if got := session.Prompt(); got != "Add a highlight." {
		t.Fatalf("unexpected highlight prompt: %q", got)
	}
	session.AcceptText("First highlight")
	if got := session.Prompt(); got != "Any more highlights?" {
		t.Fatalf("unexpected highlight loop prompt: %q", got)
	}
	session.ChooseMoreHighlights()
	session.AcceptText("Second highlight")
	session.ChooseNoMoreHighlights()
	session.AcceptText("One blocker")
	session.ChooseNoMoreBlockers()

	if done := session.AcceptText("Close it out"); !done {
		t.Fatal("expected session to complete after closing reflection")
	}

	entry := session.Entry()
	if entry.Summary != "Built journal capture" {
		t.Fatalf("unexpected summary: %q", entry.Summary)
	}
	if len(entry.Highlights) != 2 || entry.Highlights[1] != "Second highlight" {
		t.Fatalf("unexpected highlights: %#v", entry.Highlights)
	}
	if len(entry.Blockers) != 1 || entry.Blockers[0] != "One blocker" {
		t.Fatalf("unexpected blockers: %#v", entry.Blockers)
	}
	if entry.Closing != "Close it out" {
		t.Fatalf("unexpected closing: %q", entry.Closing)
	}
}
