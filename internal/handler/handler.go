package handler

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/idea"
	"github.com/axonigma/gsnote/internal/parser"
	"github.com/axonigma/gsnote/internal/task"
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

const taskHelpText = `Available task commands:
  /task new     — add a task
  /task view    — view tasks
  /task done    — mark a task complete
  /task edit    — edit a task
  /task delete  — delete a task`

const taskNewUsageText = `Usage:
  /task new <text>
  /task new YYYY-MM-DD <text>

  text       — task description (required)
  YYYY-MM-DD — schedule for a date (default: today)

Examples:
  /task new Buy groceries
  /task new 2026-04-21 Submit report`

const taskViewUsageText = `Usage:
  /task view
  /task view YYYY-MM-DD

  YYYY-MM-DD — date to view (default: today)

Examples:
  /task view
  /task view 2026-04-21`

const taskDoneUsageText = `Usage:
  /task done N
  /task done YYYY-MM-DD N

  N          — task number shown in /task view
  YYYY-MM-DD — date of the task (default: today)

Examples:
  /task done 2
  /task done 2026-04-21 2`

const taskEditUsageText = `Usage:
  /task edit N <text>
  /task edit YYYY-MM-DD N <text>

  N          — task number shown in /task view
  text       — replacement text
  YYYY-MM-DD — date of the task (default: today)

Examples:
  /task edit 2 Buy more groceries
  /task edit 2026-04-21 2 Submit final report`

const taskDeleteUsageText = `Usage:
  /task delete N
  /task delete YYYY-MM-DD N

  N          — task number shown in /task view
  YYYY-MM-DD — date of the task (default: today)

Examples:
  /task delete 2
  /task delete 2026-04-21 2`

const ideaUsageText = `Usage:
  /idea <type> <title>

  type  — one of: pain, insight, content (required)
  title — free text describing the idea (required)

Examples:
  /idea pain address invalid bikin retry cost tinggi
  /idea insight validator harus shift-left ke input
  /idea content why failed delivery is preventable not operational`

const helpText = `Available commands:
  /habit — log or list habits
  /task  — manage daily tasks
  /idea  — capture an idea (pain, insight, or content)
  /sync  — git add, commit, and push to origin main

Tip: type a command alone to see its full usage.`

const (
	cmdHabit = "/habit"
	cmdTask  = "/task"
	cmdIdea  = "/idea"
	cmdSync  = "/sync"
	cmdHelp  = "/help"
)

const habitCmdList = "list"

const (
	taskCmdNew    = "new"
	taskCmdView   = "view"
	taskCmdDone   = "done"
	taskCmdEdit   = "edit"
	taskCmdDelete = "delete"
)

const warnText = `Command not found, use /help for guide`

// Handler holds dependencies for command handling.
type Handler struct {
	bot                 *tgbotapi.BotAPI
	habitsRoot          string
	syncRoot            string
	ideasRoot           string
	taskMgr             *task.Manager
	whitelistTelegramID map[int64]bool
}

