package note

import (
	"errors"
	"strings"
)

var (
	ErrMissingLink = errors.New("missing link")
	ErrMissingDesc = errors.New("missing description")
)

type ParsedNote struct {
	Link string
	Desc string
}

func Parse(text string) (ParsedNote, error) {
	text = strings.TrimSpace(text)
	tokens := strings.Fields(text)

	if len(tokens) == 0 {
		return ParsedNote{}, ErrMissingLink
	}

	if len(tokens) < 2 {
		return ParsedNote{}, ErrMissingDesc
	}

	return ParsedNote{
		Link: tokens[0],
		Desc: strings.Join(tokens[1:], " "),
	}, nil
}
