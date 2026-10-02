// Package config loads and validates the backup stage configuration from YAML.
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

// Default values applied when the corresponding config field is empty.
const (
	defaultBranch      = "main"
	defaultTokenEnv    = "PODCAST_TLDR_BACKUP_TOKEN"
	defaultAuthorName  = "git-archive-backup"
	defaultAuthorEmail = "git-archive-backup@users.noreply.github.com"
	defaultCheckoutRel = ".backup-checkout"
)

// Config is the root backup-stage configuration.
type Config struct {
	// LogLevel is one of debug, info, warn, error. Defaults to info.
	LogLevel string `yaml:"logLevel"`
	// WorkDir is the shared work directory holding episodes.yaml and artifacts.
	WorkDir string `yaml:"workDir"`
	// RepoURL is the HTTPS URL of the private git archive repository. Required.
	RepoURL string `yaml:"repoURL"`
	// TokenEnv names the environment variable that holds the git access token.
	TokenEnv string `yaml:"tokenEnv"`
	// Branch is the branch to commit and push to. Defaults to main.
	Branch string `yaml:"branch"`
	// AuthorName is the git commit author name.
	AuthorName string `yaml:"authorName"`
	// AuthorEmail is the git commit author email.
	AuthorEmail string `yaml:"authorEmail"`
	// CheckoutDir is where the archive repo is cloned. Defaults to a directory
	// under WorkDir so it is co-located with the artifacts being backed up.
	CheckoutDir string `yaml:"checkoutDir"`
}

// Token returns the git access token read from the configured environment
// variable. It is not read from the config file so the secret never lands on disk.
func (c *Config) Token() string {
	return os.Getenv(c.TokenEnv)
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
	if strings.TrimSpace(cfg.Branch) == "" {
		cfg.Branch = defaultBranch
	}
	if strings.TrimSpace(cfg.TokenEnv) == "" {
		cfg.TokenEnv = defaultTokenEnv
	}
	if strings.TrimSpace(cfg.AuthorName) == "" {
		cfg.AuthorName = defaultAuthorName
	}
	if strings.TrimSpace(cfg.AuthorEmail) == "" {
		cfg.AuthorEmail = defaultAuthorEmail
	}
	if strings.TrimSpace(cfg.CheckoutDir) == "" {
		cfg.CheckoutDir = filepath.Join(cfg.WorkDir, defaultCheckoutRel)
	}
}

func makePathsAbsolute(cfg *Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}
	cfg.WorkDir = absFromRoot(cfg.WorkDir, cwd)
	cfg.CheckoutDir = absFromRoot(cfg.CheckoutDir, cwd)
	return nil
}

func absFromRoot(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func validate(cfg *Config) error {
	if strings.TrimSpace(cfg.RepoURL) == "" {
		return fmt.Errorf("repoURL must not be empty")
	}
	return nil
}

func logLoadedConfig(cfg *Config) {
	slog.Info("=== Configuration Loaded ===")
	slog.Info("Log Level", "value", cfg.LogLevel)
	slog.Info("Work Dir", "value", cfg.WorkDir)
	slog.Info("Repo URL", "value", cfg.RepoURL)
	slog.Info("Token Env", "value", cfg.TokenEnv)
	slog.Info("Branch", "value", cfg.Branch)
	slog.Info("Author", "name", cfg.AuthorName, "email", cfg.AuthorEmail)
	slog.Info("Checkout Dir", "value", cfg.CheckoutDir)
	slog.Info("============================")
}
