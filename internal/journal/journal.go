package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const fileExt = ".md"

type Entry struct {
	Summary    string
	Highlights []string
	Blockers   []string
	Closing    string
}

type Manager struct {
	root string
}

func New(root string) *Manager {
	return &Manager{root: root}
}

func (m *Manager) Write(now time.Time, entry Entry) error {
	if err := os.MkdirAll(m.root, 0755); err != nil {
		return err
	}

	path := m.filePath(now)
	_, statErr := os.Stat(path)
	isNew := os.IsNotExist(statErr)
	if statErr != nil && !isNew {
		return statErr
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if !isNew {
		if _, err := fmt.Fprint(f, "\n---\n\n"); err != nil {
			return err
		}
	}

	_, err = fmt.Fprint(f, formatEntry(now, entry, isNew))
	return err
}

func (m *Manager) HasData(now time.Time) bool {
	info, err := os.Stat(m.filePath(now))
	return err == nil && info.Size() > 0
}

func (m *Manager) Clear(now time.Time) error {
	err := os.Remove(m.filePath(now))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (m *Manager) filePath(now time.Time) string {
	return filepath.Join(m.root, now.Format("2006-01-02")+fileExt)
}

func formatEntry(now time.Time, entry Entry, includeTitle bool) string {
	var sb strings.Builder
	if includeTitle {
		sb.WriteString(fmt.Sprintf("# Journal - %s\n\n", now.Format("2006-01-02")))
	}

	sb.WriteString("## 1. Summary\n\n")
	sb.WriteString(strings.TrimSpace(entry.Summary))
	sb.WriteString("\n\n")

	sb.WriteString("## 2. Highlights\n\n")
	writeNumbered(&sb, entry.Highlights)
	sb.WriteString("\n")

	sb.WriteString("## 3. Blockers\n\n")
	writeNumbered(&sb, entry.Blockers)
	sb.WriteString("\n")

	sb.WriteString("## 4. Closing Reflection\n\n")
	sb.WriteString(strings.TrimSpace(entry.Closing))
	sb.WriteString("\n")

	return sb.String()
}

func writeNumbered(sb *strings.Builder, items []string) {
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(item)))
	}
}

type Step int

const (
	StepSummary Step = iota
	StepHighlight
	StepMoreHighlights
	StepBlocker
	StepMoreBlockers
	StepClosing
	StepDone
)

type Session struct {
	step  Step
	entry Entry
}

func NewSession() *Session {
	return &Session{step: StepSummary}
}

func (s *Session) Prompt() string {
	switch s.step {
	case StepSummary:
		return "What happened today?"
	case StepHighlight:
		return "Add a highlight."
	case StepMoreHighlights:
		return "Any more highlights?"
	case StepBlocker:
		return "Add a blocker."
	case StepMoreBlockers:
		return "Any more blockers?"
	case StepClosing:
		return "What should you remember from today?"
	default:
		return "Journal complete."
	}
}

func (s *Session) Step() Step {
	return s.step
}

func (s *Session) AcceptText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	switch s.step {
	case StepSummary:
		s.entry.Summary = text
		s.step = StepHighlight
	case StepHighlight:
		s.entry.Highlights = append(s.entry.Highlights, text)
		s.step = StepMoreHighlights
	case StepBlocker:
		s.entry.Blockers = append(s.entry.Blockers, text)
		s.step = StepMoreBlockers
	case StepClosing:
		s.entry.Closing = text
		s.step = StepDone
		return true
	}
	return false
}

func (s *Session) ChooseMoreHighlights() {
	if s.step == StepMoreHighlights {
		s.step = StepHighlight
	}
}

func (s *Session) ChooseNoMoreHighlights() {
	if s.step == StepMoreHighlights {
		s.step = StepBlocker
	}
}

func (s *Session) ChooseMoreBlockers() {
	if s.step == StepMoreBlockers {
		s.step = StepBlocker
	}
}

func (s *Session) ChooseNoMoreBlockers() {
	if s.step == StepMoreBlockers {
		s.step = StepClosing
	}
}

func (s *Session) Entry() Entry {
	return s.entry
}
