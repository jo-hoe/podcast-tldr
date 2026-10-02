package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestLoad_AppliesDefaults(t *testing.T) {
	path := writeConfig(t, `
baseURL: http://litellm:4000/v1
model: summarizer
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %s", cfg.LogLevel)
	}
	if cfg.APIKeyEnv != defaultAPIKeyEnv {
		t.Errorf("expected default apiKeyEnv %s, got %s", defaultAPIKeyEnv, cfg.APIKeyEnv)
	}
	if cfg.MaxTokens != 1024 {
		t.Errorf("expected default maxTokens 1024, got %d", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("expected default temperature 0.3, got %v", cfg.Temperature)
	}
	if !filepath.IsAbs(cfg.WorkDir) || !filepath.IsAbs(cfg.PromptPath) {
		t.Errorf("expected workDir and promptPath absolute, got %s %s", cfg.WorkDir, cfg.PromptPath)
	}
}

func TestLoad_ParsesOverrides(t *testing.T) {
	path := writeConfig(t, `
logLevel: debug
workDir: /tmp/work
baseURL: http://proxy/v1
model: gpt-summarizer
apiKeyEnv: MY_KEY
promptPath: /tmp/prompt.txt
maxTokens: 2048
temperature: 0.7
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.APIKeyEnv != "MY_KEY" || cfg.Model != "gpt-summarizer" {
		t.Errorf("overrides not parsed: %+v", cfg)
	}
	if cfg.MaxTokens != 2048 || cfg.Temperature != 0.7 {
		t.Errorf("numeric overrides not parsed: %+v", cfg)
	}
}

func TestAPIKey_ReadsNamedEnvVar(t *testing.T) {
	t.Setenv("MY_KEY", "secret-token")
	cfg := &Config{APIKeyEnv: "MY_KEY"}
	if got := cfg.APIKey(); got != "secret-token" {
		t.Errorf("APIKey = %q, want secret-token", got)
	}
}

func TestLoad_RejectsMissingBaseURL(t *testing.T) {
	path := writeConfig(t, "model: summarizer\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing baseURL, got nil")
	}
}

func TestLoad_RejectsMissingModel(t *testing.T) {
	path := writeConfig(t, "baseURL: http://litellm:4000/v1\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
}

func TestLoad_RejectsNegativeTemperature(t *testing.T) {
	path := writeConfig(t, `
baseURL: http://litellm:4000/v1
model: summarizer
temperature: -1
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for negative temperature, got nil")
	}
}

func TestLoad_ReasoningModelRaisesDefaultMaxTokens(t *testing.T) {
	path := writeConfig(t, `
baseURL: https://ai-proxy.example/openai/v1
model: gpt-5
reasoningModel: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.ReasoningModel {
		t.Error("expected reasoningModel true")
	}
	if cfg.MaxTokens != 8000 {
		t.Errorf("expected default maxTokens 8000 for reasoning model, got %d", cfg.MaxTokens)
	}
}

func TestLoad_ReasoningModelHonorsExplicitMaxTokens(t *testing.T) {
	path := writeConfig(t, `
baseURL: https://ai-proxy.example/openai/v1
model: gpt-5
reasoningModel: true
maxTokens: 4096
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("expected explicit maxTokens 4096, got %d", cfg.MaxTokens)
	}
}
