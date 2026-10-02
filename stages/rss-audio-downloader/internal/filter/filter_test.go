package filter

import (
	"testing"
	"time"

	"github.com/jo-hoe/rss-audio-downloader/internal/config"
	"github.com/jo-hoe/rss-audio-downloader/internal/feed"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// sampleEpisodes returns episodes in feed order (newest-first, as RSS delivers them).
// After Apply sorts chronologically the order becomes: Delta, Gamma, Beta, Alpha.
func sampleEpisodes() []feed.ParsedEpisode {
	return []feed.ParsedEpisode{
		{Title: "Episode Alpha", Published: day(2026, 3, 1)}, // chronological index 4
		{Title: "Episode Beta", Published: day(2026, 2, 1)},  // chronological index 3
		{Title: "Bonus Gamma", Published: day(2026, 1, 1)},   // chronological index 2
		{Title: "Episode Delta", Published: day(2025, 12, 1)}, // chronological index 1
	}
}

func titles(eps []feed.ParsedEpisode) []string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = e.Title
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApply_NoSelectorReturnsAll(t *testing.T) {
	got, err := Apply(sampleEpisodes(), config.Selector{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected all 4 episodes, got %d", len(got))
	}
}

func TestApply_NoSelectorResultIsChronological(t *testing.T) {
	got, _ := Apply(sampleEpisodes(), config.Selector{})
	// Chronological order: oldest first.
	want := []string{"Episode Delta", "Bonus Gamma", "Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_IndexWindow(t *testing.T) {
	// Episodes 2–3 chronologically = Gamma (Jan) and Beta (Feb).
	got, err := Apply(sampleEpisodes(), config.Selector{StartEpisode: 2, EndEpisode: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Bonus Gamma", "Episode Beta"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_StartEpisodeOnly(t *testing.T) {
	// From episode 3 onward = Beta (Feb) and Alpha (Mar).
	got, _ := Apply(sampleEpisodes(), config.Selector{StartEpisode: 3})
	want := []string{"Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_EndEpisodeOnly(t *testing.T) {
	// Up to episode 1 = Delta (Dec) only.
	got, _ := Apply(sampleEpisodes(), config.Selector{EndEpisode: 1})
	want := []string{"Episode Delta"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_FirstEpisode(t *testing.T) {
	// Episode 1 = oldest/first ever published.
	got, _ := Apply(sampleEpisodes(), config.Selector{StartEpisode: 1, EndEpisode: 1})
	want := []string{"Episode Delta"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_LatestEpisode(t *testing.T) {
	// Episode 4 = newest (total count known from feed).
	got, _ := Apply(sampleEpisodes(), config.Selector{StartEpisode: 4, EndEpisode: 4})
	want := []string{"Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_StartDateOnly(t *testing.T) {
	start := day(2026, 2, 1)
	got, _ := Apply(sampleEpisodes(), config.Selector{StartDate: &start})
	want := []string{"Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_EndDateOnly(t *testing.T) {
	end := day(2026, 1, 1)
	got, _ := Apply(sampleEpisodes(), config.Selector{EndDate: &end})
	want := []string{"Episode Delta", "Bonus Gamma"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_DateRange(t *testing.T) {
	start := day(2026, 1, 1)
	end := day(2026, 2, 28)
	got, _ := Apply(sampleEpisodes(), config.Selector{StartDate: &start, EndDate: &end})
	want := []string{"Bonus Gamma", "Episode Beta"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_NameRegex(t *testing.T) {
	got, err := Apply(sampleEpisodes(), config.Selector{NameRegex: "^Episode"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Episode Delta", "Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_CombinedRulesAreANDed(t *testing.T) {
	start := day(2026, 1, 15)
	got, _ := Apply(sampleEpisodes(), config.Selector{
		NameRegex: "^Episode",
		StartDate: &start,
	})
	want := []string{"Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestApply_InvalidRegexIsError(t *testing.T) {
	if _, err := Apply(sampleEpisodes(), config.Selector{NameRegex: "("}); err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func TestApply_ZeroPublishedDroppedWhenDateBounded(t *testing.T) {
	eps := []feed.ParsedEpisode{{Title: "No Date"}}
	start := day(2020, 1, 1)
	got, _ := Apply(eps, config.Selector{StartDate: &start})
	if len(got) != 0 {
		t.Fatalf("expected zero-date episode dropped, got %v", titles(got))
	}
}

func TestApply_StartBeyondRangeReturnsEmpty(t *testing.T) {
	got, _ := Apply(sampleEpisodes(), config.Selector{StartEpisode: 99})
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", titles(got))
	}
}

func TestSortChronological_OldestFirst(t *testing.T) {
	eps := sampleEpisodes() // feed order: newest-first
	got := sortChronological(eps)
	want := []string{"Episode Delta", "Bonus Gamma", "Episode Beta", "Episode Alpha"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

func TestSortChronological_ZeroDatesLast(t *testing.T) {
	eps := []feed.ParsedEpisode{
		{Title: "No Date"},
		{Title: "Early", Published: day(2020, 1, 1)},
	}
	got := sortChronological(eps)
	want := []string{"Early", "No Date"}
	if !equal(titles(got), want) {
		t.Fatalf("got %v, want %v", titles(got), want)
	}
}

