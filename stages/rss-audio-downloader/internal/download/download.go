// Package download fetches episode audio files over HTTP to a local path.
package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Downloader fetches the resource at url and writes it to destPath.
type Downloader interface {
	Download(ctx context.Context, url, destPath string) error
}

// HTTPDownloader is the production Downloader backed by an http.Client. It writes
// to a temp file and renames it into place so an interrupted download never leaves
// a partial file that a later stage would treat as complete.
type HTTPDownloader struct {
	client *http.Client
}

// NewHTTPDownloader constructs an HTTPDownloader with a sane default timeout.
func NewHTTPDownloader() *HTTPDownloader {
	return &HTTPDownloader{client: &http.Client{Timeout: 30 * time.Minute}}
}

// Download streams url to destPath, creating parent directories as needed.
func (d *HTTPDownloader) Download(ctx context.Context, url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build request for %s: %w", url, err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d downloading %s", resp.StatusCode, url)
	}

	return writeAtomically(destPath, resp.Body)
}

func writeAtomically(destPath string, r io.Reader) error {
	tmp := destPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp file %s: %w", tmp, err)
	}

	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("failed to write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		return fmt.Errorf("failed to finalize %s: %w", destPath, err)
	}
	return nil
}
