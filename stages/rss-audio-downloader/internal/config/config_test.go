package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestLoad_AppliesDefaults(t *testing.T) {
	path := writeConfig(t, `
feeds:
  - url: https://example.com/rss
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %s", cfg.LogLevel)
	}
	if cfg.MaxParallelDownloads != 4 {
		t.Errorf("expected default maxParallelDownloads 4, got %d", cfg.MaxParallelDownloads)
	}
	if !filepath.IsAbs(cfg.WorkDir) {
		t.Errorf("expected workDir to be absolute, got %s", cfg.WorkDir)
	}
}

func TestLoad_ParsesSelector(t *testing.T) {
	path := writeConfig(t, `
workDir: /tmp/work
feeds:
  - url: https://example.com/rss
    selector:
      startEpisode: 1
      endEpisode: 5
      startDate: 2026-01-01T00:00:00Z
      nameRegex: "^Ep"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	sel := cfg.Feeds[0].Selector
	if sel.StartEpisode != 1 || sel.EndEpisode != 5 {
		t.Errorf("unexpected index bounds: %+v", sel)
	}
	if sel.StartDate == nil || sel.StartDate.Year() != 2026 {
		t.Errorf("startDate not parsed: %+v", sel.StartDate)
	}
	if sel.NameRegex != "^Ep" {
		t.Errorf("nameRegex not parsed: %s", sel.NameRegex)
	}
}

func TestLoad_RejectsEmptyFeeds(t *testing.T) {
	path := writeConfig(t, "logLevel: debug\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing feeds, got nil")
	}
}

func TestLoad_RejectsInvalidEpisodeRange(t *testing.T) {
	path := writeConfig(t, `
feeds:
  - url: https://example.com/rss
    selector:
      startEpisode: 5
      endEpisode: 2
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for endEpisode < startEpisode, got nil")
	}
}

func TestLoad_RejectsInvalidDateRange(t *testing.T) {
	path := writeConfig(t, `
feeds:
  - url: https://example.com/rss
    selector:
      startDate: 2026-06-01T00:00:00Z
      endDate: 2026-01-01T00:00:00Z
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for endDate before startDate, got nil")
	}
}
