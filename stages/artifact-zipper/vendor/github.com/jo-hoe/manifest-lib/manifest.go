// Package manifest defines the language-neutral contract shared by every stage of
// the podcast-tldr pipeline. A single episodes.yaml file lives in a shared work
// directory and flows through the stages (download -> transcribe -> summarize ->
// zip -> backup). Each stage reads the manifest, produces its artifacts into the
// work directory, appends the fields it owns, and writes the manifest back.
//
// Stages MUST treat fields owned by other stages as read-only: only append, never
// mutate upstream data. All artifact paths in the manifest are relative to the
// work directory so the manifest is portable across mounts and containers.
package manifest

import "time"

// FileName is the canonical name of the manifest file inside the work directory.
const FileName = "episodes.yaml"

// Manifest is the root document that flows through the pipeline.
type Manifest struct {
	// Podcasts holds every show and its selected episodes.
	Podcasts []Podcast `yaml:"podcasts"`
}

// Podcast is a single show together with the episodes selected from its feed.
type Podcast struct {
	ShowTitle       string    `yaml:"showTitle"`
	ShowDescription string    `yaml:"showDescription,omitempty"`
	FeedURL         string    `yaml:"feedURL"`
	Author          string    `yaml:"author,omitempty"`
	Episodes        []Episode `yaml:"episodes"`
}

// Episode is one podcast episode and the artifacts produced for it. Fields are
// grouped by the stage that owns them; a stage only writes the fields in its group.
type Episode struct {
	// --- Stage A (download): feed-derived metadata + downloaded audio ---
	ID          string    `yaml:"id"`                    // stable slug, drives folder naming
	Title       string    `yaml:"title"`                 //
	Published   time.Time `yaml:"published,omitempty"`   // publishing date (RFC3339)
	Description string    `yaml:"description,omitempty"` //
	AudioURL    string    `yaml:"audioURL,omitempty"`    // enclosure URL from the feed
	AudioFile   string    `yaml:"audioFile,omitempty"`   // work-relative path, set after download
	Duration    string    `yaml:"duration,omitempty"`    // iTunes duration, if provided

	// --- Stage B (transcribe) ---
	TranscriptFile  string `yaml:"transcriptFile,omitempty"`
	Language        string `yaml:"language,omitempty"`        // detected language
	TranscribeModel string `yaml:"transcribeModel,omitempty"` // model used

	// --- Stage C (summarize) ---
	SummaryFile  string `yaml:"summaryFile,omitempty"`
	SummaryModel string `yaml:"summaryModel,omitempty"`

	// --- Stage D (zip) ---
	BundleFile string `yaml:"bundleFile,omitempty"`

	// --- Stage E (backup) ---
	BackedUp bool `yaml:"backedUp,omitempty"`
}