func New(bot *tgbotapi.BotAPI, habitsRoot, syncRoot, tasksRoot, ideasRoot string, whitelistTelegramID map[int64]bool) *Handler {
	return &Handler{
		bot:                 bot,
		habitsRoot:          habitsRoot,
		syncRoot:            syncRoot,
		ideasRoot:           ideasRoot,
		taskMgr:             task.New(tasksRoot),
		whitelistTelegramID: whitelistTelegramID,
	}
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
	case strings.HasPrefix(text, cmdTask):
		h.handleTask(msg, strings.TrimPrefix(text, cmdTask))
	case strings.HasPrefix(text, cmdIdea):
		h.handleIdea(msg, strings.TrimPrefix(text, cmdIdea))
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

func (h *Handler) handleIdea(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		h.reply(msg, ideaUsageText)
		return
	}

	input, err := idea.Parse(args)
	if err != nil {
		h.reply(msg, "Please re-enter with the correct format. Use /help.")
		return
	}

	if err := idea.Write(h.ideasRoot, input, time.Now()); err != nil {
		log.Printf("write error for idea %q: %v", input.Title, err)
		h.reply(msg, "Try again with the correct command. Use /help.")
		return
	}

	h.reply(msg, fmt.Sprintf("Idea captured: %s", input.Title))
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

func (h *Handler) handleTask(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		h.reply(msg, taskHelpText)
		return
	}

	parts := strings.Fields(args)
	sub := parts[0]
	rest := parts[1:]
	today := time.Now().Format("2006-01-02")

	switch sub {
	case taskCmdNew:
		if len(rest) == 0 {
			h.reply(msg, taskNewUsageText)
			return
		}
		date, text, ok := parseTaskNewArgs(rest, today)
		if !ok || text == "" {
			h.reply(msg, taskNewUsageText)
			return
		}
		if err := h.taskMgr.Add(date, text); err != nil {
			log.Printf("task add error: %v", err)
			h.reply(msg, "Failed to add task.")
			return
		}
		h.reply(msg, fmt.Sprintf("Task added for %s.", date))

	case taskCmdView:
		date := today
		if len(rest) > 0 && isDateArg(rest[0]) {
			date = rest[0]
		}
		out, err := h.taskMgr.View(date)
		if err != nil {
			log.Printf("task view error: %v", err)
			h.reply(msg, "Failed to view tasks.")
			return
		}
		h.reply(msg, out)

	case taskCmdDone:
		if len(rest) == 0 {
			h.reply(msg, taskDoneUsageText)
			return
		}
		date, n, ok := parseTaskNArgs(rest, today)
		if !ok {
			h.reply(msg, taskDoneUsageText)
			return
		}
		if err := h.taskMgr.Done(date, n); err != nil {
			log.Printf("task done error: %v", err)
			h.reply(msg, fmt.Sprintf("Could not update task: %v", err))
			return
		}
		h.reply(msg, fmt.Sprintf("Task %d toggled.", n))

	case taskCmdEdit:
		if len(rest) == 0 {
			h.reply(msg, taskEditUsageText)
			return
		}
		date, n, text, ok := parseTaskEditArgs(rest, today)
		if !ok || text == "" {
			h.reply(msg, taskEditUsageText)
			return
		}
		if err := h.taskMgr.Edit(date, n, text); err != nil {
			log.Printf("task edit error: %v", err)
			h.reply(msg, fmt.Sprintf("Could not edit task: %v", err))
			return
		}
		h.reply(msg, fmt.Sprintf("Task %d updated.", n))

	case taskCmdDelete:
		if len(rest) == 0 {
			h.reply(msg, taskDeleteUsageText)
			return
		}
		date, n, ok := parseTaskNArgs(rest, today)
		if !ok {
			h.reply(msg, taskDeleteUsageText)
			return
		}
		if err := h.taskMgr.Delete(date, n); err != nil {
			log.Printf("task delete error: %v", err)
			h.reply(msg, fmt.Sprintf("Could not delete task: %v", err))
			return
		}
		h.reply(msg, fmt.Sprintf("Task %d deleted.", n))

	default:
		h.reply(msg, taskHelpText)
	}
}

func isDateArg(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// parseTaskNewArgs parses: [YYYY-MM-DD] <text...>
func parseTaskNewArgs(parts []string, defaultDate string) (date, text string, ok bool) {
	if len(parts) == 0 {
		return "", "", false
	}
	if isDateArg(parts[0]) {
		if len(parts) < 2 {
			return "", "", false
		}
		return parts[0], strings.Join(parts[1:], " "), true
	}
	return defaultDate, strings.Join(parts, " "), true
}

// parseTaskNArgs parses: [YYYY-MM-DD] N
func parseTaskNArgs(parts []string, defaultDate string) (date string, n int, ok bool) {
	switch len(parts) {
	case 1:
		n64, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || n64 < 1 {
			return "", 0, false
		}
		return defaultDate, int(n64), true
	case 2:
		if !isDateArg(parts[0]) {
			return "", 0, false
		}
		n64, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || n64 < 1 {
			return "", 0, false
		}
		return parts[0], int(n64), true
	default:
		return "", 0, false
	}
}

// parseTaskEditArgs parses: [YYYY-MM-DD] N <text...>
func parseTaskEditArgs(parts []string, defaultDate string) (date string, n int, text string, ok bool) {
	if len(parts) < 2 {
		return "", 0, "", false
	}
	if isDateArg(parts[0]) {
		if len(parts) < 3 {
			return "", 0, "", false
		}
		n64, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || n64 < 1 {
			return "", 0, "", false
		}
		return parts[0], int(n64), strings.Join(parts[2:], " "), true
	}
	n64, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || n64 < 1 {
		return "", 0, "", false
	}
	return defaultDate, int(n64), strings.Join(parts[1:], " "), true
}

func (h *Handler) reply(msg *tgbotapi.Message, text string) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	if _, err := h.bot.Send(reply); err != nil {
		log.Printf("send reply error: %v", err)
	}
}
