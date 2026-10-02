// Package llm wraps an OpenAI-compatible chat client behind a small interface so
// the summarize stage can talk to a LiteLLM proxy in production and a stub in tests.
package llm

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// Request is a single summarization request. Instruction is the prompt template
// text; Transcript is the episode transcript to summarize; Metadata carries the
// episode's known-good context (from the podcast feed) that grounds the report.
type Request struct {
	Model       string
	Instruction string
	Metadata    Metadata
	Transcript  string
	MaxTokens   int
	Temperature float32
	// ReasoningModel selects reasoning-model request semantics: the token budget
	// is sent as max_completion_tokens and temperature is omitted.
	ReasoningModel bool
}

// Metadata is the feed-provided episode context. Unlike the transcript it is not
// machine-transcribed, so the model should treat it as authoritative (e.g. for the
// correct show/episode title and topic) when correcting transcription errors.
type Metadata struct {
	Show        string
	Title       string
	Published   string
	Duration    string
	Language    string
	Description string
}

// Client turns a Request into summary markdown.
type Client interface {
	Summarize(ctx context.Context, req Request) (string, error)
}

// OpenAIClient is the production Client backed by an OpenAI-compatible endpoint
// (a LiteLLM proxy). The base URL and API key are injected at construction.
type OpenAIClient struct {
	client *openai.Client
}

// NewOpenAIClient constructs a client pointed at baseURL (e.g. a LiteLLM proxy).
// The apiKey may be empty when the proxy requires no authentication.
func NewOpenAIClient(baseURL, apiKey string) *OpenAIClient {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = baseURL
	return &OpenAIClient{client: openai.NewClientWithConfig(cfg)}
}

// Summarize sends the instruction as a system message and the transcript as the
// user message, returning the assistant's markdown reply.
//
// Reasoning models (req.ReasoningModel) reject max_tokens and any non-default
// temperature, so the token budget is sent as max_completion_tokens and both
// max_tokens and temperature are left unset. Classic models use max_tokens and
// the requested temperature.
func (c *OpenAIClient) Summarize(ctx context.Context, req Request) (string, error) {
	chatReq := openai.ChatCompletionRequest{
		Model: req.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: req.Instruction},
			{Role: openai.ChatMessageRoleUser, Content: buildUserMessage(req.Metadata, req.Transcript)},
		},
	}
	if req.ReasoningModel {
		chatReq.MaxCompletionTokens = req.MaxTokens
	} else {
		chatReq.MaxTokens = req.MaxTokens
		chatReq.Temperature = req.Temperature
	}

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return "", fmt.Errorf("chat completion failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completion returned no choices")
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("chat completion returned empty content")
	}
	return content, nil
}

// buildUserMessage composes the user turn: an authoritative metadata block from
// the feed followed by the clearly-delimited machine transcript. Separating the
// two lets the prompt tell the model which parts to trust and which to correct.
func buildUserMessage(meta Metadata, transcript string) string {
	var b strings.Builder
	b.WriteString("PODCAST METADATA (authoritative — from the feed, not transcribed):\n")
	writeField(&b, "Show", meta.Show)
	writeField(&b, "Episode title", meta.Title)
	writeField(&b, "Published", meta.Published)
	writeField(&b, "Duration", meta.Duration)
	writeField(&b, "Language", meta.Language)
	writeField(&b, "Episode description", meta.Description)
	b.WriteString("\nMACHINE TRANSCRIPT (auto-generated; may contain errors — see instructions):\n")
	b.WriteString(transcript)
	return b.String()
}

func writeField(b *strings.Builder, label, value string) {
	if v := strings.TrimSpace(value); v != "" {
		fmt.Fprintf(b, "- %s: %s\n", label, v)
	}
}
