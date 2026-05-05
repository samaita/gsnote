package handler

import (
	"errors"
	"testing"

	"github.com/axonigma/gsnote/internal/cron"
)

func TestParseCronSpecAndCommandAcceptsDailyTaskView(t *testing.T) {
	spec, cmd, err := parseCronSpecAndCommand("06:00 /task view")
	if err != nil {
		t.Fatalf("parse cron create args: %v", err)
	}
	if spec != "06:00" {
		t.Fatalf("unexpected spec: %q", spec)
	}
	if cmd != "/task view" {
		t.Fatalf("unexpected command: %q", cmd)
	}
}

func TestParseCronEditArgsAcceptsCronSyntax(t *testing.T) {
	line, spec, cmd, err := parseCronEditArgs("2 */5 * * * * /sync")
	if err != nil {
		t.Fatalf("parse cron edit args: %v", err)
	}
	if line != 2 || spec != "*/5 * * * *" || cmd != "/sync" {
		t.Fatalf("unexpected parse result: line=%d spec=%q cmd=%q", line, spec, cmd)
	}
}

func TestParseCronSpecAndCommandRejectsUnsupportedCommand(t *testing.T) {
	if _, _, err := parseCronSpecAndCommand("06:00 /note https://example.com desc"); !errors.Is(err, cron.ErrUnsupportedCmd) {
		t.Fatalf("expected unsupported command error, got %v", err)
	}
}

func TestExecuteScheduledCommandTaskViewUsesToday(t *testing.T) {
	root := t.TempDir()
	h := New(nil, root, root, root, root, root, root, "", "", "", nil)

	today := "2026-04-25"
	if err := h.taskMgr.Add(today, "Review cron output"); err != nil {
		t.Fatalf("add task: %v", err)
	}

	out, err := h.executeTaskView(today)
	if err != nil {
		t.Fatalf("execute task view: %v", err)
	}
	expected := "Tasks 2026-04-25:\n1. [ ] Review cron output"
	if out != expected {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestExecuteDefaultTaskViewShowsMonthAndToday(t *testing.T) {
	root := t.TempDir()
	h := New(nil, root, root, root, root, root, root, "", "", "", nil)

	if err := h.taskMgr.Add("2026-04", "Monthly only"); err != nil {
		t.Fatalf("add monthly task: %v", err)
	}
	if err := h.taskMgr.Add("2026-04-25", "Today task"); err != nil {
		t.Fatalf("add today task: %v", err)
	}

	out, err := h.executeDefaultTaskView("2026-04-25")
	if err != nil {
		t.Fatalf("execute default task view: %v", err)
	}

	expected := "Tasks 2026-04:\n1. [ ] Monthly only\n\nTasks 2026-04-25:\n1. [ ] Today task"
	if out != expected {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestIsMonthArg(t *testing.T) {
	if !isMonthArg("2026-04") {
		t.Fatalf("expected valid month")
	}
	if isMonthArg("2026-04-25") {
		t.Fatalf("did not expect full date to be treated as month")
	}
	if isMonthArg("2026-13") {
		t.Fatalf("did not expect invalid month")
	}
}

func TestParseTaskDeleteArgsAcceptsMonth(t *testing.T) {
	date, n, ok := parseTaskNArgs([]string{"2026-04", "2"}, "2026-04-25")
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if date != "2026-04" || n != 2 {
		t.Fatalf("unexpected parse result: date=%q n=%d", date, n)
	}
}

func TestParseTaskEditArgsAcceptsMonth(t *testing.T) {
	date, n, text, ok := parseTaskEditArgs([]string{"2026-04", "2", "Update", "roadmap"}, "2026-04-25")
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if date != "2026-04" || n != 2 || text != "Update roadmap" {
		t.Fatalf("unexpected parse result: date=%q n=%d text=%q", date, n, text)
	}
}

func TestParseTaskNewArgsAcceptsMonth(t *testing.T) {
	date, text, ok := parseTaskNewArgs([]string{"2026-04", "Plan", "review"}, "2026-04-25")
	if !ok {
		t.Fatalf("expected parse to succeed")
	}
	if date != "2026-04" || text != "Plan review" {
		t.Fatalf("unexpected parse result: date=%q text=%q", date, text)
	}
}
