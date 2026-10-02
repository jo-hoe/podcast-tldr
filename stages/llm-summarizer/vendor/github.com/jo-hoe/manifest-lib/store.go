package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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
func (m *Manifest) Save(workDir string) error {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("failed to create work dir %s: %w", workDir, err)
	}

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
