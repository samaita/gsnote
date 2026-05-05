package task

import "testing"

func TestViewMonthReadsSingleMonthlyFile(t *testing.T) {
	root := t.TempDir()
	mgr := New(root)

	if err := mgr.Add("2026-04", "First"); err != nil {
		t.Fatalf("add first monthly task: %v", err)
	}
	if err := mgr.Add("2026-04", "Second"); err != nil {
		t.Fatalf("add second monthly task: %v", err)
	}

	got, err := mgr.ViewMonth("2026-04")
	if err != nil {
		t.Fatalf("view month: %v", err)
	}

	want := "Tasks 2026-04:\n1. [ ] First\n2. [ ] Second"
	if got != want {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestViewMonthNoTasks(t *testing.T) {
	root := t.TempDir()
	mgr := New(root)

	got, err := mgr.ViewMonth("2026-04")
	if err != nil {
		t.Fatalf("view month: %v", err)
	}
	if got != "No tasks for 2026-04." {
		t.Fatalf("unexpected output: %q", got)
	}
}
