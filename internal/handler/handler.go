package handler

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/axonigma/gsnote/internal/cron"
	"github.com/axonigma/gsnote/internal/idea"
	"github.com/axonigma/gsnote/internal/note"
	"github.com/axonigma/gsnote/internal/parser"
	"github.com/axonigma/gsnote/internal/syncgit"
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

const noteUsageText = `Usage:
  /note <link> <desc>

  link — URL of the resource (required)
  desc — short description (required)

Examples:
  /note https://www.google.com a search engine made by Google`

const cronHelpText = `Available cron commands:
  /cron <HH:MM> <command>       — run a command daily
  /cron <cron spec> <command>  — run a command on a cron schedule
  /cron view                   — view scheduled commands
  /cron edit N <spec> <command>   — replace a scheduled command
  /cron delete N                  — delete a scheduled command

Supported commands:
  /task view
  /sync

Examples:
  /cron 06:00 /task view
  /cron */5 * * * * /sync
  /cron edit 2 07:30 /task view
  /cron delete 2`

const helpText = `Available commands:
  /habit — log or list habits
  /task  — manage daily tasks
  /idea  — capture an idea (pain, insight, or content)
  /note  — save a link with your take
  /cron  — schedule /task view or /sync
  /sync  — git add, commit, and push to origin main

Tip: type a command alone to see its full usage.`

const (
	cmdHabit = "/habit"
	cmdTask  = "/task"
	cmdIdea  = "/idea"
	cmdNote  = "/note"
	cmdCron  = "/cron"
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
	notesRoot           string
	syncer              *syncgit.Service
	taskMgr             *task.Manager
	cronStore           *cron.Store
	whitelistTelegramID map[int64]bool
	pendingNotes        map[int64]string // userID → note file path awaiting "My Take"
}

