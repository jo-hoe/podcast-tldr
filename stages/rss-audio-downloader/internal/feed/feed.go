// Package feed parses podcast RSS/Atom feeds into a stage-neutral episode model.
// It wraps github.com/mmcdole/gofeed behind a small Parser interface so the rest
// of the stage never depends on the third-party types directly and can be tested
// with an in-memory parser.
package feed

import (
	"fmt"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// ParsedFeed is a podcast feed reduced to the fields this pipeline needs. Episodes
// are ordered exactly as the source feed lists them (conventionally newest-first).
type ParsedFeed struct {
	ShowTitle       string
	ShowDescription string
	Author          string
	Episodes        []ParsedEpisode
}

// ParsedEpisode is a single episode with its audio enclosure and metadata.
type ParsedEpisode struct {
	Title       string
	Description string
	Published   time.Time
	AudioURL    string
	Duration    string
	// AudioType is the enclosure MIME type, e.g. "audio/mpeg".
	AudioType string
}

// Parser turns a feed URL into a ParsedFeed. It is an interface so callers can
// substitute a fake in tests.
type Parser interface {
	Parse(url string) (*ParsedFeed, error)
}

// GofeedParser is the production Parser backed by gofeed.
type GofeedParser struct {
	fp *gofeed.Parser
}

// NewGofeedParser constructs a GofeedParser.
func NewGofeedParser() *GofeedParser {
	return &GofeedParser{fp: gofeed.NewParser()}
}

// Parse fetches and parses the feed at url.
func (p *GofeedParser) Parse(url string) (*ParsedFeed, error) {
	src, err := p.fp.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse feed %s: %w", url, err)
	}
	return mapFeed(src), nil
}

// mapFeed converts a gofeed.Feed into our stage-neutral ParsedFeed. Episodes
// without a usable audio enclosure are skipped: they cannot be downloaded.
func mapFeed(src *gofeed.Feed) *ParsedFeed {
	out := &ParsedFeed{
		ShowTitle:       src.Title,
		ShowDescription: src.Description,
		Author:          feedAuthor(src),
	}
	for _, item := range src.Items {
		ep, ok := mapEpisode(item)
		if ok {
			out.Episodes = append(out.Episodes, ep)
		}
	}
	return out
}

func mapEpisode(item *gofeed.Item) (ParsedEpisode, bool) {
	enc := audioEnclosure(item)
	if enc == nil {
		return ParsedEpisode{}, false
	}
	ep := ParsedEpisode{
		Title:       item.Title,
		Description: item.Description,
		AudioURL:    enc.URL,
		AudioType:   enc.Type,
	}
	if item.PublishedParsed != nil {
		ep.Published = *item.PublishedParsed
	}
	if item.ITunesExt != nil {
		ep.Duration = item.ITunesExt.Duration
	}
	return ep, true
}

// audioEnclosure returns the first enclosure whose type is an audio media type,
// falling back to the first enclosure if none advertise an audio type (some feeds
// omit or misreport the MIME type).
func audioEnclosure(item *gofeed.Item) *gofeed.Enclosure {
	if len(item.Enclosures) == 0 {
		return nil
	}
	for _, enc := range item.Enclosures {
		if strings.HasPrefix(strings.ToLower(enc.Type), "audio/") {
			return enc
		}
	}
	return item.Enclosures[0]
}

func feedAuthor(src *gofeed.Feed) string {
	if src.ITunesExt != nil && src.ITunesExt.Author != "" {
		return src.ITunesExt.Author
	}
	if src.Author != nil {
		return src.Author.Name
	}
	return ""
}
