package archive

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// readZip returns a map of entry name -> content for the zip at path.
func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("failed to open zip %s: %v", path, err)
	}
	defer r.Close()

	out := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open entry %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(data)
	}
	return out
}

func TestWrite_IncludesDataAndFileEntries(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "transcript.json")
	if err := os.WriteFile(srcPath, []byte(`{"text":"hello"}`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	dest := filepath.Join(dir, "sub", "bundle.zip")
	entries := []Entry{
		{Name: "metadata.yaml", Data: []byte("title: Example\n")},
		{Name: "transcript.json", SourcePath: srcPath},
	}
	if err := NewZipWriter().Write(dest, entries); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	contents := readZip(t, dest)
	if len(contents) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(contents), contents)
	}
	if contents["metadata.yaml"] != "title: Example\n" {
		t.Errorf("metadata entry content = %q", contents["metadata.yaml"])
	}
	if contents["transcript.json"] != `{"text":"hello"}` {
		t.Errorf("transcript entry content = %q", contents["transcript.json"])
	}
}

func TestWrite_UsesDeflateCompression(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bundle.zip")
	if err := NewZipWriter().Write(dest, []Entry{{Name: "a.txt", Data: []byte("x")}}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	r, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()
	if r.File[0].Method != zip.Deflate {
		t.Errorf("expected Deflate method, got %d", r.File[0].Method)
	}
}

func TestWrite_LeavesNoPartFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bundle.zip")
	if err := NewZipWriter().Write(dest, []Entry{{Name: "a.txt", Data: []byte("x")}}); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Errorf("expected no .part file, stat err = %v", err)
	}
}

func TestWrite_MissingSourceIsError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bundle.zip")
	err := NewZipWriter().Write(dest, []Entry{{Name: "x", SourcePath: filepath.Join(dir, "missing")}})
	if err == nil {
		t.Fatal("expected error for missing source file, got nil")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Errorf("expected no bundle written on failure, stat err = %v", statErr)
	}
}
