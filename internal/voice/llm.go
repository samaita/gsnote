package voice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// NoteLLM processes a transcript into structured note information.
type NoteLLM interface {
	Process(transcript string) (*VoiceInfo, error)
}

// OpenAILLM processes a transcript through an OpenAI-compatible chat model.
type OpenAILLM struct {
	APIKey  string
	BaseURL string
	Model   string
}

// VoiceInfo contains the structured result from the LLM.
type VoiceInfo struct {
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Content  string   `json:"content"`
	Type     string   `json:"type"`
	Category string   `json:"category"`
	Project  string   `json:"project"`
	Tags     []string `json:"tags"`
}

const (
	systemPrompt = `You are a note-processing assistant. You receive raw transcripts from voice messages — spoken Indonesian/English that may contain filler words, repetitions, incomplete sentences, slang, technical terms, and STT errors.

Your job:
- Understand the intended meaning despite speech imperfections
- Remove filler words and unnecessary repetition
- Preserve meaning, technical terminology, uncertainty
- Summarize accurately
- Classify the note (type must be one of: idea, task, insight, pain, note)
- Determine semantic category (e.g. address-quality, obsidian-sync, project)
- Determine semantic project (same as category)
- Generate useful tags
- NEVER invent facts
- NEVER generate filesystem paths or directory names
- Keep output concise and natural`
)

// Process sends the transcript to the LLM and returns structured voice info.
func (l *OpenAILLM) Process(transcript string) (*VoiceInfo, error) {
	payload := map[string]interface{}{
		"model": l.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": transcript},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := l.BaseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+l.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(apiResp.Choices) == 0 || apiResp.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("empty LLM response")
	}

	var info VoiceInfo
	if err := json.Unmarshal([]byte(apiResp.Choices[0].Message.Content), &info); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w", err)
	}
	return &info, nil
}

var validVoiceTypes = map[string]bool{
	"idea":    true,
	"task":    true,
	"insight": true,
	"pain":    true,
	"note":    true,
}

// validateVoiceInfo checks that the LLM returned a usable, safe result.
// The LLM must never control physical file locations, so category/project
// and tags are rejected if they contain path separators.
func validateVoiceInfo(info *VoiceInfo) error {
	if info == nil {
		return fmt.Errorf("nil LLM result")
	}
	if strings.TrimSpace(info.Title) == "" {
		return fmt.Errorf("missing title")
	}
	if strings.TrimSpace(info.Summary) == "" {
		return fmt.Errorf("missing summary")
	}
	if !validVoiceTypes[strings.TrimSpace(info.Type)] {
		return fmt.Errorf("invalid type %q", info.Type)
	}
	if strings.ContainsAny(info.Category, `/\`) || strings.ContainsAny(info.Project, `/\`) {
		return fmt.Errorf("category or project contains a path separator")
	}
	for _, tag := range info.Tags {
		if strings.ContainsAny(tag, `/\`) {
			return fmt.Errorf("tag contains a path separator: %q", tag)
		}
	}
	return nil
}
