// Package layout builds the archive repository's folder structure for a podcast
// episode. The archive stores each episode under an intuitive, greppable path:
//
//	podcasts/<show-slug>/<episode-slug>/summary.md
//	podcasts/<show-slug>/<episode-slug>/bundle.zip
//
// Paths are always forward-slash and repo-relative so they are portable and can be
// handed directly to a git worktree regardless of the host operating system.
package layout

import (
	manifest "github.com/jo-hoe/manifest-lib"
)

// Canonical archive constants.
const (
	// PodcastsDir is the top-level directory under which all shows are stored.
	PodcastsDir = "podcasts"
	// SummaryName is the file name of the (unzipped) episode summary.
	SummaryName = "summary.md"
	// BundleName is the file name of the zipped episode bundle.
	BundleName = "bundle.zip"
)

// EpisodeDir returns the repo-relative directory for an episode of a show.
func EpisodeDir(showTitle, episodeID string) string {
	return join(PodcastsDir, manifest.Slugify(showTitle), episodeID)
}

// SummaryPath returns the repo-relative path of the episode summary.
func SummaryPath(showTitle, episodeID string) string {
	return join(EpisodeDir(showTitle, episodeID), SummaryName)
}

// BundlePath returns the repo-relative path of the episode bundle.
func BundlePath(showTitle, episodeID string) string {
	return join(EpisodeDir(showTitle, episodeID), BundleName)
}

// join concatenates path segments with forward slashes.
func join(parts ...string) string {
	out := ""
	for _, p := range parts {
		if out == "" {
			out = p
			continue
		}
		out += "/" + p
	}
	return out
}
