// Package config loads and validates the summarize stage configuration from YAML.
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

// defaultAPIKeyEnv is the environment variable read for the LLM API key when the
// config does not name a different one.
const defaultAPIKeyEnv = "LITELLM_API_KEY"

// Config is the root summarize-stage configuration.
type Config struct {
	// LogLevel is one of debug, info, warn, error. Defaults to info.
	LogLevel string `yaml:"logLevel"`
	// WorkDir holds episodes.yaml plus the transcripts to read and summaries to write.
	WorkDir string `yaml:"workDir"`
	// BaseURL is the OpenAI-compatible endpoint, typically a LiteLLM proxy, e.g.
	// http://litellm:4000/v1. Required.
	BaseURL string `yaml:"baseURL"`
	// Model is the model name/alias configured on the proxy. Required.
	Model string `yaml:"model"`
	// ReasoningModel selects reasoning-model request semantics (e.g. gpt-5): the
	// token budget is sent as max_completion_tokens and temperature is omitted,
	// because such models reject max_tokens and any non-default temperature.
	ReasoningModel bool `yaml:"reasoningModel"`
	// APIKeyEnv names the environment variable that holds the API key. The key
	// itself is never stored in config. Defaults to LITELLM_API_KEY.
	APIKeyEnv string `yaml:"apiKeyEnv"`
	// PromptPath points to the prompt template file. Required.
	PromptPath string `yaml:"promptPath"`
	// MaxTokens bounds the summary length. Defaults to 1024.
	MaxTokens int `yaml:"maxTokens"`
	// Temperature controls sampling randomness. Defaults to 0.3.
	Temperature float32 `yaml:"temperature"`
}

// APIKey returns the API key read from the environment variable named by
// APIKeyEnv. It may be empty when the proxy requires no authentication.
func (c *Config) APIKey() string {
	return os.Getenv(c.APIKeyEnv)
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
	if strings.TrimSpace(cfg.APIKeyEnv) == "" {
		cfg.APIKeyEnv = defaultAPIKeyEnv
	}
	if strings.TrimSpace(cfg.PromptPath) == "" {
		cfg.PromptPath = filepath.Join(".", "config", "prompt.txt")
	}
	if cfg.MaxTokens <= 0 {
		if cfg.ReasoningModel {
			// Reasoning models spend most of the budget on hidden reasoning tokens
			// before emitting any visible text, so default to a generous cap.
			cfg.MaxTokens = 8000
		} else {
			cfg.MaxTokens = 1024
		}
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.3
	}
}

// makePathsAbsolute resolves WorkDir and PromptPath relative to the current
// working directory so the stage behaves the same regardless of where it is run.
func makePathsAbsolute(cfg *Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine working directory: %w", err)
	}
	cfg.WorkDir = absFromRoot(cfg.WorkDir, cwd)
	cfg.PromptPath = absFromRoot(cfg.PromptPath, cwd)
	return nil
}

func absFromRoot(path, root string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func validate(cfg *Config) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("baseURL must not be empty")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("model must not be empty")
	}
	if cfg.Temperature < 0 {
		return fmt.Errorf("temperature must not be negative")
	}
	return nil
}

func logLoadedConfig(cfg *Config) {
	slog.Info("=== Configuration Loaded ===")
	slog.Info("Log Level", "value", cfg.LogLevel)
	slog.Info("Work Dir", "value", cfg.WorkDir)
	slog.Info("Base URL", "value", cfg.BaseURL)
	slog.Info("Model", "value", cfg.Model)
	slog.Info("Reasoning Model", "value", cfg.ReasoningModel)
	slog.Info("API Key Env", "value", cfg.APIKeyEnv)
	slog.Info("Prompt Path", "value", cfg.PromptPath)
	slog.Info("Max Tokens", "value", cfg.MaxTokens)
	slog.Info("Temperature", "value", cfg.Temperature)
	slog.Info("============================")
}
