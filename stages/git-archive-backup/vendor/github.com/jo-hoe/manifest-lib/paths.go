package manifest

import (
	"regexp"
	"strings"
)

// Canonical sub-directory names inside the work directory. Every stage writes its
// artifacts under one of these so the layout is predictable and greppable.
const (
	AudioDir      = "audio"
	TranscriptDir = "transcripts"
	SummaryDir    = "summaries"
	BundleDir     = "bundles"
)

var (
	nonSlugChars   = regexp.MustCompile(`[^a-z0-9]+`)
	trimSlugDashes = regexp.MustCompile(`^-+|-+$`)
)

// Slugify converts an arbitrary title into a filesystem- and URL-safe slug:
// lowercase, ASCII alphanumerics separated by single dashes. It is deterministic
// so the same title always yields the same slug across stages and runs. An input
// with no usable characters yields "untitled".
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlugChars.ReplaceAllString(s, "-")
	s = trimSlugDashes.ReplaceAllString(s, "")
	if s == "" {
		return "untitled"
	}
	return s
}

// AudioPath returns the work-relative path where the episode's audio is stored.
func (e *Episode) AudioPath(ext string) string {
	return join(AudioDir, e.ID+dot(ext))
}

// TranscriptPath returns the work-relative path for the episode transcript.
func (e *Episode) TranscriptPath(ext string) string {
	return join(TranscriptDir, e.ID+dot(ext))
}

// SummaryPath returns the work-relative path for the episode summary.
func (e *Episode) SummaryPath() string {
	return join(SummaryDir, e.ID+".md")
}

// BundlePath returns the work-relative path for the zipped episode bundle.
func (e *Episode) BundlePath() string {
	return join(BundleDir, e.ID+".zip")
}

// join builds a forward-slash work-relative path. Manifest paths are always
// stored with forward slashes so they are portable across operating systems.
func join(parts ...string) string {
	return strings.Join(parts, "/")
}

// dot normalizes an optional extension so callers may pass either "mp3" or ".mp3".
func dot(ext string) string {
	if ext == "" {
		return ""
	}
	if strings.HasPrefix(ext, ".") {
		return ext
	}
	return "." + ext
}
