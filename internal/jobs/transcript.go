package jobs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxTitleRunes = 64

// TranscriptMarkdown returns the collision-safe dated Inbox note name and content.
func TranscriptMarkdown(id, audioPath string, date time.Time, transcript string) (filename, title, body string) {
	title = transcriptTitle(transcript, id)
	filename = title
	if utf8.RuneCountInString(filename) > maxTitleRunes {
		filename = string([]rune(filename)[:maxTitleRunes])
	}
	body = fmt.Sprintf("## %s\n\nDate: %s\nSource: gsnote voice %s\nAudio: %s\n\n%s\n", title, date.Format("2006-01-02 15:04"), id, filepath.ToSlash(audioPath), transcript)
	return filename, title, body
}

func transcriptTitle(text, fallback string) string {
	fields := strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) })
	if len(fields) == 0 {
		return fallback
	}
	if len(fields) > 5 {
		fields = fields[:5]
	}
	for i, word := range fields {
		word = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || r == '/' || r == '\\' {
				return ' '
			}
			return r
		}, word)
		word = strings.TrimRightFunc(word, func(r rune) bool { return unicode.IsPunct(r) })
		fields[i] = word
	}
	title := strings.TrimSpace(strings.Join(fields, " "))
	if title == "" {
		return fallback
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		title = string([]rune(title)[:maxTitleRunes])
		title = strings.TrimSpace(title)
	}
	return title
}
