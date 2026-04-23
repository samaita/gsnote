package idea

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9 ]+`)

func Write(ideasRoot string, input ParsedIdea, now time.Time) error {
	filename := now.Format("2006-01-02") + "-" + toSnakeCase(input.Title) + ".md"
	path := filepath.Join(ideasRoot, filename)

	content := fmt.Sprintf("type: %s\nstatus: **raw**\ntags: []\n\n---\n\n# %s\n## Problem\n\n## Insight\n\n## Why it matters\n",
		input.Type, toTitleCase(input.Title))

	return os.WriteFile(path, []byte(content), 0644)
}

func toSnakeCase(s string) string {
	s = nonAlphanumeric.ReplaceAllString(strings.ToLower(s), "")
	s = strings.ReplaceAll(s, " ", "_")
	return strings.Trim(s, "_")
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}
