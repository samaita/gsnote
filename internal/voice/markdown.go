package voice

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// VoiceMetadata is the data written to the markdown file.
type VoiceMetadata struct {
	ID         string
	Date       time.Time
	Title      string
	Summary    string
	Content    string
	Transcript string
	VoiceType  string
	Category   string
	Project    string
	Tags       []string
	Audio      string
}

// WriteMarkdown writes the voice note as a markdown file.
func WriteMarkdown(path string, meta VoiceMetadata) error {
	tagsLines := ""
	for _, tag := range meta.Tags {
		tagsLines += "      - " + tag + "\n"
	}

	content := fmt.Sprintf(`---
id: "%s"
date: %s
title: "%s"
type: %s
category: %s
project: %s
tags:
%ssource: telegram-voice
audio: %s
---

# %s

## Summary

%s

## Transcript

%s
`, meta.ID, meta.Date.Format("2006-01-02"), meta.Title, meta.VoiceType, meta.Category, meta.Project, strings.TrimSpace(tagsLines), meta.Audio, meta.Title, meta.Summary, meta.Transcript)

	return os.WriteFile(path, []byte(content), 0644)
}

// DefaultMDFilename generates a markdown filename for a voice capture.
func DefaultMDFilename(voiceID string, date time.Time) string {
	return fmt.Sprintf("%s-%s.md", voiceID, date.Format("20060102"))
}
