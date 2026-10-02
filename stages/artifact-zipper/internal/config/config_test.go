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
	path := writeConfig(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default logLevel info, got %s", cfg.LogLevel)
	}
	if !filepath.IsAbs(cfg.WorkDir) {
		t.Errorf("expected workDir to be absolute, got %s", cfg.WorkDir)
	}
}

func TestLoad_ParsesExplicitValues(t *testing.T) {
	// Use an OS-absolute path so makePathsAbsolute leaves it untouched on any
	// platform (on Windows a leading-slash path is not absolute).
	absWork := filepath.Join(t.TempDir(), "work")
	path := writeConfig(t, "logLevel: debug\nworkDir: "+filepath.ToSlash(absWork)+"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("logLevel = %s", cfg.LogLevel)
	}
	if filepath.Clean(cfg.WorkDir) != filepath.Clean(absWork) {
		t.Errorf("workDir = %s, want %s", cfg.WorkDir, absWork)
	}
}

func TestLoad_ResolvesRelativeWorkDirToAbsolute(t *testing.T) {
	path := writeConfig(t, "workDir: relative/work\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !filepath.IsAbs(cfg.WorkDir) {
		t.Errorf("expected relative workDir to be resolved to absolute, got %s", cfg.WorkDir)
	}
}

func TestLoad_MissingFileIsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestResolvePath_EnvOverride(t *testing.T) {
	t.Setenv(EnvConfigPath, "/custom/path.yaml")
	got, err := ResolvePath()
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if got != "/custom/path.yaml" {
		t.Errorf("expected env override, got %s", got)
	}
}
