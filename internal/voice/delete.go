package voice

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DeleteByID removes all files belonging to a voice capture identified by its ID.
// Only deletes files whose basename starts with the validated ID.
func DeleteByID(voicesRoot, voiceID string) (string, error) {
	id := strings.TrimSpace(voiceID)
	if id == "" {
		return "", fmt.Errorf("usage: /voice delete <id>")
	}

	entries, err := os.ReadDir(voicesRoot)
	if err != nil {
		return "", fmt.Errorf("read voices dir: %w", err)
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), id+"-") {
			matches = append(matches, e.Name())
		}
	}

	if len(matches) == 0 {
		return "Voice " + id + " not found.", nil
	}
	if len(matches) > 1 {
		return "Unable to delete voice " + id + ": multiple matching files found.", nil
	}

	matchedName := matches[0]
	mdPath := filepath.Join(voicesRoot, matchedName)

	log.Printf("deleting voice files: %s, md=%s", filepath.Join(voicesRoot, matchedName), mdPath)

	os.Remove(filepath.Join(voicesRoot, matchedName))
	os.Remove(mdPath)

	return "Deleted voice " + id + ".", nil
}
