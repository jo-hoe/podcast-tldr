// Package service orchestrates the summarize stage: it loads the episodes.yaml
// manifest, reads each episode's transcript, summarizes it through the LLM client,
// writes summaries/<id>.md, and records the summary path and model on the manifest.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	manifest "github.com/jo-hoe/manifest-lib"
	"github.com/jo-hoe/llm-summarizer/internal/config"
	"github.com/jo-hoe/llm-summarizer/internal/llm"
	"github.com/jo-hoe/llm-summarizer/internal/prompt"
)

// Service runs the summarize stage. Dependencies are injected so the stage can be
// exercised in tests without a real LLM endpoint.
type Service struct {
	cfg       *config.Config
	client    llm.Client
	template  *prompt.Template
	episodeID string // when non-empty, only this episode is processed
}

// New constructs a Service with a loaded prompt template.
// episodeID may be empty to process all eligible episodes.
func New(cfg *config.Config, client llm.Client, template *prompt.Template, episodeID string) *Service {
	return &Service{cfg: cfg, client: client, template: template, episodeID: episodeID}
}

// Run loads the manifest, summarizes every eligible episode, and writes the
// manifest back. A per-episode failure is logged and skipped so one bad episode
// does not abort the whole run. It returns an error only when the manifest itself
// cannot be loaded or saved.
func (s *Service) Run(ctx context.Context) error {
	m, err := manifest.Load(s.cfg.WorkDir)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	summarized := 0
	for pi := range m.Podcasts {
		pod := &m.Podcasts[pi]
		for ei := range pod.Episodes {
			ep := &pod.Episodes[ei]
			if s.episodeID != "" && ep.ID != s.episodeID {
				continue
			}
			if s.summarizeEpisode(ctx, pod, ep) {
				summarized++
			}
		}
	}

	if err := m.Save(s.cfg.WorkDir); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	slog.Info("summarize stage complete", "workDir", s.cfg.WorkDir, "summarized", summarized)
	return nil
}

// summarizeEpisode summarizes a single episode, updating the fields the summarize
// stage owns. It returns true when a summary was produced. The owning podcast is
// passed so the report can be grounded in the show-level metadata.
func (s *Service) summarizeEpisode(ctx context.Context, pod *manifest.Podcast, ep *manifest.Episode) bool {
	if ep.TranscriptFile == "" {
		slog.Warn("skipping episode without transcript", "id", ep.ID)
		return false
	}

	transcriptPath := filepath.Join(s.cfg.WorkDir, filepath.FromSlash(ep.TranscriptFile))
	transcript, err := prompt.TranscriptText(transcriptPath)
	if err != nil {
		slog.Error("failed to read transcript", "id", ep.ID, "err", err)
		return false
	}

	summary, err := s.client.Summarize(ctx, llm.Request{
		Model:          s.cfg.Model,
		Instruction:    s.template.Instruction,
		Metadata:       episodeMetadata(pod, ep),
		Transcript:     transcript,
		MaxTokens:      s.cfg.MaxTokens,
		Temperature:    s.cfg.Temperature,
		ReasoningModel: s.cfg.ReasoningModel,
	})
	if err != nil {
		slog.Error("summarization failed", "id", ep.ID, "err", err)
		return false
	}

	if err := s.writeSummary(ep, summary); err != nil {
		slog.Error("failed to write summary", "id", ep.ID, "err", err)
		return false
	}

	ep.SummaryFile = ep.SummaryPath()
	ep.SummaryModel = s.cfg.Model
	slog.Info("summarized", "id", ep.ID, "file", ep.SummaryFile)
	return true
}

// writeSummary writes the markdown summary to the episode's summary path under the
// work directory, creating the summaries directory as needed.
func (s *Service) writeSummary(ep *manifest.Episode, summary string) error {
	dest := filepath.Join(s.cfg.WorkDir, filepath.FromSlash(ep.SummaryPath()))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("failed to create summaries directory: %w", err)
	}
	if err := os.WriteFile(dest, []byte(summary+"\n"), 0o644); err != nil {
		return fmt.Errorf("failed to write summary file: %w", err)
	}
	return nil
}

// episodeMetadata maps the manifest fields the model should treat as authoritative
// into an llm.Metadata. Published is rendered as a date; a zero time is omitted.
func episodeMetadata(pod *manifest.Podcast, ep *manifest.Episode) llm.Metadata {
	published := ""
	if !ep.Published.IsZero() {
		published = ep.Published.Format("2006-01-02")
	}
	return llm.Metadata{
		Show:        pod.ShowTitle,
		Title:       ep.Title,
		Published:   published,
		Duration:    ep.Duration,
		Language:    ep.Language,
		Description: ep.Description,
	}
}
