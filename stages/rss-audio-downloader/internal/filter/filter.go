// Package filter selects episodes from a parsed feed according to a Selector.
//
// Selection is composed of independent predicates combined with AND semantics.
// Episodes are first sorted by publish date ascending (oldest = episode 1, newest =
// last), then the index window [StartEpisode, EndEpisode] selects the range, then
// the per-episode predicates (date range, name regex) filter the result. Each rule
// is a small single-responsibility function to keep cyclomatic complexity low and
// make every rule independently testable.
package filter

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/jo-hoe/rss-audio-downloader/internal/config"
	"github.com/jo-hoe/rss-audio-downloader/internal/feed"
)

// Apply returns the episodes of f selected by sel, in chronological order
// (oldest first). An invalid NameRegex is reported as an error rather than
// silently dropping episodes.
func Apply(episodes []feed.ParsedEpisode, sel config.Selector) ([]feed.ParsedEpisode, error) {
	predicate, err := episodePredicate(sel)
	if err != nil {
		return nil, err
	}

	sorted := sortChronological(episodes)
	windowed := applyIndexWindow(sorted, sel.StartEpisode, sel.EndEpisode)

	selected := make([]feed.ParsedEpisode, 0, len(windowed))
	for _, ep := range windowed {
		if predicate(ep) {
			selected = append(selected, ep)
		}
	}
	return selected, nil
}

// sortChronological returns a new slice sorted by Published ascending (oldest
// first). Episodes with a zero Published date are placed at the end.
func sortChronological(episodes []feed.ParsedEpisode) []feed.ParsedEpisode {
	out := make([]feed.ParsedEpisode, len(episodes))
	copy(out, episodes)
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].Published, out[j].Published
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})
	return out
}

// applyIndexWindow slices episodes to the 1-based inclusive [start, end] range.
// A zero bound means "unbounded" on that side. Out-of-range bounds are clamped.
func applyIndexWindow(episodes []feed.ParsedEpisode, start, end int) []feed.ParsedEpisode {
	n := len(episodes)
	lo := 0
	if start > 0 {
		lo = start - 1
	}
	hi := n
	if end > 0 && end < n {
		hi = end
	}
	if lo >= n || lo >= hi {
		return nil
	}
	return episodes[lo:hi]
}

// episodePredicate builds the combined per-episode predicate (date range AND name
// regex). Rules that are unset contribute an always-true predicate.
func episodePredicate(sel config.Selector) (func(feed.ParsedEpisode) bool, error) {
	nameMatches, err := namePredicate(sel.NameRegex)
	if err != nil {
		return nil, err
	}
	withinDates := datePredicate(sel.StartDate, sel.EndDate)

	return func(ep feed.ParsedEpisode) bool {
		return withinDates(ep) && nameMatches(ep)
	}, nil
}

// datePredicate returns a predicate that keeps episodes whose Published date is
// within the optional [start, end] bounds. Episodes with a zero Published date are
// kept only when no date bound is set.
func datePredicate(start, end *time.Time) func(feed.ParsedEpisode) bool {
	if start == nil && end == nil {
		return alwaysTrue
	}
	return func(ep feed.ParsedEpisode) bool {
		if ep.Published.IsZero() {
			return false
		}
		if start != nil && ep.Published.Before(*start) {
			return false
		}
		if end != nil && ep.Published.After(*end) {
			return false
		}
		return true
	}
}

// namePredicate returns a predicate matching the episode title against pattern.
// An empty pattern matches everything.
func namePredicate(pattern string) (func(feed.ParsedEpisode) bool, error) {
	if pattern == "" {
		return alwaysTrue, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid nameRegex %q: %w", pattern, err)
	}
	return func(ep feed.ParsedEpisode) bool {
		return re.MatchString(ep.Title)
	}, nil
}

func alwaysTrue(feed.ParsedEpisode) bool { return true }
