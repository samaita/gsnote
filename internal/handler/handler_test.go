package handler

import (
	"errors"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/cron"
	"github.com/axonigma/gsnote/internal/journal"
	"github.com/axonigma/gsnote/internal/voice"
)

type fakeVoiceSvc struct {
	processed  int
	deleteID   string
	deleteOut  string
	deleteErr  error
	listCalled bool
}

func (f *fakeVoiceSvc) ProcessVoiceMessage(msg *tgbotapi.Message) {
	f.processed++
}

func (f *fakeVoiceSvc) Delete(id string) (string, error) {
	f.deleteID = id
	return f.deleteOut, f.deleteErr
}

func (f *fakeVoiceSvc) List() (string, error) {
	f.listCalled = true
	return "Recent voice captures:\n  00001", nil
}

func TestHandleRoutesVoiceMessageToProcessor(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{}
	h.StartVoiceProcessor(svc)

	msg := &tgbotapi.Message{
		MessageID: 7,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Voice:     &tgbotapi.Voice{FileID: "file_id"},
	}
	h.Handle(tgbotapi.Update{Message: msg})

	if svc.processed != 1 {
		t.Fatalf("expected voice message routed to processor, processed=%d", svc.processed)
	}
}

func TestHandleVoiceMessageWithoutProcessorDoesNotPanic(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)

	msg := &tgbotapi.Message{
		MessageID: 7,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Voice:     &tgbotapi.Voice{FileID: "file_id"},
	}
	h.Handle(tgbotapi.Update{Message: msg})
}

func TestHandleTextDoesNotRouteToVoiceProcessor(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{}
	h.StartVoiceProcessor(svc)

	msg := &tgbotapi.Message{
		MessageID: 8,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Text:      "just a thought",
	}
	h.Handle(tgbotapi.Update{Message: msg})

	if svc.processed != 0 {
		t.Fatalf("plain text must not reach the voice processor")
	}
}

func TestHandleVoiceDeleteCommand(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{deleteOut: "Deleted voice 00001."}
	h.StartVoiceProcessor(svc)

	msg := &tgbotapi.Message{
		MessageID: 9,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Text:      "/voice delete 00001",
	}
	h.Handle(tgbotapi.Update{Message: msg})

	if svc.deleteID != "00001" {
		t.Fatalf("expected delete of 00001, got %q", svc.deleteID)
	}
}

func TestHandleVoiceDeleteInvalidID(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{deleteErr: voice.ErrInvalidVoiceID}
	h.StartVoiceProcessor(svc)

	msg := &tgbotapi.Message{
		MessageID: 10,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Text:      "/voice delete ../",
	}
	h.Handle(tgbotapi.Update{Message: msg})

	if svc.deleteID != "../" {
		t.Fatalf("expected delete attempted with raw arg, got %q", svc.deleteID)
	}
}

func TestHandleVoiceListCommand(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{}
	h.StartVoiceProcessor(svc)

	msg := &tgbotapi.Message{
		MessageID: 11,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Text:      "/voice list",
	}
	h.Handle(tgbotapi.Update{Message: msg})

	if !svc.listCalled {
		t.Fatalf("expected list called")
	}
}

func TestHandleVoiceHelpDoesNotDelete(t *testing.T) {
	h := New(nil, t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "", "", "", nil)
	svc := &fakeVoiceSvc{}
	h.StartVoiceProcessor(svc)

	for _, text := range []string{"/voice", "/voice help", "/voice bogus"} {
		msg := &tgbotapi.Message{
			MessageID: 12,
			Chat:      &tgbotapi.Chat{ID: 1},
			From:      &tgbotapi.User{ID: 2},
			Text:      text,
		}
		h.Handle(tgbotapi.Update{Message: msg})
	}

	if svc.deleteID != "" || svc.listCalled {
		t.Fatalf("help/unknown voice commands must not trigger list or delete")
	}
}


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

func TestParseCronSpecAndCommandAcceptsJournal(t *testing.T) {
	spec, cmd, err := parseCronSpecAndCommand("23:00 /journal")
	if err != nil {
		t.Fatalf("parse cron journal args: %v", err)
	}
	if spec != "23:00" {
		t.Fatalf("unexpected spec: %q", spec)
	}
	if cmd != "/journal" {
		t.Fatalf("unexpected command: %q", cmd)
	}
}

func TestExecuteScheduledCommandTaskViewUsesToday(t *testing.T) {
	root := t.TempDir()
	h := New(nil, root, root, root, root, root, root, root, "", "", "", nil)

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
	h := New(nil, root, root, root, root, root, root, root, "", "", "", nil)

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

func TestExecuteScheduledJournalPromptsOnlyWhenMissing(t *testing.T) {
	root := t.TempDir()
	h := New(nil, root, root, root, root, root, root, root, "", "", "", nil)
	loc := time.FixedZone("GMT+7", 7*60*60)
	now := time.Date(2026, 6, 3, 23, 0, 0, 0, loc)

	out, err := h.executeScheduledCommand("/journal", 123, now)
	if err != nil {
		t.Fatalf("execute scheduled journal: %v", err)
	}
	if out != "Time to complete your journal. What happened today?" {
		t.Fatalf("unexpected missing journal prompt: %q", out)
	}

	if err := h.journalMgr.Write(now, journal.Entry{Summary: "Done", Highlights: []string{"Win"}, Blockers: []string{"None"}, Closing: "Close"}); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	out, err = h.executeScheduledCommand("/journal", 123, now)
	if err != nil {
		t.Fatalf("execute scheduled journal with data: %v", err)
	}
	if out != "" {
		t.Fatalf("expected no prompt when journal exists, got %q", out)
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
