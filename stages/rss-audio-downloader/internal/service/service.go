// Package service orchestrates the download stage: for each configured feed it
// parses the feed, selects episodes via the configured selector, downloads their
// audio concurrently (bounded by MaxParallelDownloads), and writes the resulting
// episodes.yaml manifest into the work directory.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jo-hoe/rss-audio-downloader/internal/config"
	"github.com/jo-hoe/rss-audio-downloader/internal/download"
	"github.com/jo-hoe/rss-audio-downloader/internal/feed"
	"github.com/jo-hoe/rss-audio-downloader/internal/filter"
	manifest "github.com/jo-hoe/manifest-lib"
)

// Service runs the download stage. Its dependencies are injected so the whole
// stage can be exercised in tests without network or disk access.
type Service struct {
	cfg        *config.Config
	parser     feed.Parser
	downloader download.Downloader
}

// New constructs a Service.
func New(cfg *config.Config, parser feed.Parser, downloader download.Downloader) *Service {
	return &Service{cfg: cfg, parser: parser, downloader: downloader}
}

// Run executes the stage and writes the manifest. It returns an error only for
// failures that make the manifest unwritable; per-episode download failures are
// logged and skipped so one bad episode does not abort the whole run.
func (s *Service) Run(ctx context.Context) error {
	m := &manifest.Manifest{}

	for _, feedCfg := range s.cfg.Feeds {
		podcast, err := s.collectPodcast(ctx, feedCfg)
		if err != nil {
			slog.Error("skipping feed", "url", feedCfg.URL, "err", err)
			continue
		}
		if len(podcast.Episodes) > 0 {
			m.Podcasts = append(m.Podcasts, *podcast)
		}
	}

	if err := m.Save(s.cfg.WorkDir); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	slog.Info("download stage complete", "workDir", s.cfg.WorkDir, "podcasts", len(m.Podcasts))
	return nil
}

// collectPodcast parses and filters a single feed, then downloads the selected
// episodes into a manifest.Podcast.
func (s *Service) collectPodcast(ctx context.Context, feedCfg config.Feed) (*manifest.Podcast, error) {
	parsed, err := s.parser.Parse(feedCfg.URL)
	if err != nil {
		return nil, err
	}

	selected, err := filter.Apply(parsed.Episodes, feedCfg.Selector)
	if err != nil {
		return nil, err
	}
	slog.Info("feed parsed", "show", parsed.ShowTitle, "total", len(parsed.Episodes), "selected", len(selected))

	episodes := s.buildEpisodes(parsed, selected)
	s.downloadAll(ctx, episodes)

	return &manifest.Podcast{
		ShowTitle:       parsed.ShowTitle,
		ShowDescription: parsed.ShowDescription,
		FeedURL:         feedCfg.URL,
		Author:          parsed.Author,
		Episodes:        keepDownloaded(episodes),
	}, nil
}

// buildEpisodes maps selected parsed episodes into manifest episodes with stable
// IDs and target audio paths (audio not yet downloaded).
func (s *Service) buildEpisodes(parsed *feed.ParsedFeed, selected []feed.ParsedEpisode) []manifest.Episode {
	show := manifest.Slugify(parsed.ShowTitle)
	episodes := make([]manifest.Episode, 0, len(selected))
	for _, pe := range selected {
		id := episodeID(show, pe.Title, pe.Published)
		ep := manifest.Episode{
			ID:          id,
			Title:       pe.Title,
			Published:   pe.Published,
			Description: pe.Description,
			AudioURL:    pe.AudioURL,
			Duration:    pe.Duration,
		}
		ep.AudioFile = ep.AudioPath(audioExt(pe.AudioType, pe.AudioURL))
		episodes = append(episodes, ep)
	}
	return episodes
}

// downloadAll downloads every episode's audio concurrently, bounded by
// MaxParallelDownloads. On failure it clears AudioFile so the episode is dropped.
func (s *Service) downloadAll(ctx context.Context, episodes []manifest.Episode) {
	sem := make(chan struct{}, s.cfg.MaxParallelDownloads)
	var wg sync.WaitGroup

	for i := range episodes {
		wg.Add(1)
		go func(ep *manifest.Episode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dest := filepath.Join(s.cfg.WorkDir, filepath.FromSlash(ep.AudioFile))
			if err := s.downloader.Download(ctx, ep.AudioURL, dest); err != nil {
				slog.Error("download failed", "id", ep.ID, "url", ep.AudioURL, "err", err)
				ep.AudioFile = ""
				return
			}
			slog.Info("downloaded", "id", ep.ID, "file", ep.AudioFile)
		}(&episodes[i])
	}
	wg.Wait()
}

// keepDownloaded returns only the episodes whose audio was successfully written.
func keepDownloaded(episodes []manifest.Episode) []manifest.Episode {
	kept := make([]manifest.Episode, 0, len(episodes))
	for _, ep := range episodes {
		if ep.AudioFile != "" {
			kept = append(kept, ep)
		}
	}
	return kept
}

// episodeID builds a stable per-episode ID from the show slug, publish date
// (YYYY-MM-DD), and title slug. Using the publish date instead of a sequential
// index means IDs are independent of feed order and selector window — the same
// episode always gets the same ID regardless of how it was selected.
// When the publish date is zero (missing from the feed), it falls back to the
// title slug alone.
func episodeID(showSlug, title string, published time.Time) string {
	titleSlug := manifest.Slugify(title)
	if published.IsZero() {
		return fmt.Sprintf("%s-%s", showSlug, titleSlug)
	}
	return fmt.Sprintf("%s-%s-%s", showSlug, published.UTC().Format("2006-01-02"), titleSlug)
}

// audioExt determines the file extension from the enclosure MIME type, falling
// back to the URL extension, then to "mp3".
func audioExt(mimeType, url string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	case "audio/mp4", "audio/x-m4a", "audio/aac":
		return "m4a"
	case "audio/ogg", "audio/opus":
		return "ogg"
	case "audio/wav", "audio/x-wav":
		return "wav"
	}
	if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(stripQuery(url))), "."); ext != "" {
		return ext
	}
	return "mp3"
}

func stripQuery(url string) string {
	if i := strings.IndexAny(url, "?#"); i >= 0 {
		return url[:i]
	}
	return url
}
