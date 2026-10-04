package main

import (
	"strings"
	"testing"
)

func TestVersionAndUnknownArgs(t *testing.T) {
	if err := run([]string{"-version"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"worker", "-version"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"bogus"}); err == nil || !strings.Contains(err.Error(), "unknown arguments") {
		t.Fatalf("unknown args: %v", err)
	}
}
