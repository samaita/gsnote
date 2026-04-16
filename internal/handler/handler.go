package handler

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/parser"
	"github.com/axonigma/gsnote/internal/writer"
)

const helpText = `Usage:
  /habit <name> [value] [note]
  /habit list

  name  — single word (required)
  value — number, supports decimals (optional)
  note  — free text (optional)
  
  list  — show all tracked habits (first 10 only)

Examples:
  /habit work
  /habit work deep focus session
  /habit pushup 20
  /habit pushup 20 after dinner
  /habit run 2.5 morning jog
  /habit list

/help — show this message`

const warnText = `Command not found, use /help for guide`

// Handler holds dependencies for command handling.
type Handler struct {
	bot                 *tgbotapi.BotAPI
	habitsRoot          string
	whitelistTelegramID map[int64]bool
}

func New(bot *tgbotapi.BotAPI, habitsRoot string, whitelistTelegramID map[int64]bool) *Handler {
	return &Handler{bot: bot, habitsRoot: habitsRoot, whitelistTelegramID: whitelistTelegramID}
}

// Handle routes incoming updates to the appropriate command handler.
func (h *Handler) Handle(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	text := strings.TrimSpace(msg.Text)

	if h.whitelistTelegramID != nil && !h.whitelistTelegramID[msg.From.ID] {
		return
	}

	switch {
	case strings.HasPrefix(text, "/habit"):
		h.handleHabit(msg, strings.TrimPrefix(text, "/habit"))
	case text == "/help":
		h.reply(msg, helpText)
	case strings.Split(text, "")[0] == "/":
		h.reply(msg, warnText)
	}
}

func (h *Handler) handleHabitList(msg *tgbotapi.Message) {
	entries, err := os.ReadDir(h.habitsRoot)
	if err != nil {
		log.Printf("read habits dir error: %v", err)
		h.reply(msg, "Could not read habits folder.")
		return
	}

	var habits []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			habits = append(habits, strings.TrimSuffix(e.Name(), ".md"))
		}
	}

	if len(habits) == 0 {
		h.reply(msg, "No habits found.")
		return
	}

	const limit = 10
	truncated := len(habits) > limit
	if truncated {
		habits = habits[:limit]
	}

	var sb strings.Builder
	sb.WriteString("Available habits:\n")
	for _, name := range habits {
		sb.WriteString("  • ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	if truncated {
		sb.WriteString("\nToo many habits to display. Please check Obsidian for the full list.")
	}

	h.reply(msg, sb.String())
}

func (h *Handler) handleHabit(msg *tgbotapi.Message, args string) {
	if strings.TrimSpace(args) == "list" {
		h.handleHabitList(msg)
		return
	}

	input, err := parser.Parse(args)
	if err != nil {
		h.reply(msg, "Please re-enter with the correct format. Use /help.")
		return
	}

	if err := writer.Write(h.habitsRoot, input, time.Now()); err != nil {
		log.Printf("write error for habit %q: %v", input.Habit, err)
		h.reply(msg, "Try again with the correct command. Use /help.")
		return
	}

	h.reply(msg, fmt.Sprintf("Logged: %s", input.Habit))
}

func (h *Handler) reply(msg *tgbotapi.Message, text string) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := h.bot.Send(reply); err != nil {
		log.Printf("send reply error: %v", err)
	}
}
