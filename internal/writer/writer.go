package writer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/axonigma/gsnote/internal/parser"
)

// Write appends a habit entry to the appropriate markdown file.
func Write(habitsRoot string, input parser.ParsedInput, now time.Time) error {
	filename := input.Habit + ".md"
	path := filepath.Join(habitsRoot, filename)

	monthSection := "## " + now.Format("2006-01")
	entry := formatEntry(now, input)

	// Read existing content to determine what to prepend
	content, readErr := os.ReadFile(path)
	isNew := os.IsNotExist(readErr)
	if readErr != nil && !isNew {
		return fmt.Errorf("read file: %w", readErr)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if isNew || len(content) == 0 {
		// New file: write header + monthly section + entry
		_, err = fmt.Fprintf(f, "# Habit: %s\n\n%s\n\n%s\n", input.Habit, monthSection, entry)
		return err
	}

	// File exists: check for monthly section
	if !strings.Contains(string(content), monthSection) {
		_, err = fmt.Fprintf(f, "\n%s\n\n%s\n", monthSection, entry)
		return err
	}

	// Monthly section exists: append entry only
	_, err = fmt.Fprintf(f, "%s\n", entry)
	return err
}

// formatEntry builds the markdown entry line.
// Format: - YYYY-MM-DD HH:MM | <value> | <note>  (pipes only when fields present)
func formatEntry(now time.Time, input parser.ParsedInput) string {
	timestamp := now.Format("2006-01-02 15:04")
	parts := []string{"- " + timestamp}

	if input.Value != nil {
		parts = append(parts, formatFloat(*input.Value))
	}

	if input.Note != "" {
		parts = append(parts, input.Note)
	}

	return strings.Join(parts, " | ")
}

// formatFloat strips unnecessary trailing zeros.
// 20.0 → "20", 2.5 → "2.5", 10.7738 → "10.7738"
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
