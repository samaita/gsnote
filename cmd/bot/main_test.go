package main

import (
	"context"
	"strings"
	"testing"

	"github.com/axonigma/gsnote/internal/jobs"
)

func TestVersionAndUnknownArgs(t *testing.T) {
	if err := run([]string{"-version"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"worker", "-version"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown arguments") {
		t.Fatalf("unknown args error = %v", err)
	}
}

func TestWorkerStartsWithBot(t *testing.T) {
	if err := runWorker(nil, "", context.Background(), nil); err == nil {
		t.Fatal("expected invalid worker setup")
	}
}
func TestParseMaxAttempts(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  int
		valid bool
	}{{"5", 5, true}, {"1", 1, true}, {"0", 0, false}, {"abc", 0, false}, {"", jobs.DefaultMaxAttempts, true}} {
		got, err := parseMaxAttempts(tt.value)
		if tt.valid && (err != nil || got != tt.want) {
			t.Fatalf("%q got=%d err=%v", tt.value, got, err)
		}
		if !tt.valid && err == nil {
			t.Fatalf("%q should be rejected", tt.value)
		}
	}
}
