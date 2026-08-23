package handler

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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

func newTestHandler(t *testing.T) *Handler {
	return New(nil, t.TempDir(), t.TempDir(), "", "", "", nil)
}

func TestHandleRoutesVoiceMessageToProcessor(t *testing.T) {
	h := newTestHandler(t)
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
	h := newTestHandler(t)

	msg := &tgbotapi.Message{
		MessageID: 7,
		Chat:      &tgbotapi.Chat{ID: 1},
		From:      &tgbotapi.User{ID: 2},
		Voice:     &tgbotapi.Voice{FileID: "file_id"},
	}
	h.Handle(tgbotapi.Update{Message: msg})
}

func TestHandleTextDoesNotRouteToVoiceProcessor(t *testing.T) {
	h := newTestHandler(t)
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
	h := newTestHandler(t)
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
	h := newTestHandler(t)
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
	h := newTestHandler(t)
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
	h := newTestHandler(t)
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
