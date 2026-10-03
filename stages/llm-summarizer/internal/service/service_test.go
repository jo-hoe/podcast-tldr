package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	manifest "github.com/jo-hoe/manifest-lib"
	"github.com/jo-hoe/llm-summarizer/internal/config"
	"github.com/jo-hoe/llm-summarizer/internal/llm"
	"github.com/jo-hoe/llm-summarizer/internal/prompt"
)

// stubChatServer returns an httptest server that answers OpenAI chat-completion
// requests with a fixed assistant message.
func stubChatServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  "summarizer",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": reply,
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedManifest(t *testing.T, workDir string, ep manifest.Episode) {
	t.Helper()
	m := &manifest.Manifest{Podcasts: []manifest.Podcast{{
		ShowTitle: "Test Show",
		FeedURL:   "http://example.com/feed",
		Episodes:  []manifest.Episode{ep},
	}}}
	if err := m.Save(workDir); err != nil {
		t.Fatalf("failed to seed manifest: %v", err)
	}
}

func writeTranscript(t *testing.T, workDir, rel, text string) {
	t.Helper()
	dest := filepath.Join(workDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("failed to create transcript dir: %v", err)
	}
	doc := map[string]any{"text": text}
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}
}

func newService(t *testing.T, workDir string, client llm.Client) *Service {
	t.Helper()
	cfg := &config.Config{
		WorkDir:     workDir,
		Model:       "summarizer",
		MaxTokens:   256,
		Temperature: 0.3,
	}
	return New(cfg, client, &prompt.Template{Instruction: "Summarize."}, "")
}

// captureChatServer records the decoded request body of the first chat-completion
// call and answers with a fixed assistant message.
func captureChatServer(t *testing.T, reply string, captured *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*captured = body
		resp := map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  "summarizer",
			"choices": []map[string]any{{
				"index":         0,
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": reply},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runOnceCapturing(t *testing.T, cfg *config.Config) map[string]any {
	t.Helper()
	workDir := t.TempDir()
	cfg.WorkDir = workDir
	ep := manifest.Episode{ID: "ep-1", Title: "First", TranscriptFile: "transcripts/ep-1.json"}
	seedManifest(t, workDir, ep)
	writeTranscript(t, workDir, ep.TranscriptFile, "a full transcript body")

	var captured map[string]any
	srv := captureChatServer(t, "summary text", &captured)
	svc := New(cfg, llm.NewOpenAIClient(srv.URL, ""), &prompt.Template{Instruction: "Summarize."}, "")
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	return captured
}

func TestRun_ClassicModelSendsMaxTokensAndTemperature(t *testing.T) {
	body := runOnceCapturing(t, &config.Config{
		Model:       "summarizer",
		MaxTokens:   256,
		Temperature: 0.3,
	})
	if _, ok := body["max_tokens"]; !ok {
		t.Errorf("classic mode must send max_tokens, body=%v", body)
	}
	if _, ok := body["temperature"]; !ok {
		t.Errorf("classic mode must send temperature, body=%v", body)
	}
	if _, ok := body["max_completion_tokens"]; ok {
		t.Errorf("classic mode must NOT send max_completion_tokens, body=%v", body)
	}
}

func TestRun_ReasoningModelSendsMaxCompletionTokensAndNoTemperature(t *testing.T) {
	body := runOnceCapturing(t, &config.Config{
		Model:          "gpt-5",
		MaxTokens:      4096,
		Temperature:    0.3,
		ReasoningModel: true,
	})
	if _, ok := body["max_completion_tokens"]; !ok {
		t.Errorf("reasoning mode must send max_completion_tokens, body=%v", body)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Errorf("reasoning mode must NOT send max_tokens, body=%v", body)
	}
	if _, ok := body["temperature"]; ok {
		t.Errorf("reasoning mode must NOT send temperature, body=%v", body)
	}
}

func TestRun_WritesSummaryAndUpdatesManifest(t *testing.T) {
	workDir := t.TempDir()
	ep := manifest.Episode{ID: "ep-1", Title: "First", TranscriptFile: "transcripts/ep-1.json"}
	seedManifest(t, workDir, ep)
	writeTranscript(t, workDir, ep.TranscriptFile, "a full transcript body")

	srv := stubChatServer(t, "## Summary\n\nThis is the summary.")
	client := llm.NewOpenAIClient(srv.URL, "test-key")
	svc := newService(t, workDir, client)

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Summary file written at the canonical summary path.
	summaryPath := filepath.Join(workDir, filepath.FromSlash(ep.SummaryPath()))
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("expected summary file at %s: %v", summaryPath, err)
	}
	if string(data) != "## Summary\n\nThis is the summary.\n" {
		t.Errorf("unexpected summary content: %q", string(data))
	}

	// Manifest updated with the summarize-stage fields.
	m, err := manifest.Load(workDir)
	if err != nil {
		t.Fatalf("failed to reload manifest: %v", err)
	}
	got := m.Podcasts[0].Episodes[0]
	if got.SummaryFile != ep.SummaryPath() {
		t.Errorf("SummaryFile = %q, want %q", got.SummaryFile, ep.SummaryPath())
	}
	if got.SummaryModel != "summarizer" {
		t.Errorf("SummaryModel = %q, want summarizer", got.SummaryModel)
	}
	// Upstream fields left untouched.
	if got.TranscriptFile != ep.TranscriptFile || got.Title != ep.Title {
		t.Errorf("upstream fields mutated: %+v", got)
	}
}

func TestRun_SkipsEpisodeWithoutTranscript(t *testing.T) {
	workDir := t.TempDir()
	seedManifest(t, workDir, manifest.Episode{ID: "ep-2", Title: "No transcript"})

	srv := stubChatServer(t, "should not be used")
	svc := newService(t, workDir, llm.NewOpenAIClient(srv.URL, ""))

	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	m, _ := manifest.Load(workDir)
	if got := m.Podcasts[0].Episodes[0]; got.SummaryFile != "" {
		t.Errorf("expected no summary for episode without transcript, got %q", got.SummaryFile)
	}
}

func TestRun_SkipsWhenTranscriptFileMissing(t *testing.T) {
	workDir := t.TempDir()
	// TranscriptFile references a path that does not exist on disk.
	ep := manifest.Episode{ID: "ep-3", TranscriptFile: "transcripts/ep-3.json"}
	seedManifest(t, workDir, ep)

	srv := stubChatServer(t, "unused")
	svc := newService(t, workDir, llm.NewOpenAIClient(srv.URL, ""))

	// Run must not fail the whole stage for one unreadable transcript.
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run should tolerate a missing transcript file, got: %v", err)
	}

	m, _ := manifest.Load(workDir)
	if got := m.Podcasts[0].Episodes[0]; got.SummaryFile != "" {
		t.Errorf("expected no summary when transcript missing, got %q", got.SummaryFile)
	}
}

