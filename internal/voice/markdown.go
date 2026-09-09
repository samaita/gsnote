package voice

import (
	"fmt"
	"os"
	"time"
)

// VoiceMetadata is the data written to the transcript note.
type VoiceMetadata struct {
	ID         string
	Date       time.Time
	Transcript string
	Audio      string // audio filename relative to the gsnote root
}

// WriteMarkdown writes the voice note as a markdown file: frontmatter with
// the capture identity plus the verbatim transcript as the body.
func WriteMarkdown(path string, meta VoiceMetadata) error {
	content := fmt.Sprintf(`---
id: "%s"
date: %s
source: telegram-voice
audio: %s
---

%s
`, meta.ID, meta.Date.Format("2006-01-02 15:04"), meta.Audio, meta.Transcript)
	return os.WriteFile(path, []byte(content), 0644)
}

// DefaultMDFilename generates a markdown filename for a voice capture.
func DefaultMDFilename(voiceID string, date time.Time) string {
	return fmt.Sprintf("%s-%s.md", voiceID, date.Format("20060102"))
}