func New(bot *tgbotapi.BotAPI, habitsRoot, syncRoot, tasksRoot, ideasRoot, notesRoot, cronRoot, githubToken, gitAuthorName, gitAuthorEmail string, whitelistTelegramID map[int64]bool) *Handler {
	return &Handler{
		bot:                 bot,
		habitsRoot:          habitsRoot,
		syncRoot:            syncRoot,
		ideasRoot:           ideasRoot,
		notesRoot:           notesRoot,
		syncer:              syncgit.New(syncRoot, githubToken, gitAuthorName, gitAuthorEmail),
		taskMgr:             task.New(tasksRoot),
		cronStore:           cron.NewStore(cronRoot),
		whitelistTelegramID: whitelistTelegramID,
		pendingNotes:        make(map[int64]string),
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

	if !strings.HasPrefix(text, "/") {
		if filePath, ok := h.pendingNotes[msg.From.ID]; ok {
			delete(h.pendingNotes, msg.From.ID)
			if err := note.FillMyTake(filePath, text); err != nil {
				log.Printf("fill my take error: %v", err)
				h.reply(msg, "Failed to save your take.")
				return
			}
			h.reply(msg, "Note saved!")
			return
		}
	}

	switch {
	case strings.HasPrefix(text, cmdHabit):
		h.handleHabit(msg, strings.TrimPrefix(text, cmdHabit))
	case strings.HasPrefix(text, cmdTask):
		h.handleTask(msg, strings.TrimPrefix(text, cmdTask))
	case strings.HasPrefix(text, cmdIdea):
		h.handleIdea(msg, strings.TrimPrefix(text, cmdIdea))
	case strings.HasPrefix(text, cmdNote):
		h.handleNote(msg, strings.TrimPrefix(text, cmdNote))
	case strings.HasPrefix(text, cmdCron):
		h.handleCron(msg, strings.TrimPrefix(text, cmdCron))
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

func (h *Handler) handleNote(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		h.reply(msg, noteUsageText)
		return
	}

	input, err := note.Parse(args)
	if err != nil {
		h.reply(msg, "Please re-enter with the correct format. Use /help.")
		return
	}

	filePath, err := note.Write(h.notesRoot, input, time.Now())
	if err != nil {
		log.Printf("write error for note %q: %v", input.Desc, err)
		h.reply(msg, "Try again with the correct command. Use /help.")
		return
	}

	h.pendingNotes[msg.From.ID] = filePath
	h.reply(msg, "What is your take?")
}

func (h *Handler) handleSync(msg *tgbotapi.Message) {
	out, err := h.executeSync()
	if err != nil {
		log.Printf("sync error: %v", err)
	}
	h.reply(msg, out)
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
		out, err := h.executeTaskView(date)
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

func (h *Handler) handleCron(msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	if args == "" {
		h.reply(msg, cronHelpText)
		return
	}

	if args == "view" {
		h.handleCronView(msg)
		return
	}

	if strings.HasPrefix(args, "delete ") {
		h.handleCronDelete(msg, strings.TrimSpace(strings.TrimPrefix(args, "delete ")))
		return
	}

	if strings.HasPrefix(args, "edit ") {
		h.handleCronEdit(msg, strings.TrimSpace(strings.TrimPrefix(args, "edit ")))
		return
	}

	spec, cmd, err := parseCronSpecAndCommand(args)
	if err != nil {
		h.reply(msg, cronHelpText)
		return
	}

	entry, err := h.cronStore.Add(spec, cmd, msg.Chat.ID)
	if err != nil {
		h.reply(msg, formatCronError(err))
		return
	}

	h.reply(msg, fmt.Sprintf("Cron added: %s -> %s", entry.Spec, entry.Cmd))
}

func (h *Handler) handleCronView(msg *tgbotapi.Message) {
	entries, err := h.cronStore.List()
	if err != nil {
		log.Printf("cron view error: %v", err)
		h.reply(msg, "Failed to view cron entries.")
		return
	}
	if len(entries) == 0 {
		h.reply(msg, "No cron entries.")
		return
	}

	var sb strings.Builder
	sb.WriteString("Cron entries:\n")
	for i, entry := range entries {
		sb.WriteString(fmt.Sprintf("%d. %s -> %s\n", i+1, entry.Spec, entry.Cmd))
	}
	h.reply(msg, strings.TrimRight(sb.String(), "\n"))
}

func (h *Handler) handleCronEdit(msg *tgbotapi.Message, args string) {
	line, spec, cmd, err := parseCronEditArgs(args)
	if err != nil {
		h.reply(msg, cronHelpText)
		return
	}

	entry, err := h.cronStore.Edit(line, spec, cmd)
	if err != nil {
		h.reply(msg, formatCronError(err))
		return
	}

	h.reply(msg, fmt.Sprintf("Cron %d updated: %s -> %s", line, entry.Spec, entry.Cmd))
}

func (h *Handler) handleCronDelete(msg *tgbotapi.Message, args string) {
	line, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil || line < 1 {
		h.reply(msg, cronHelpText)
		return
	}

	entry, err := h.cronStore.Delete(line)
	if err != nil {
		h.reply(msg, formatCronError(err))
		return
	}

	h.reply(msg, fmt.Sprintf("Cron %d deleted: %s -> %s", line, entry.Spec, entry.Cmd))
}

func (h *Handler) StartCronScheduler() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var lastMinute time.Time
		for range ticker.C {
			now := time.Now().In(time.Local).Truncate(time.Minute)
			if now.Equal(lastMinute) {
				continue
			}
			lastMinute = now

			entries, err := h.cronStore.List()
			if err != nil {
				log.Printf("cron scheduler list error: %v", err)
				continue
			}

			for _, entry := range entries {
				match, err := cron.Matches(entry.Spec, now)
				if err != nil {
					log.Printf("cron scheduler match error spec=%q: %v", entry.Spec, err)
					continue
				}
				if !match {
					continue
				}

				out, err := h.executeScheduledCommand(entry.Cmd)
				if err != nil {
					log.Printf("cron scheduler execute error cmd=%q: %v", entry.Cmd, err)
				}
				h.sendToChat(entry.ChatID, out)
			}
		}
	}()
}

func (h *Handler) executeScheduledCommand(cmd string) (string, error) {
	switch cmd {
	case "/task view":
		return h.executeTaskView(time.Now().In(time.Local).Format("2006-01-02"))
	case "/sync":
		return h.executeSync()
	default:
		return "", cron.ErrUnsupportedCmd
	}
}

func (h *Handler) executeTaskView(date string) (string, error) {
	return h.taskMgr.View(date)
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

func parseCronEditArgs(args string) (line int, spec, cmd string, err error) {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return 0, "", "", cron.ErrInvalidLine
	}

	line, err = strconv.Atoi(parts[0])
	if err != nil || line < 1 {
		return 0, "", "", cron.ErrInvalidLine
	}

	spec, cmd, err = parseCronSpecAndCommand(strings.TrimSpace(strings.TrimPrefix(args, parts[0])))
	if err != nil {
		return 0, "", "", err
	}
	return line, spec, cmd, nil
}

func parseCronSpecAndCommand(args string) (spec, cmd string, err error) {
	args = strings.TrimSpace(args)
	commands := []string{"/task view", "/sync"}
	slices.SortFunc(commands, func(a, b string) int {
		return len(b) - len(a)
	})

	for _, candidate := range commands {
		if !strings.HasSuffix(args, candidate) {
			continue
		}
		spec = strings.TrimSpace(strings.TrimSuffix(args, candidate))
		if spec == "" {
			continue
		}
		if _, err := cron.NormalizeSpec(spec); err != nil {
			return "", "", err
		}
		return spec, candidate, nil
	}

	return "", "", cron.ErrUnsupportedCmd
}

func formatCronError(err error) string {
	switch {
	case errors.Is(err, cron.ErrUnsupportedCmd):
		return "Unsupported cron command. Allowed: /task view, /sync."
	case errors.Is(err, cron.ErrEntryNotFound):
		return "Cron entry not found."
	case errors.Is(err, cron.ErrMultipleFiles):
		return "Cron storage must contain only cron.txt."
	case errors.Is(err, cron.ErrInvalidSpec), errors.Is(err, cron.ErrInvalidLine):
		return cronHelpText
	default:
		return "Failed to update cron entries."
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
