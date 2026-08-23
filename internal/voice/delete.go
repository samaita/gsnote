package voice

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrInvalidVoiceID is returned when the requested ID does not match the
	// expected voice ID format (e.g. "00001").
	ErrInvalidVoiceID = errors.New("invalid voice ID")

	voiceIDPattern = regexp.MustCompile(`^\d{5}$`)
)

// ValidVoiceID reports whether id matches the expected voice ID format.
// Only zero-padded 5-digit numeric IDs are accepted, which rejects path
// separators, relative/absolute paths, and other unexpected characters.
func ValidVoiceID(id string) bool {
	return voiceIDPattern.MatchString(id)
}

// DeleteByID removes all files belonging to a voice capture identified by its ID.
// Files are matched by basename prefix "<id>-" (audio and note markdown) or
// "<id>." (raw transcript markdown) so the whole capture is removed. The ID is
// validated before any filesystem access. A capture normally consists of three
// files; more matches are treated as ambiguous and nothing is deleted.
func DeleteByID(voicesRoot, voiceID string) (string, error) {
	id := strings.TrimSpace(voiceID)
	if !ValidVoiceID(id) {
		return "", fmt.Errorf("%w: expected format 00001", ErrInvalidVoiceID)
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
		if strings.HasPrefix(e.Name(), id+"-") || strings.HasPrefix(e.Name(), id+".") {
			matches = append(matches, e.Name())
		}
	}

	if len(matches) == 0 {
		return "Voice " + id + " not found.", nil
	}
	if len(matches) > 3 {
		return "Unable to delete voice " + id + ": multiple matching files found.", nil
	}

	for _, name := range matches {
		log.Printf("deleting voice file: %s", filepath.Join(voicesRoot, name))
		os.Remove(filepath.Join(voicesRoot, name))
	}

	return "Deleted voice " + id + ".", nil
}
