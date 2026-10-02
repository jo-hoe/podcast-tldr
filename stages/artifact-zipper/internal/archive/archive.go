// Package archive builds zip bundles from a set of named entries using the
// standard library archive/zip. It is deliberately small and free of any
// pipeline-specific knowledge so it can be unit-tested in isolation.
package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Entry is a single member of a zip bundle. Exactly one of SourcePath or Data
// supplies the content: SourcePath streams an existing file from disk, while Data
// provides in-memory bytes (used for generated files such as metadata.yaml).
type Entry struct {
	// Name is the path of the entry inside the zip (forward slashes).
	Name string
	// SourcePath, when non-empty, is the on-disk file to copy into the entry.
	SourcePath string
	// Data, used when SourcePath is empty, is the in-memory content of the entry.
	Data []byte
}

// Writer creates zip bundles. It is an interface so the service layer can be
// tested against a fake without touching the filesystem.
type Writer interface {
	// Write creates a zip archive at destPath containing the given entries,
	// compressing each with Deflate. Parent directories are created as needed.
	Write(destPath string, entries []Entry) error
}

// ZipWriter is the production Writer backed by archive/zip.
type ZipWriter struct{}

// NewZipWriter constructs a ZipWriter.
func NewZipWriter() *ZipWriter { return &ZipWriter{} }

// Write implements Writer. It writes to a temp file that is renamed into place so
// an interrupted run never leaves a partial bundle a later stage would trust.
func (ZipWriter) Write(destPath string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
	}

	tmp := destPath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("failed to create temp bundle %s: %w", tmp, err)
	}

	if err := writeEntries(f, entries); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to close bundle %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		return fmt.Errorf("failed to finalize bundle %s: %w", destPath, err)
	}
	return nil
}

func writeEntries(w io.Writer, entries []Entry) error {
	zw := zip.NewWriter(w)
	for _, e := range entries {
		if err := writeEntry(zw, e); err != nil {
			zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("failed to finalize zip stream: %w", err)
	}
	return nil
}

func writeEntry(zw *zip.Writer, e Entry) error {
	header := &zip.FileHeader{Name: e.Name, Method: zip.Deflate}
	dst, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to add entry %s: %w", e.Name, err)
	}
	if e.SourcePath != "" {
		return copyFile(dst, e.SourcePath, e.Name)
	}
	if _, err := dst.Write(e.Data); err != nil {
		return fmt.Errorf("failed to write entry %s: %w", e.Name, err)
	}
	return nil
}

func copyFile(dst io.Writer, srcPath, name string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source %s for entry %s: %w", srcPath, name, err)
	}
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy source %s into entry %s: %w", srcPath, name, err)
	}
	return nil
}
