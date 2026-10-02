// Package service orchestrates the zip stage: for each episode that has a
// transcript it builds a zip bundle containing the transcript plus podcast and
// episode metadata (the intermediate states), records the bundle path on the
// manifest, and writes the manifest back.
//
// The episode summary markdown is deliberately NOT placed inside the zip. It
// stays unzipped alongside the bundle in the summaries directory so it can be read
// directly (e.g. by the backup stage) without unpacking the archive.
package service

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/jo-hoe/artifact-zipper/internal/archive"
	"github.com/jo-hoe/artifact-zipper/internal/config"
	manifest "github.com/jo-hoe/manifest-lib"
	"gopkg.in/yaml.v3"
)

// bundleMetadataName is the name of the generated metadata file inside each zip.
const bundleMetadataName = "metadata.yaml"

// Service runs the zip stage. Its archive.Writer dependency is injected so the
// stage can be exercised without a real zip file when desired.
type Service struct {
	cfg    *config.Config
	writer archive.Writer
}

// New constructs a Service.
func New(cfg *config.Config, writer archive.Writer) *Service {
	return &Service{cfg: cfg, writer: writer}
}

// bundleMetadata is the traceability record embedded in each bundle. It links the
// zipped transcript back to its podcast and episode so an archived bundle is
// self-describing without the surrounding manifest.
type bundleMetadata struct {
	ShowTitle       string `yaml:"showTitle"`
	ShowDescription string `yaml:"showDescription,omitempty"`
	FeedURL         string `yaml:"feedURL"`
	Author          string `yaml:"author,omitempty"`

	EpisodeID      string `yaml:"episodeId"`
	Title          string `yaml:"title"`
	Published      string `yaml:"published,omitempty"`
	Description    string `yaml:"description,omitempty"`
	AudioURL       string `yaml:"audioURL,omitempty"`
	Duration       string `yaml:"duration,omitempty"`
	Language       string `yaml:"language,omitempty"`
	TranscribeModel string `yaml:"transcribeModel,omitempty"`

	// TranscriptEntry is the name of the transcript file inside this bundle.
	TranscriptEntry string `yaml:"transcriptEntry"`
}

// Run executes the stage and writes the manifest. Per-episode failures are logged
// and skipped; an error is returned only when the manifest cannot be read or the
// final write-back fails.
func (s *Service) Run() error {
	m, err := manifest.Load(s.cfg.WorkDir)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	bundled := 0
	for pi := range m.Podcasts {
		podcast := &m.Podcasts[pi]
		for ei := range podcast.Episodes {
			if s.bundleEpisode(podcast, &podcast.Episodes[ei]) {
				bundled++
			}
		}
	}

	if err := m.Save(s.cfg.WorkDir); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	slog.Info("zip stage complete", "workDir", s.cfg.WorkDir, "bundled", bundled)
	return nil
}

// bundleEpisode builds one episode's bundle and sets ep.BundleFile on success.
// It returns false (and logs) when the episode has no transcript to bundle or the
// bundle could not be written.
func (s *Service) bundleEpisode(podcast *manifest.Podcast, ep *manifest.Episode) bool {
	if ep.TranscriptFile == "" {
		slog.Warn("skipping episode without transcript", "id", ep.ID)
		return false
	}

	entries, err := s.buildEntries(podcast, ep)
	if err != nil {
		slog.Error("failed to build bundle entries", "id", ep.ID, "err", err)
		return false
	}

	bundleRel := ep.BundlePath()
	dest := filepath.Join(s.cfg.WorkDir, filepath.FromSlash(bundleRel))
	if err := s.writer.Write(dest, entries); err != nil {
		slog.Error("failed to write bundle", "id", ep.ID, "err", err)
		return false
	}

	ep.BundleFile = bundleRel
	slog.Info("bundled", "id", ep.ID, "bundle", bundleRel)
	return true
}

// buildEntries assembles the zip members: the transcript file plus a generated
// metadata.yaml. The summary is intentionally excluded.
func (s *Service) buildEntries(podcast *manifest.Podcast, ep *manifest.Episode) ([]archive.Entry, error) {
	transcriptEntry := filepath.Base(filepath.FromSlash(ep.TranscriptFile))

	meta := bundleMetadata{
		ShowTitle:       podcast.ShowTitle,
		ShowDescription: podcast.ShowDescription,
		FeedURL:         podcast.FeedURL,
		Author:          podcast.Author,
		EpisodeID:       ep.ID,
		Title:           ep.Title,
		Description:     ep.Description,
		AudioURL:        ep.AudioURL,
		Duration:        ep.Duration,
		Language:        ep.Language,
		TranscribeModel: ep.TranscribeModel,
		TranscriptEntry: transcriptEntry,
	}
	if !ep.Published.IsZero() {
		meta.Published = ep.Published.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	metaBytes, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal bundle metadata: %w", err)
	}

	transcriptSrc := filepath.Join(s.cfg.WorkDir, filepath.FromSlash(ep.TranscriptFile))
	return []archive.Entry{
		{Name: bundleMetadataName, Data: metaBytes},
		{Name: transcriptEntry, SourcePath: transcriptSrc},
	}, nil
}
