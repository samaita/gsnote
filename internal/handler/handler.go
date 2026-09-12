package handler

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const helpText = `gsnote — voice-only notes.

Send a voice message to capture a note (audio + transcript).

Commands:
  /help — this message`

const warnText = `Command not found, use /help for guide`

// voiceService is the minimal voice pipeline surface the handler depends on.
type voiceService interface {
	ProcessVoiceMessage(msg *tgbotapi.Message)
}

// Handler routes incoming Telegram updates.
type Handler struct {
	bot                 *tgbotapi.BotAPI
	whitelistTelegramID map[int64]bool
	voiceSvc            voiceService
	sendToChat          func(chatID int64, text string, replyIDs ...int)
}

// New creates a Handler for the given bot and whitelist.
func New(bot *tgbotapi.BotAPI, whitelistTelegramID map[int64]bool) *Handler {
	h := &Handler{
		bot:                 bot,
		whitelistTelegramID: whitelistTelegramID,
	}
	h.sendToChat = h.send
	return h
}

// StartVoiceProcessor registers the voice pipeline used for voice messages.
func (h *Handler) StartVoiceProcessor(vp voiceService) {
	h.voiceSvc = vp
}

// Handle routes incoming updates to the appropriate handler.
func (h *Handler) Handle(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	text := strings.TrimSpace(msg.Text)

	if h.whitelistTelegramID != nil && !h.whitelistTelegramID[msg.From.ID] {
		return
	}

	if msg.Voice != nil {
		if h.voiceSvc == nil {
			h.reply(msg, "Voice capture unavailable: transcription is not configured.")
			return
		}
		h.voiceSvc.ProcessVoiceMessage(msg)
		return
	}

	switch {
	case text == "/help":
		h.reply(msg, helpText)
	case strings.HasPrefix(text, "/"):
		h.reply(msg, warnText)
	}
}

func (h *Handler) reply(msg *tgbotapi.Message, text string) {
	if msg == nil {
		return
	}
	h.sendToChat(msg.Chat.ID, text, msg.MessageID)
}

func (h *Handler) send(chatID int64, text string, replyIDs ...int) {
	if h.bot == nil {
		return
	}
	reply := tgbotapi.NewMessage(chatID, text)
	if len(replyIDs) > 0 {
		reply.ReplyToMessageID = replyIDs[0]
	}
	if _, err := h.bot.Send(reply); err != nil {
		log.Printf("send reply error: %v", err)
	}
}
