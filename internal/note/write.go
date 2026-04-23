package note

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

func Write(notesRoot string, input ParsedNote, now time.Time) (string, error) {
	filename := now.Format("2006-01-02") + "-" + toSnakeCase(input.Desc) + ".md"
	path := filepath.Join(notesRoot, filename)

	content := fmt.Sprintf("# %s\nLink: %s\n\n## My Take\n...\n", toTitleCase(input.Desc), input.Link)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func FillMyTake(path, take string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated := strings.Replace(string(data), "...", take, 1)
	return os.WriteFile(path, []byte(updated), 0644)
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
