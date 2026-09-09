package voice

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newElevenServer(t *testing.T, handler http.HandlerFunc) *ElevenTranscriber {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &ElevenTranscriber{APIKey: "test-key", BaseURL: srv.URL, Model: "scribe_v1", Client: srv.Client()}
}

func TestElevenTranscriber_HappyPath(t *testing.T) {
	var gotAuth, gotModel string
	var gotFile []byte
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("xi-api-key")
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		gotModel = r.FormValue("model_id")
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		gotFile, _ = io.ReadAll(f)
		json.NewEncoder(w).Encode(map[string]any{"language_code": "eng", "text": "hello world"})
	})

	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("FAKEOGG"), 0644)

	text, err := tr.Transcribe(tmp)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
	if gotAuth != "test-key" {
		t.Errorf("api key header = %q", gotAuth)
	}
	if gotModel != "scribe_v1" {
		t.Errorf("model_id = %q", gotModel)
	}
	if string(gotFile) != "FAKEOGG" {
		t.Errorf("uploaded audio mismatch")
	}
}

func TestElevenTranscriber_LanguageCode(t *testing.T) {
	var gotLang string
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotLang = r.FormValue("language_code")
		json.NewEncoder(w).Encode(map[string]any{"text": "halo"})
	})
	tr.Language = "id"
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	if _, err := tr.Transcribe(tmp); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if gotLang != "id" {
		t.Errorf("language_code = %q, want %q", gotLang, "id")
	}
}

func TestElevenTranscriber_LanguageOmittedWhenEmpty(t *testing.T) {
	var gotLang string
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseMultipartForm(10 << 20)
		gotLang = r.FormValue("language_code")
		json.NewEncoder(w).Encode(map[string]any{"text": "halo"})
	})
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	if _, err := tr.Transcribe(tmp); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if gotLang != "" {
		t.Errorf("language_code = %q, want omitted (auto-detect)", gotLang)
	}
}

func TestElevenTranscriber_MissingAPIKey(t *testing.T) {
	tr := &ElevenTranscriber{}
	_, err := tr.Transcribe(filepath.Join(t.TempDir(), "v.ogg"))
	if err == nil || !strings.Contains(err.Error(), "ELEVEN_API_KEY") {
		t.Fatalf("want missing-key error, got %v", err)
	}
}

func TestElevenTranscriber_APIError(t *testing.T) {
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail":{"status":401,"message":"invalid_api_key"}}`))
	})
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	_, err := tr.Transcribe(tmp)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestElevenTranscriber_EmptyTranscript(t *testing.T) {
	tr := newElevenServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"text": "   "})
	})
	tmp := filepath.Join(t.TempDir(), "v.ogg")
	os.WriteFile(tmp, []byte("x"), 0644)
	text, err := tr.Transcribe(tmp)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "" {
		t.Errorf("want trimmed empty, got %q", text)
	}
}

func TestElevenTranscriber_MissingAudioFile(t *testing.T) {
	tr := &ElevenTranscriber{APIKey: "k"}
	_, err := tr.Transcribe(filepath.Join(t.TempDir(), "nonexistent.ogg"))
	if err == nil {
		t.Fatal("want open error for missing file")
	}
}
