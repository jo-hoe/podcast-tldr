// Package config loads and validates the zip stage configuration from YAML.
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

	"gopkg.in/yaml.v3"
)

// EnvConfigPath is the environment variable that overrides the config file path.
const EnvConfigPath = "CONFIG_PATH"

// Config is the root zip-stage configuration.
type Config struct {
	// LogLevel is one of debug, info, warn, error. Defaults to info.
	LogLevel string `yaml:"logLevel"`
	// WorkDir is the shared work directory that holds episodes.yaml and the
	// artifacts produced by upstream stages. Bundles are written under it.
	WorkDir string `yaml:"workDir"`
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
	if strings.TrimSpace(cfg.WorkDir) == "" {
		return fmt.Errorf("workDir must not be empty")
	}
	return nil
}

func logLoadedConfig(cfg *Config) {
	slog.Info("=== Configuration Loaded ===")
	slog.Info("Log Level", "value", cfg.LogLevel)
	slog.Info("Work Dir", "value", cfg.WorkDir)
	slog.Info("============================")
}
