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
	path := writeConfig(t, "repoURL: https://example.com/archive.git\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %s", cfg.LogLevel)
	}
	if cfg.Branch != defaultBranch {
		t.Errorf("expected default branch %s, got %s", defaultBranch, cfg.Branch)
	}
	if cfg.TokenEnv != defaultTokenEnv {
		t.Errorf("expected default tokenEnv %s, got %s", defaultTokenEnv, cfg.TokenEnv)
	}
	if cfg.AuthorName != defaultAuthorName {
		t.Errorf("expected default authorName %s, got %s", defaultAuthorName, cfg.AuthorName)
	}
	if cfg.AuthorEmail != defaultAuthorEmail {
		t.Errorf("expected default authorEmail %s, got %s", defaultAuthorEmail, cfg.AuthorEmail)
	}
	if !filepath.IsAbs(cfg.WorkDir) {
		t.Errorf("expected workDir to be absolute, got %s", cfg.WorkDir)
	}
	if !filepath.IsAbs(cfg.CheckoutDir) {
		t.Errorf("expected checkoutDir to be absolute, got %s", cfg.CheckoutDir)
	}
}

func TestLoad_CheckoutDefaultsUnderWorkDir(t *testing.T) {
	path := writeConfig(t, `
workDir: /tmp/work
repoURL: https://example.com/archive.git
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	want := filepath.Join(cfg.WorkDir, defaultCheckoutRel)
	if cfg.CheckoutDir != want {
		t.Errorf("expected checkoutDir %s, got %s", want, cfg.CheckoutDir)
	}
}

func TestLoad_OverridesValues(t *testing.T) {
	path := writeConfig(t, `
logLevel: debug
repoURL: https://example.com/archive.git
tokenEnv: MY_TOKEN
branch: archive
authorName: Alice
authorEmail: alice@example.com
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("logLevel not applied: %s", cfg.LogLevel)
	}
	if cfg.TokenEnv != "MY_TOKEN" {
		t.Errorf("tokenEnv not applied: %s", cfg.TokenEnv)
	}
	if cfg.Branch != "archive" {
		t.Errorf("branch not applied: %s", cfg.Branch)
	}
	if cfg.AuthorName != "Alice" || cfg.AuthorEmail != "alice@example.com" {
		t.Errorf("author not applied: %s <%s>", cfg.AuthorName, cfg.AuthorEmail)
	}
}

func TestLoad_RejectsMissingRepoURL(t *testing.T) {
	path := writeConfig(t, "logLevel: debug\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing repoURL, got nil")
	}
}

func TestToken_ReadsFromEnv(t *testing.T) {
	t.Setenv("MY_TOKEN", "s3cr3t")
	cfg := &Config{TokenEnv: "MY_TOKEN"}
	if got := cfg.Token(); got != "s3cr3t" {
		t.Errorf("expected token s3cr3t, got %q", got)
	}
}

func TestResolvePath_UsesEnvOverride(t *testing.T) {
	t.Setenv(EnvConfigPath, "/custom/config.yaml")
	got, err := ResolvePath()
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if got != "/custom/config.yaml" {
		t.Errorf("expected env override path, got %s", got)
	}
}
