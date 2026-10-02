package layout

import "testing"

func TestEpisodeDir_SlugifiesShowTitle(t *testing.T) {
	got := EpisodeDir("The Daily Show!", "ep-001")
	want := "podcasts/the-daily-show/ep-001"
	if got != want {
		t.Errorf("EpisodeDir = %q, want %q", got, want)
	}
}

func TestSummaryPath(t *testing.T) {
	got := SummaryPath("My Show", "ep-001")
	want := "podcasts/my-show/ep-001/summary.md"
	if got != want {
		t.Errorf("SummaryPath = %q, want %q", got, want)
	}
}

func TestBundlePath(t *testing.T) {
	got := BundlePath("My Show", "ep-001")
	want := "podcasts/my-show/ep-001/bundle.zip"
	if got != want {
		t.Errorf("BundlePath = %q, want %q", got, want)
	}
}

func TestPaths_UseForwardSlashes(t *testing.T) {
	for _, p := range []string{
		EpisodeDir("Show", "id"),
		SummaryPath("Show", "id"),
		BundlePath("Show", "id"),
	} {
		for _, r := range p {
			if r == '\\' {
				t.Errorf("path %q contains a backslash", p)
			}
		}
	}
}
