package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSlugify_Basic(t *testing.T) {
	cases := map[string]string{
		"Science Vs":               "science-vs",
		"  Sliced Bread!!  ":       "sliced-bread",
		"Ep. 12: The A.I. Episode": "ep-12-the-a-i-episode",
		"---weird___input---":      "weird-input",
		"":                         "untitled",
		"你好":                       "untitled",
		"Already-a-slug":           "already-a-slug",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEpisodePaths_UseForwardSlashesAndID(t *testing.T) {
	e := &Episode{ID: "science-vs-42"}
	if got, want := e.AudioPath("mp3"), "audio/science-vs-42.mp3"; got != want {
		t.Errorf("AudioPath = %q, want %q", got, want)
	}
	if got, want := e.AudioPath(".mp3"), "audio/science-vs-42.mp3"; got != want {
		t.Errorf("AudioPath with dotted ext = %q, want %q", got, want)
	}
	if got, want := e.TranscriptPath("json"), "transcripts/science-vs-42.json"; got != want {
		t.Errorf("TranscriptPath = %q, want %q", got, want)
	}
	if got, want := e.SummaryPath(), "summaries/science-vs-42.md"; got != want {
		t.Errorf("SummaryPath = %q, want %q", got, want)
	}
	if got, want := e.BundlePath(), "bundles/science-vs-42.zip"; got != want {
		t.Errorf("BundlePath = %q, want %q", got, want)
	}
}

func TestSaveThenLoad_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	published := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	original := &Manifest{
		Podcasts: []Podcast{{
			ShowTitle: "Science Vs",
			FeedURL:   "https://example.com/rss",
			Episodes: []Episode{{
				ID:        "science-vs-42",
				Title:     "The A.I. Episode",
				Published: published,
				AudioURL:  "https://example.com/ep.mp3",
				AudioFile: "audio/science-vs-42.mp3",
			}},
		}},
	}

	if err := original.Save(dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded.Podcasts) != 1 || len(loaded.Podcasts[0].Episodes) != 1 {
		t.Fatalf("unexpected structure: %+v", loaded)
	}
	ep := loaded.Podcasts[0].Episodes[0]
	if ep.ID != "science-vs-42" || ep.Title != "The A.I. Episode" {
		t.Errorf("episode fields not preserved: %+v", ep)
	}
	if !ep.Published.Equal(published) {
		t.Errorf("published time not preserved: got %v want %v", ep.Published, published)
	}
}

func TestSave_IsAtomicAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{}
	if err := m.Save(dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Errorf("expected manifest file to exist, stat err = %v", err)
	}
}

func TestLoad_MissingFileIsError(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected error loading missing manifest, got nil")
	}
}
