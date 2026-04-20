package handler

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/parser"
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

const helpText = `Available commands:
  /habit — log or list habits
  /sync  — git add, commit, and push to origin main

Tip: type a command alone to see its full usage.`

const (
	cmdHabit = "/habit"
	cmdSync  = "/sync"
	cmdHelp  = "/help"
)

const habitCmdList = "list"

const warnText = `Command not found, use /help for guide`

// Handler holds dependencies for command handling.
type Handler struct {
	bot                 *tgbotapi.BotAPI
	habitsRoot          string
	syncRoot            string
	whitelistTelegramID map[int64]bool
}

func New(bot *tgbotapi.BotAPI, habitsRoot, syncRoot string, whitelistTelegramID map[int64]bool) *Handler {
	return &Handler{bot: bot, habitsRoot: habitsRoot, syncRoot: syncRoot, whitelistTelegramID: whitelistTelegramID}
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
	case strings.HasPrefix(text, cmdHabit):
		h.handleHabit(msg, strings.TrimPrefix(text, cmdHabit))
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

func (h *Handler) handleSync(msg *tgbotapi.Message) {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", h.syncRoot}, args...)...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	stashOut, _ := run("stash")
	stashed := !strings.Contains(stashOut, "No local changes to stash")

	if _, err := run("fetch", "origin"); err != nil {
		log.Printf("sync git fetch error: %v", err)
		if stashed {
			run("stash", "pop")
		}
		h.reply(msg, "Sync failed: git fetch error.")
		return
	}

	if rebaseOut, err := run("rebase", "origin/main"); err != nil {
		log.Printf("sync git rebase error: %v — %s", err, rebaseOut)
		run("rebase", "--abort")
		if stashed {
			run("stash", "pop")
		}
		h.reply(msg, "Sync failed: rebase conflict, aborted. Resolve manually.")
		return
	}

	if stashed {
		if popOut, err := run("stash", "pop"); err != nil {
			log.Printf("sync git stash pop error: %v — %s", err, popOut)
			h.reply(msg, "Sync failed: stash pop conflict. Resolve manually.")
			return
		}
	}

	if _, err := run("add", "."); err != nil {
		log.Printf("sync git add error: %v", err)
		h.reply(msg, "Sync failed: git add error.")
		return
	}

	commitOut, err := run("commit", "-m", "Sync from telegram")
	if err != nil {
		if strings.Contains(commitOut, "nothing to commit") {
			h.reply(msg, "Nothing to commit.")
			return
		}
		log.Printf("sync git commit error: %v — %s", err, commitOut)
		h.reply(msg, "Sync failed: git commit error.")
		return
	}

	if _, err := run("push", "origin", "main"); err != nil {
		log.Printf("sync git push error: %v", err)
		h.reply(msg, "Sync failed: git push error.")
		return
	}

	h.reply(msg, "Synced to GitHub.")
}

func (h *Handler) reply(msg *tgbotapi.Message, text string) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := h.bot.Send(reply); err != nil {
		log.Printf("send reply error: %v", err)
	}
}
