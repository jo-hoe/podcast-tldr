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

// Save writes the manifest to FileName inside the given work directory. It uses
// a cross-process file lock and a read-modify-write strategy so parallel fan-out
// chains on a shared volume never overwrite each other's episode fields.
//
// The merge strategy: for each episode in m, apply its non-zero fields onto the
// on-disk manifest (re-read under the lock). Fields already set on disk and absent
// in m are preserved. This means each stage only needs to carry its own episode
// fields — it won't clobber fields written by a concurrent chain.
func (m *Manifest) Save(workDir string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("failed to create work dir %s: %w", workDir, err)
	}

	lockPath := filepath.Join(workDir, lockFileName)
	if err := acquireLock(lockPath, 60*time.Second); err != nil {
		return fmt.Errorf("failed to acquire manifest lock: %w", err)
	}
	defer releaseLock(lockPath)

	// Re-read the on-disk manifest under the lock to pick up any concurrent writes.
	merged := m
	if disk, err := loadLocked(workDir); err == nil {
		merged = mergeManifest(disk, m)
	}
	// If the file doesn't exist yet (first writer), use m as-is.

	data, err := yaml.Marshal(merged)
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

// loadLocked reads the manifest without acquiring the lock (caller holds it).
func loadLocked(workDir string) (*Manifest, error) {
	path := filepath.Join(workDir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// mergeManifest merges the updates from src into base. For each episode, non-zero
// fields from src overwrite the corresponding fields in base, preserving everything
// else. This allows parallel fan-out chains to each write only their stage's fields
// without clobbering concurrent writes by other chains.
func mergeManifest(base, src *Manifest) *Manifest {
	// Index base episodes by ID for O(1) lookup.
	type key struct{ feed, id string }
	type epRef struct{ pi, ei int }
	idx := make(map[string]epRef)
	for pi := range base.Podcasts {
		for ei := range base.Podcasts[pi].Episodes {
			ep := &base.Podcasts[pi].Episodes[ei]
			idx[ep.ID] = epRef{pi, ei}
		}
	}

	for _, srcPod := range src.Podcasts {
		for _, srcEp := range srcPod.Episodes {
			ref, ok := idx[srcEp.ID]
			if !ok {
				// Episode not in base — shouldn't happen in normal usage, skip.
				continue
			}
			dst := &base.Podcasts[ref.pi].Episodes[ref.ei]
			mergeEpisode(dst, &srcEp)
		}
	}
	return base
}

// mergeEpisode applies non-zero fields from src onto dst.
func mergeEpisode(dst, src *Episode) {
	if src.AudioFile != "" {
		dst.AudioFile = src.AudioFile
	}
	if src.TranscriptFile != "" {
		dst.TranscriptFile = src.TranscriptFile
	}
	if src.Language != "" {
		dst.Language = src.Language
	}
	if src.TranscribeModel != "" {
		dst.TranscribeModel = src.TranscribeModel
	}
	if src.SummaryFile != "" {
		dst.SummaryFile = src.SummaryFile
	}
	if src.SummaryModel != "" {
		dst.SummaryModel = src.SummaryModel
	}
	if src.BundleFile != "" {
		dst.BundleFile = src.BundleFile
	}
	if src.BackedUp {
		dst.BackedUp = true
	}
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
