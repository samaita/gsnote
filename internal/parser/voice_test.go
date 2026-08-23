package parser

import (
	"errors"
	"testing"
)

func TestParseVoiceCommandDelete(t *testing.T) {
	got, err := ParseVoiceCommand("delete 00001")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Action != VoiceCmdDelete || got.Arg != "00001" {
		t.Fatalf("unexpected parse: %+v", got)
	}
}

func TestParseVoiceCommandDeleteMissingID(t *testing.T) {
	if _, err := ParseVoiceCommand("delete"); !errors.Is(err, ErrMissingArgs) {
		t.Fatalf("expected ErrMissingArgs, got %v", err)
	}
}

func TestParseVoiceCommandList(t *testing.T) {
	got, err := ParseVoiceCommand("list")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Action != VoiceCmdList {
		t.Fatalf("unexpected action: %+v", got)
	}
}

func TestParseVoiceCommandHelp(t *testing.T) {
	got, err := ParseVoiceCommand("help")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Action != VoiceCmdHelp {
		t.Fatalf("unexpected action: %+v", got)
	}
}

func TestParseVoiceCommandEmpty(t *testing.T) {
	if _, err := ParseVoiceCommand(""); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("expected ErrInvalidCommand, got %v", err)
	}
	if _, err := ParseVoiceCommand("   "); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("expected ErrInvalidCommand, got %v", err)
	}
}

func TestParseVoiceCommandUnknown(t *testing.T) {
	if _, err := ParseVoiceCommand("purge"); !errors.Is(err, ErrUnknownSub) {
		t.Fatalf("expected ErrUnknownSub, got %v", err)
	}
}