func TestRun_FailsWhenManifestMissing(t *testing.T) {
	svc := newService(t, t.TempDir(), llm.NewOpenAIClient("http://unused", ""))
	if err := svc.Run(context.Background()); err == nil {
		t.Fatal("expected error when manifest is absent, got nil")
	}
}

// userMessage extracts the user-role message content from a captured chat request.
func userMessage(t *testing.T, body map[string]any) string {
	t.Helper()
	msgs, ok := body["messages"].([]any)
	if !ok {
		t.Fatalf("no messages in body: %v", body)
	}
	for _, raw := range msgs {
		m, _ := raw.(map[string]any)
		if m["role"] == "user" {
			return m["content"].(string)
		}
	}
	t.Fatalf("no user message in body: %v", body)
	return ""
}

func TestRun_UserMessageIncludesMetadataAndDelimitedTranscript(t *testing.T) {
	workDir := t.TempDir()
	cfg := &config.Config{Model: "summarizer", MaxTokens: 256, Temperature: 0.3, WorkDir: workDir}

	published, _ := time.Parse(time.RFC3339, "2026-09-24T09:00:00Z")
	ep := manifest.Episode{
		ID: "ep-1", Title: "Series 5: Pets", Published: published,
		Description: "Is this product worth it?", Language: "en", Duration: "30:00",
		TranscriptFile: "transcripts/ep-1.json",
	}
	seedManifest(t, workDir, ep)
	writeTranscript(t, workDir, ep.TranscriptFile, "the transcript body text")

	var captured map[string]any
	srv := captureChatServer(t, "report", &captured)
	svc := New(cfg, llm.NewOpenAIClient(srv.URL, ""), &prompt.Template{Instruction: "Write a report."}, "")
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	msg := userMessage(t, captured)
	for _, want := range []string{
		"Test Show",              // show title (authoritative)
		"Series 5: Pets",         // episode title
		"2026-09-24",             // published date, formatted
		"Is this product worth it?", // description
		"the transcript body text",  // the transcript itself
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("user message missing %q\n---\n%s", want, msg)
		}
	}
	// The transcript must be labelled as machine-generated so the prompt's
	// correction instructions apply to the right section.
	if !strings.Contains(msg, "MACHINE TRANSCRIPT") {
		t.Errorf("user message must delimit the machine transcript, got:\n%s", msg)
	}
}
