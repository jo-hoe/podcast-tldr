package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const lockFileName = "episodes.yaml.lock"

// Load reads and parses the manifest from FileName inside the given work directory.
// A missing file is reported as an error; callers that want to start a fresh
// pipeline should construct an empty Manifest instead.
func Load(workDir string) (*Manifest, error) {
	path := filepath.Join(workDir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest %s: %w", path, err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest %s: %w", path, err)
	}
	return &m, nil
}

// Save writes the manifest to FileName inside the given work directory, creating
// the directory if necessary. The write is atomic: it goes to a temp file that is
// renamed into place so a crash cannot leave a half-written manifest behind.
// A cross-process advisory lock (lockfile) serialises concurrent writers so
// parallel fan-out chains on a shared volume do not corrupt the manifest.
func (m *Manifest) Save(workDir string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("failed to create work dir %s: %w", workDir, err)
	}

	lockPath := filepath.Join(workDir, lockFileName)
	if err := acquireLock(lockPath, 60*time.Second); err != nil {
		return fmt.Errorf("failed to acquire manifest lock: %w", err)
	}
	defer releaseLock(lockPath)

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	path := filepath.Join(workDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to finalize manifest %s: %w", path, err)
	}
	return nil
}

// acquireLock spins on creating an exclusive lockfile, retrying with backoff
// up to timeout. Uses O_EXCL (atomic create) for cross-process safety on
// shared filesystems (works on Linux/Docker volume mounts).
func acquireLock(lockPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	sleep := 50 * time.Millisecond
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return nil
		}
		if !os.IsExist(err) {
			return fmt.Errorf("unexpected lock error: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for manifest lock after %s", timeout)
		}
		time.Sleep(sleep)
		if sleep < 2*time.Second {
			sleep *= 2
		}
	}
}

func releaseLock(lockPath string) {
	_ = os.Remove(lockPath)
}
