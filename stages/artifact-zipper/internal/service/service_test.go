package service

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jo-hoe/artifact-zipper/internal/archive"
	"github.com/jo-hoe/artifact-zipper/internal/config"
	manifest "github.com/jo-hoe/manifest-lib"
)

// setupWorkDir writes a manifest with one podcast/episode plus its transcript and
// summary files, returning the work directory.
func setupWorkDir(t *testing.T, withTranscript bool) (string, *manifest.Episode) {
	t.Helper()
	workDir := t.TempDir()

	ep := manifest.Episode{
		ID:              "science-vs-001-sourdough",
		Title:           "Sourdough",
		Description:     "All about bread.",
		Language:        "en",
		TranscribeModel: "base",
		SummaryFile:     "summaries/science-vs-001-sourdough.md",
	}

	if withTranscript {
		ep.TranscriptFile = ep.TranscriptPath("json")
		writeFile(t, workDir, ep.TranscriptFile, `{"segments":[{"text":"hi"}]}`)
	}
	// Summary always exists on disk (produced by stage C).
	writeFile(t, workDir, ep.SummaryFile, "# Summary\n\nBread is great.")

	m := &manifest.Manifest{Podcasts: []manifest.Podcast{{
		ShowTitle: "Science Vs",
		FeedURL:   "https://example.com/rss",
		Author:    "Spotify",
		Episodes:  []manifest.Episode{ep},
	}}}
	if err := m.Save(workDir); err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}
	return workDir, &ep
}

func writeFile(t *testing.T, workDir, rel, content string) {
	t.Helper()
	path := filepath.Join(workDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", rel, err)
	}
}

func zipEntryNames(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()
	out := make(map[string]string)
	for _, f := range r.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(data)
	}
	return out
}

func TestRun_BundleContainsTranscriptAndMetadataNotSummary(t *testing.T) {
	workDir, ep := setupWorkDir(t, true)

	svc := New(&config.Config{WorkDir: workDir}, archive.NewZipWriter())
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	bundlePath := filepath.Join(workDir, filepath.FromSlash(ep.BundlePath()))
	entries := zipEntryNames(t, bundlePath)

	if _, ok := entries["transcript.json"]; !ok {
		if _, ok2 := entries[filepath.Base(ep.TranscriptFile)]; !ok2 {
			t.Errorf("expected transcript entry in bundle, got %v", keys(entries))
		}
	}
	if _, ok := entries["metadata.yaml"]; !ok {
		t.Errorf("expected metadata.yaml in bundle, got %v", keys(entries))
	}
	for name := range entries {
		if strings.HasSuffix(name, ".md") {
			t.Errorf("summary markdown %q must not be inside the bundle", name)
		}
	}
	// Metadata should trace back to the podcast/episode.
	if !strings.Contains(entries["metadata.yaml"], "Science Vs") {
		t.Errorf("metadata missing show title: %q", entries["metadata.yaml"])
	}
	if !strings.Contains(entries["metadata.yaml"], "science-vs-001-sourdough") {
		t.Errorf("metadata missing episode id: %q", entries["metadata.yaml"])
	}
}

func TestRun_SummaryRemainsUnzippedOnDisk(t *testing.T) {
	workDir, ep := setupWorkDir(t, true)

	svc := New(&config.Config{WorkDir: workDir}, archive.NewZipWriter())
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	summaryPath := filepath.Join(workDir, filepath.FromSlash(ep.SummaryFile))
	if _, err := os.Stat(summaryPath); err != nil {
		t.Errorf("summary should remain on disk unzipped: %v", err)
	}
}

func TestRun_SetsBundleFileOnManifest(t *testing.T) {
	workDir, ep := setupWorkDir(t, true)

	svc := New(&config.Config{WorkDir: workDir}, archive.NewZipWriter())
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	m, err := manifest.Load(workDir)
	if err != nil {
		t.Fatalf("failed to reload manifest: %v", err)
	}
	got := m.Podcasts[0].Episodes[0].BundleFile
	if got != ep.BundlePath() {
		t.Errorf("BundleFile = %q, want %q", got, ep.BundlePath())
	}
}

func TestRun_SkipsEpisodeWithoutTranscript(t *testing.T) {
	workDir, ep := setupWorkDir(t, false)

	svc := New(&config.Config{WorkDir: workDir}, archive.NewZipWriter())
	if err := svc.Run(); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	bundlePath := filepath.Join(workDir, filepath.FromSlash(ep.BundlePath()))
	if _, err := os.Stat(bundlePath); !os.IsNotExist(err) {
		t.Errorf("expected no bundle for transcript-less episode, stat err = %v", err)
	}
	m, _ := manifest.Load(workDir)
	if m.Podcasts[0].Episodes[0].BundleFile != "" {
		t.Errorf("BundleFile should stay empty, got %q", m.Podcasts[0].Episodes[0].BundleFile)
	}
}

func TestRun_MissingManifestIsError(t *testing.T) {
	svc := New(&config.Config{WorkDir: t.TempDir()}, archive.NewZipWriter())
	if err := svc.Run(); err == nil {
		t.Fatal("expected error when manifest is missing, got nil")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
