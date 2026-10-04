package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranscriptMarkdownTitleAndVerbatimBody(t *testing.T) {
	name, title, body := TranscriptMarkdown("VN-1", "Inbox/Voices/VN-1.ogg", time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), "Hello world from local English model extra words")
	if name != "Hello world from local English" || title != name {
		t.Fatalf("name/title = %q/%q", name, title)
	}
	if !strings.HasSuffix(body, "Hello world from local English model extra words\n") {
		t.Fatalf("transcript changed: %q", body)
	}
	if !strings.Contains(body, "Audio: Inbox/Voices/VN-1.ogg") {
		t.Fatalf("audio link missing: %q", body)
	}
}

func TestTranscriptMarkdownFallbackSanitizationAndUnicode(t *testing.T) {
	cases := []struct{ input, want string }{
		{"", "VN-2"}, {" / \\\n", "VN-2"}, {"Halo, dunia! ini/penting dan aman", "Halo dunia ini penting dan aman"}, {"Penting—sekali café pagi ini", "Penting—sekali café pagi ini"},
	}
	for _, tc := range cases {
		_, got, _ := TranscriptMarkdown("VN-2", "voice.ogg", time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), tc.input)
		if got != tc.want {
			t.Errorf("title(%q)=%q want %q", tc.input, got, tc.want)
		}
	}
}

func TestSaveTranscriptNoClobberCollision(t *testing.T) {
	dir := t.TempDir()
	filename, title, body := TranscriptMarkdown("VN-3", "Inbox/Voices/VN-3.ogg", time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), "Same title here")
	path := filepath.Join(dir, "2026-10-04 - "+filename+".md")
	if err := os.WriteFile(path, []byte("existing note"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := SaveTranscript(dir, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), filename, "VN-3", body)
	if err != nil {
		t.Fatal(err)
	}
	filename = "2026-10-04 - " + filename + ".md"
	if first != filepath.Join(dir, "2026-10-04 - "+title+" (VN-3).md") {
		t.Fatalf("collision path=%q", first)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "existing note" {
		t.Fatalf("existing note overwritten: %q", got)
	}
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), title) || !strings.Contains(string(data), "Same title here") {
		t.Fatalf("bad new note %q", data)
	}
}
