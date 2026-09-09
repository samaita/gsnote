package handler

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type fakeVoiceService struct {
	called int
}

func (f *fakeVoiceService) ProcessVoiceMessage(msg *tgbotapi.Message) {
	f.called++
}

func newTestHandler(whitelist map[int64]bool) (*Handler, *[]string) {
	h := New(nil, whitelist)
	sent := []string{}
	h.sendToChat = func(chatID int64, text string, replyIDs ...int) {
		sent = append(sent, text)
	}
	return h, &sent
}

func TestHandleHelp(t *testing.T) {
	h, sent := newTestHandler(map[int64]bool{100: true})
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 1,
		From:      &tgbotapi.User{ID: 100},
		Chat:      &tgbotapi.Chat{ID: 100},
		Text:      "/help",
	}})
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "/help") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestHandleVoiceMessage(t *testing.T) {
	h, _ := newTestHandler(map[int64]bool{100: true})
	vs := &fakeVoiceService{}
	h.StartVoiceProcessor(vs)
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 2,
		From:      &tgbotapi.User{ID: 100},
		Chat:      &tgbotapi.Chat{ID: 100},
		Voice:     &tgbotapi.Voice{FileID: "f1"},
	}})
	if vs.called != 1 {
		t.Fatalf("voice service called %d times, want 1", vs.called)
	}
}

func TestHandleVoiceWithoutProcessor(t *testing.T) {
	h, sent := newTestHandler(map[int64]bool{100: true})
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 3,
		From:      &tgbotapi.User{ID: 100},
		Chat:      &tgbotapi.Chat{ID: 100},
		Voice:     &tgbotapi.Voice{FileID: "f1"},
	}})
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "unavailable") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestHandleWhitelistRejects(t *testing.T) {
	h, sent := newTestHandler(map[int64]bool{100: true})
	vs := &fakeVoiceService{}
	h.StartVoiceProcessor(vs)
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 4,
		From:      &tgbotapi.User{ID: 999},
		Chat:      &tgbotapi.Chat{ID: 999},
		Text:      "/help",
	}})
	if len(*sent) != 0 {
		t.Fatalf("non-whitelisted message must be ignored, sent = %v", *sent)
	}
	if vs.called != 0 {
		t.Fatalf("voice service must not be called for strangers")
	}
}

func TestHandleUnknownCommand(t *testing.T) {
	h, sent := newTestHandler(map[int64]bool{100: true})
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 5,
		From:      &tgbotapi.User{ID: 100},
		Chat:      &tgbotapi.Chat{ID: 100},
		Text:      "/sync",
	}})
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "not found") {
		t.Fatalf("sent = %v", *sent)
	}
}

func TestHandlePlainNoop(t *testing.T) {
	h, sent := newTestHandler(map[int64]bool{100: true})
	h.Handle(tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 6,
		From:      &tgbotapi.User{ID: 100},
		Chat:      &tgbotapi.Chat{ID: 100},
		Text:      "just chatting",
	}})
	if len(*sent) != 0 {
		t.Fatalf("plain text must be ignored, sent = %v", *sent)
	}
}
