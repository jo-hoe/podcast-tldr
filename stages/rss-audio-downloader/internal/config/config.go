// Package config loads and validates the download stage configuration from YAML.
//
// The loader follows the podcast-tldr convention: a single YAML file (path from
// the CONFIG_PATH env var, else ./config/config.yaml), unmarshalled into typed
// structs with camelCase keys, with defaults applied and relative paths resolved
// against the current working directory.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvConfigPath is the environment variable that overrides the config file path.
const EnvConfigPath = "CONFIG_PATH"

// Config is the root download-stage configuration.
type Config struct {
	// LogLevel is one of debug, info, warn, error. Defaults to info.
	LogLevel string `yaml:"logLevel"`
	// WorkDir is where audio files and episodes.yaml are written.
	WorkDir string `yaml:"workDir"`
	// MaxParallelDownloads bounds concurrent episode downloads. Defaults to 4.
	MaxParallelDownloads int `yaml:"maxParallelDownloads"`
	// Feeds lists the podcast feeds to download, each with its own selectors.
	Feeds []Feed `yaml:"feeds"`
}

// Feed is a single podcast RSS feed plus the selectors that limit which episodes
// are downloaded. All selectors are optional; when none are set, every episode is
// selected.
type Feed struct {
	// URL is the podcast RSS/Atom feed URL.
	URL string `yaml:"url"`
	// Selector limits which episodes of this feed are downloaded.
	Selector Selector `yaml:"selector"`
}

// Selector expresses the episode selection rules for a feed. Index bounds and
// date bounds and the name regex are combined with AND semantics: an episode must
// satisfy every set rule to be selected. Each field is independently optional.
type Selector struct {
	// StartEpisode is the 1-based chronological index of the first episode to
	// include (episode 1 = oldest/first published). 0 means unset (no lower bound).
	StartEpisode int `yaml:"startEpisode"`
	// EndEpisode is the 1-based inclusive chronological index of the last episode
	// to include. 0 means unset (no upper index bound).
	EndEpisode int `yaml:"endEpisode"`
	// StartDate includes only episodes published on or after this date. Optional.
	StartDate *time.Time `yaml:"startDate"`
	// EndDate includes only episodes published on or before this date. Optional.
	EndDate *time.Time `yaml:"endDate"`
	// NameRegex includes only episodes whose title matches this pattern. Optional.
	NameRegex string `yaml:"nameRegex"`
}

// Load reads, parses, defaults, and validates the config at the given path.
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	setDefaults(&cfg)
	if err := makePathsAbsolute(&cfg); err != nil {
		return nil, err
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}

	logLoadedConfig(&cfg)
	return &cfg, nil
}

// ResolvePath returns the config file path from CONFIG_PATH, or the default
// ./config/config.yaml under the current working directory.
func ResolvePath() (string, error) {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine working directory: %w", err)
	}
	return filepath.Join(cwd, "config", "config.yaml"), nil
}

func setDefaults(cfg *Config) {
	if strings.TrimSpace(cfg.LogLevel) == "" {
		cfg.LogLevel = "info"
	}
	if strings.TrimSpace(cfg.WorkDir) == "" {
		cfg.WorkDir = filepath.Join(".", "mount", "work")
	}
	if cfg.MaxParallelDownloads <= 0 {
		cfg.MaxParallelDownloads = 4
	}
}

func makePathsAbsolute(cfg *Config) error {
	if filepath.IsAbs(cfg.WorkDir) {
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}
	cfg.WorkDir = filepath.Join(cwd, cfg.WorkDir)
	return nil
}

func validate(cfg *Config) error {
	if len(cfg.Feeds) == 0 {
		return fmt.Errorf("config must define at least one feed")
	}
	for i, f := range cfg.Feeds {
		if strings.TrimSpace(f.URL) == "" {
			return fmt.Errorf("feeds[%d]: url must not be empty", i)
		}
		if err := f.Selector.validate(); err != nil {
			return fmt.Errorf("feeds[%d]: %w", i, err)
		}
	}
	return nil
}

func (s Selector) validate() error {
	if s.StartEpisode < 0 || s.EndEpisode < 0 {
		return fmt.Errorf("episode indexes must not be negative")
	}
	if s.StartEpisode > 0 && s.EndEpisode > 0 && s.EndEpisode < s.StartEpisode {
		return fmt.Errorf("endEpisode (%d) must be >= startEpisode (%d)", s.EndEpisode, s.StartEpisode)
	}
	if s.StartDate != nil && s.EndDate != nil && s.EndDate.Before(*s.StartDate) {
		return fmt.Errorf("endDate must not be before startDate")
	}
	return nil
}

func logLoadedConfig(cfg *Config) {
	slog.Info("=== Configuration Loaded ===")
	slog.Info("Log Level", "value", cfg.LogLevel)
	slog.Info("Work Dir", "value", cfg.WorkDir)
	slog.Info("Max Parallel Downloads", "value", cfg.MaxParallelDownloads)
	slog.Info("Feeds", "count", len(cfg.Feeds))
	slog.Info("============================")
}
