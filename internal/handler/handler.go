package handler

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/parser"
	"github.com/axonigma/gsnote/internal/syncgit"
	"github.com/axonigma/gsnote/internal/voice"
	"github.com/axonigma/gsnote/internal/writer"
)

const habitUsageText = `Usage:
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
  /habit list`

const voiceUsageText = `Available voice commands:
  /voice list           — list recent voice captures
  /voice delete <id>    — delete a voice capture, e.g. /voice delete 00001
  /voice help           — show this help

Tip: send a Telegram voice message to capture a note.`

const helpText = `Available commands:
  /habit — log or list habits
  /voice — capture a voice note; manage with /voice list, /voice delete
  /sync  — git add, commit, and push to origin main

Tip: type a command alone to see its full usage.`

const (
	cmdHabit = "/habit"
	cmdVoice = "/voice"
	cmdSync  = "/sync"
	cmdHelp  = "/help"
)

const habitCmdList = "list"

const warnText = `Command not found, use /help for guide`

// voiceService is the minimal voice pipeline surface the handler depends on.
type voiceService interface {
	ProcessVoiceMessage(msg *tgbotapi.Message)
	Delete(voiceID string) (string, error)
	List() (string, error)
}

// Handler holds dependencies for command handling.
type Handler struct {
	bot                 *tgbotapi.BotAPI
	habitsRoot          string
	syncRoot            string
	syncer              *syncgit.Service
	whitelistTelegramID map[int64]bool
	voiceSvc            voiceService
}

func New(bot *tgbotapi.BotAPI, habitsRoot, syncRoot, githubToken, gitAuthorName, gitAuthorEmail string, whitelistTelegramID map[int64]bool) *Handler {
	return &Handler{
		bot:                 bot,
		habitsRoot:          habitsRoot,
		syncRoot:            syncRoot,
		syncer:              syncgit.New(syncRoot, githubToken, gitAuthorName, gitAuthorEmail),
		whitelistTelegramID: whitelistTelegramID,
	}
}

// StartVoiceProcessor registers the voice pipeline used for voice messages
// and the /voice command.
func (h *Handler) StartVoiceProcessor(vp voiceService) {
	h.voiceSvc = vp
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

	if msg.Voice != nil {
		if h.voiceSvc == nil {
			h.reply(msg, "Voice capture unavailable: STT/LLM not configured.")
			return
		}
		h.voiceSvc.ProcessVoiceMessage(msg)
		return
	}

	switch {
	case strings.HasPrefix(text, cmdHabit):
		h.handleHabit(msg, strings.TrimPrefix(text, cmdHabit))
	case strings.HasPrefix(text, cmdVoice):
		h.handleVoice(msg, strings.TrimPrefix(text, cmdVoice))
	case text == cmdSync:
		h.handleSync(msg)
	case text == cmdHelp:
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
	args = strings.TrimSpace(args)
	if args == "" {
		h.reply(msg, habitUsageText)
		return
	}
	if args == habitCmdList {
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

func (h *Handler) handleVoice(msg *tgbotapi.Message, args string) {
	if h.voiceSvc == nil {
		h.reply(msg, "Voice capture unavailable: STT/LLM not configured.")
		return
	}

	parsed, err := parser.ParseVoiceCommand(args)
	if err != nil {
		h.reply(msg, voiceUsageText)
		return
	}

	switch parsed.Action {
	case parser.VoiceCmdList:
		out, err := h.voiceSvc.List()
		if err != nil {
			log.Printf("voice list error: %v", err)
			h.reply(msg, "Failed to list voice captures.")
			return
		}
		h.reply(msg, out)
	case parser.VoiceCmdDelete:
		out, err := h.voiceSvc.Delete(parsed.Arg)
		if err != nil {
			if errors.Is(err, voice.ErrInvalidVoiceID) {
				h.reply(msg, "Invalid voice ID. Expected format: 00001")
				return
			}
			log.Printf("voice delete error: %v", err)
			h.reply(msg, "Failed to delete voice capture.")
			return
		}
		h.reply(msg, out)
	default:
		h.reply(msg, voiceUsageText)
	}
}

func (h *Handler) handleSync(msg *tgbotapi.Message) {
	out, err := h.executeSync()
	if err != nil {
		log.Printf("sync error: %v", err)
	}
	h.reply(msg, out)
}

func (h *Handler) executeSync() (string, error) {
	outcome, err := h.syncer.Sync()
	if err != nil {
		switch {
		case errors.Is(err, syncgit.ErrFetch):
			return "Sync failed: git fetch error.", err
		case errors.Is(err, syncgit.ErrRebaseConflict):
			return "Sync failed: rebase conflict, aborted. Resolve manually.", err
		case errors.Is(err, syncgit.ErrStashConflict):
			return "Sync failed: stash pop conflict. Resolve manually.", err
		case errors.Is(err, syncgit.ErrAdd):
			return "Sync failed: git add error.", err
		case errors.Is(err, syncgit.ErrCommit):
			return "Sync failed: git commit error.", err
		case errors.Is(err, syncgit.ErrPush):
			return "Sync failed: git push error.", err
		default:
			return fmt.Sprintf("Sync failed: %v", err), err
		}
	}

	if outcome == syncgit.OutcomeNothingToCommit {
		return "Nothing to commit.", nil
	}

	return "Synced to GitHub.", nil
}

func (h *Handler) reply(msg *tgbotapi.Message, text string) {
	if msg == nil {
		return
	}
	h.sendToChat(msg.Chat.ID, text, msg.MessageID)
}

func (h *Handler) sendToChat(chatID int64, text string, replyIDs ...int) {
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
