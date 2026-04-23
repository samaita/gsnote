package idea

import (
	"errors"
	"strings"
)

var (
	ErrMissingType  = errors.New("missing idea type")
	ErrInvalidType  = errors.New("invalid idea type: must be pain, insight, or content")
	ErrMissingTitle = errors.New("missing idea title")
)

var validTypes = map[string]bool{
	"pain":    true,
	"insight": true,
	"content": true,
}

type ParsedIdea struct {
	Type  string
	Title string
}

func Parse(text string) (ParsedIdea, error) {
	text = strings.TrimSpace(text)
	tokens := strings.Fields(text)

	if len(tokens) == 0 {
		return ParsedIdea{}, ErrMissingType
	}

	ideaType := strings.ToLower(tokens[0])
	if !validTypes[ideaType] {
		return ParsedIdea{}, ErrInvalidType
	}

	if len(tokens) < 2 {
		return ParsedIdea{}, ErrMissingTitle
	}

	return ParsedIdea{
		Type:  ideaType,
		Title: strings.Join(tokens[1:], " "),
	}, nil
}
