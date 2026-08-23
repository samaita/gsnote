package voice

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newOpenAILLM(t *testing.T, srv *httptest.Server) *OpenAILLM {
	t.Helper()
	return &OpenAILLM{
		APIKey:  "test-key",
		BaseURL: srv.URL,
		Model:   "gpt-4o-mini",
		Client:  srv.Client(),
	}
}

func llmReply(content string) string {
	out, err := json.Marshal(map[string]interface{}{
		"choices": []map[string]interface{}{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(out)
}

func TestLLMProcessPlainJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(readAll(t, r), "response_format") {
			t.Fatal("expected response_format in first request")
		}
		w.Write([]byte(llmReply(`{"title":"Address Quality","summary":"Summary","content":"Content","type":"idea","category":"address-quality","project":"address-quality","tags":["api"]}`)))
	}))
	defer srv.Close()

	info, err := newOpenAILLM(t, srv).Process("transcript")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if info.Title != "Address Quality" || info.Type != "idea" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestLLMProcessJSONInCodeFence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(llmReply("Here you go:\n```json\n{\"title\":\"T\",\"summary\":\"S\",\"content\":\"C\",\"type\":\"note\",\"category\":\"cat\",\"project\":\"proj\",\"tags\":[\"a\"]}\n```")))
	}))
	defer srv.Close()

	info, err := newOpenAILLM(t, srv).Process("transcript")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if info.Title != "T" {
		t.Fatalf("unexpected title: %q", info.Title)
	}
}

func TestLLMProcessProseAroundJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(llmReply("Sure! {\"title\":\"T\",\"summary\":\"S\",\"content\":\"C\",\"type\":\"task\",\"category\":\"cat\",\"project\":\"proj\",\"tags\":[\"a\"]} Hope that helps!")))
	}))
	defer srv.Close()

	info, err := newOpenAILLM(t, srv).Process("transcript")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if info.Type != "task" {
		t.Fatalf("unexpected type: %q", info.Type)
	}
}

func TestLLMProcessNonJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(llmReply("🌟 Greetings, Earthling! How art thou? ✨")))
	}))
	defer srv.Close()

	_, err := newOpenAILLM(t, srv).Process("transcript")
	if err == nil {
		t.Fatal("expected error for non-JSON reply")
	}
	if !strings.Contains(err.Error(), "no JSON object") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLLMProcessRetriesWithoutResponseFormat(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "response_format not supported", http.StatusBadRequest)
			return
		}
		if strings.Contains(readAll(t, r), "response_format") {
			t.Fatal("retry must not include response_format")
		}
		w.Write([]byte(llmReply(`{"title":"T","summary":"S","content":"C","type":"insight","category":"cat","project":"proj","tags":["a"]}`)))
	}))
	defer srv.Close()

	info, err := newOpenAILLM(t, srv).Process("transcript")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected exactly two calls (initial + retry), got %d", calls)
	}
	if info.Type != "insight" {
		t.Fatalf("unexpected type: %q", info.Type)
	}
}

func readAll(t *testing.T, r *http.Request) string {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}
