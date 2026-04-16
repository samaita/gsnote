package parser

import (
	"errors"
	"strconv"
	"strings"
)

var ErrMissingHabit = errors.New("missing habit name")

type ParsedInput struct {
	Habit string
	Value *float64
	Note  string
}

// Parse parses the text after the /habit command.
// Format: <habit> [value] [note]
func Parse(text string) (ParsedInput, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedInput{}, ErrMissingHabit
	}

	tokens := strings.Fields(text)

	habit := strings.ToLower(tokens[0])
	var value *float64
	var noteParts []string

	if len(tokens) > 1 {
		if f, err := strconv.ParseFloat(tokens[1], 64); err == nil {
			value = &f
			noteParts = tokens[2:]
		} else {
			noteParts = tokens[1:]
		}
	}

	return ParsedInput{
		Habit: habit,
		Value: value,
		Note:  strings.Join(noteParts, " "),
	}, nil
}
