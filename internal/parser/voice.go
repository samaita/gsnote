package parser

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidCommand = errors.New("invalid voice command")
	ErrMissingArgs    = errors.New("missing required arguments")
	ErrUnknownSub     = errors.New("unknown subcommand")
)

// VoiceAction represents a parsed voice command.
type VoiceAction int

const (
	VoiceCmdHelp VoiceAction = iota
	VoiceCmdDelete
	VoiceCmdList
)

// ParsedVoice holds the result of parsing a voice command.
type ParsedVoice struct {
	Action VoiceAction
	Arg    string // used by VoiceCmdDelete for the voice ID
}

// ParseVoiceCommand parses the args after "/voice".
// Returns ParsedVoice or an error describing what went wrong.
func ParseVoiceCommand(raw string) (ParsedVoice, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedVoice{}, ErrInvalidCommand
	}

	parts := strings.Fields(raw)
	sub := strings.ToLower(parts[0])

	switch sub {
	case "help":
		return ParsedVoice{Action: VoiceCmdHelp}, nil
	case "list":
		return ParsedVoice{Action: VoiceCmdList}, nil
	case "delete":
		if len(parts) < 2 {
			return ParsedVoice{}, fmt.Errorf("%w: missing voice ID", ErrMissingArgs)
		}
		return ParsedVoice{Action: VoiceCmdDelete, Arg: parts[1]}, nil
	default:
		return ParsedVoice{}, ErrUnknownSub
	}
}
