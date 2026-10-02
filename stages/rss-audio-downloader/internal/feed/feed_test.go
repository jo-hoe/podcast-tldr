package feed

import (
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
)

// parseString maps a raw RSS string through the same mapFeed path Parse uses.
func parseString(t *testing.T, raw string) *ParsedFeed {
	t.Helper()
	src, err := gofeed.NewParser().ParseString(raw)
	if err != nil {
		t.Fatalf("gofeed failed to parse fixture: %v", err)
	}
	return mapFeed(src)
}

const sampleRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
  <channel>
    <title>Science Vs</title>
    <description>We pit facts against everything.</description>
    <itunes:author>Spotify Studios</itunes:author>
    <item>
      <title>Sourdough</title>
      <description>All about bread.</description>
      <pubDate>Wed, 15 Jan 2026 00:00:00 +0000</pubDate>
      <itunes:duration>1830</itunes:duration>
      <enclosure url="https://cdn.example.com/sourdough.mp3" length="1000" type="audio/mpeg"/>
    </item>
    <item>
      <title>No Audio Episode</title>
      <pubDate>Wed, 08 Jan 2026 00:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

func TestMapFeed_ExtractsShowAndEpisodeMetadata(t *testing.T) {
	pf := parseString(t, sampleRSS)

	if pf.ShowTitle != "Science Vs" {
		t.Errorf("show title = %q", pf.ShowTitle)
	}
	if pf.Author != "Spotify Studios" {
		t.Errorf("author = %q", pf.Author)
	}
	if len(pf.Episodes) != 1 {
		t.Fatalf("expected 1 episode with audio, got %d", len(pf.Episodes))
	}
	ep := pf.Episodes[0]
	if ep.Title != "Sourdough" {
		t.Errorf("title = %q", ep.Title)
	}
	if ep.AudioURL != "https://cdn.example.com/sourdough.mp3" {
		t.Errorf("audioURL = %q", ep.AudioURL)
	}
	if ep.Duration != "1830" {
		t.Errorf("duration = %q", ep.Duration)
	}
	if !ep.Published.Equal(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("published = %v", ep.Published)
	}
}

func TestMapFeed_SkipsEpisodesWithoutAudio(t *testing.T) {
	pf := parseString(t, sampleRSS)
	for _, ep := range pf.Episodes {
		if ep.Title == "No Audio Episode" {
			t.Fatal("episode without an enclosure should have been skipped")
		}
	}
}

func TestMapFeed_PrefersAudioEnclosure(t *testing.T) {
	raw := strings.Replace(sampleRSS,
		`<enclosure url="https://cdn.example.com/sourdough.mp3" length="1000" type="audio/mpeg"/>`,
		`<enclosure url="https://cdn.example.com/art.png" length="1" type="image/png"/>`+
			`<enclosure url="https://cdn.example.com/sourdough.mp3" length="1000" type="audio/mpeg"/>`,
		1)
	pf := parseString(t, raw)
	if pf.Episodes[0].AudioURL != "https://cdn.example.com/sourdough.mp3" {
		t.Errorf("expected audio enclosure preferred, got %q", pf.Episodes[0].AudioURL)
	}
}
