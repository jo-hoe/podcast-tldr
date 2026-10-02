package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jo-hoe/rss-audio-downloader/internal/config"
	"github.com/jo-hoe/rss-audio-downloader/internal/feed"
	manifest "github.com/jo-hoe/manifest-lib"
)

// fakeParser returns a canned feed per URL.
type fakeParser struct {
	feeds map[string]*feed.ParsedFeed
	err   error
}

func (f *fakeParser) Parse(url string) (*feed.ParsedFeed, error) {
	if f.err != nil {
		return nil, f.err
	}
	pf, ok := f.feeds[url]
	if !ok {
		return nil, fmt.Errorf("no feed for %s", url)
	}
	return pf, nil
}

// fakeDownloader records requested URLs and writes placeholder content, optionally
// failing for URLs listed in failURLs.
type fakeDownloader struct {
	failURLs map[string]bool
	got      []string
}

func (d *fakeDownloader) Download(_ context.Context, url, destPath string) error {
	d.got = append(d.got, url)
	if d.failURLs[url] {
		return fmt.Errorf("simulated failure for %s", url)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, []byte("audio"), 0o644)
}

func testConfig(workDir, feedURL string) *config.Config {
	return &config.Config{
		WorkDir:              workDir,
		MaxParallelDownloads: 2,
		Feeds:                []config.Feed{{URL: feedURL}},
	}
}

func TestRun_WritesManifestAndDownloadsAudio(t *testing.T) {
	workDir := t.TempDir()
	feedURL := "https://example.com/rss"
	parser := &fakeParser{feeds: map[string]*feed.ParsedFeed{
		feedURL: {
			ShowTitle: "Science Vs",
			Episodes: []feed.ParsedEpisode{
				{Title: "Episode One", AudioURL: "https://cdn/1.mp3", AudioType: "audio/mpeg"},
				{Title: "Episode Two", AudioURL: "https://cdn/2.mp3", AudioType: "audio/mpeg"},
			},
		},
	}}
	dl := &fakeDownloader{}

	svc := New(testConfig(workDir, feedURL), parser, dl)
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	m, err := manifest.Load(workDir)
	if err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	if len(m.Podcasts) != 1 || len(m.Podcasts[0].Episodes) != 2 {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	ep := m.Podcasts[0].Episodes[0]
	// Episodes have no publish date in this fixture → fallback ID: show-title.
	if ep.AudioFile != "audio/science-vs-episode-one.mp3" {
		t.Errorf("unexpected audio path: %s", ep.AudioFile)
	}
	if _, err := os.Stat(filepath.Join(workDir, filepath.FromSlash(ep.AudioFile))); err != nil {
		t.Errorf("audio file not on disk: %v", err)
	}
	if len(dl.got) != 2 {
		t.Errorf("expected 2 downloads, got %d", len(dl.got))
	}
}

func TestRun_FailedDownloadDropsEpisode(t *testing.T) {
	workDir := t.TempDir()
	feedURL := "https://example.com/rss"
	parser := &fakeParser{feeds: map[string]*feed.ParsedFeed{
		feedURL: {
			ShowTitle: "Sliced Bread",
			Episodes: []feed.ParsedEpisode{
				{Title: "Good", AudioURL: "https://cdn/good.mp3", AudioType: "audio/mpeg"},
				{Title: "Bad", AudioURL: "https://cdn/bad.mp3", AudioType: "audio/mpeg"},
			},
		},
	}}
	dl := &fakeDownloader{failURLs: map[string]bool{"https://cdn/bad.mp3": true}}

	svc := New(testConfig(workDir, feedURL), parser, dl)
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	m, _ := manifest.Load(workDir)
	if len(m.Podcasts[0].Episodes) != 1 {
		t.Fatalf("expected 1 surviving episode, got %d", len(m.Podcasts[0].Episodes))
	}
	if m.Podcasts[0].Episodes[0].Title != "Good" {
		t.Errorf("wrong episode survived: %s", m.Podcasts[0].Episodes[0].Title)
	}
}

func TestRun_ParseErrorSkipsFeedButStillWritesManifest(t *testing.T) {
	workDir := t.TempDir()
	parser := &fakeParser{feeds: map[string]*feed.ParsedFeed{}} // no feed -> parse error
	dl := &fakeDownloader{}

	svc := New(testConfig(workDir, "https://missing/rss"), parser, dl)
	if err := svc.Run(context.Background()); err != nil {
		t.Fatalf("Run should not fail on per-feed parse error: %v", err)
	}
	m, err := manifest.Load(workDir)
	if err != nil {
		t.Fatalf("manifest should still be written: %v", err)
	}
	if len(m.Podcasts) != 0 {
		t.Errorf("expected no podcasts, got %d", len(m.Podcasts))
	}
}

func TestAudioExt(t *testing.T) {
	cases := []struct {
		mime, url, want string
	}{
		{"audio/mpeg", "https://x/y.mp3", "mp3"},
		{"audio/x-m4a", "https://x/y", "m4a"},
		{"", "https://x/y.ogg?token=1", "ogg"},
		{"", "https://x/y", "mp3"},
		{"application/octet-stream", "https://x/z.wav", "wav"},
	}
	for _, c := range cases {
		if got := audioExt(c.mime, c.url); got != c.want {
			t.Errorf("audioExt(%q,%q)=%q want %q", c.mime, c.url, got, c.want)
		}
	}
}
