package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
	return path
}

func TestLoad_ReturnsTrimmedInstruction(t *testing.T) {
	path := writeFile(t, "prompt.txt", "\n  Summarize the transcript.  \n")
	tmpl, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if tmpl.Instruction != "Summarize the transcript." {
		t.Errorf("Instruction = %q, want trimmed value", tmpl.Instruction)
	}
}

func TestLoad_RejectsEmptyFile(t *testing.T) {
	path := writeFile(t, "prompt.txt", "   \n\t\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
}

func TestLoad_RejectsMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent.txt")); err == nil {
		t.Fatal("expected error for missing prompt, got nil")
	}
}

func TestTranscriptText_PrefersTextField(t *testing.T) {
	path := writeFile(t, "t.json", `{"text":"full transcript","segments":[{"start":0,"end":1,"text":"ignored"}]}`)
	got, err := TranscriptText(path)
	if err != nil {
		t.Fatalf("TranscriptText failed: %v", err)
	}
	if got != "full transcript" {
		t.Errorf("got %q, want text field to win", got)
	}
}

func TestTranscriptText_FallsBackToSegments(t *testing.T) {
	path := writeFile(t, "t.json", `{"segments":[{"start":0,"end":1,"text":"hello"},{"start":1,"end":2,"text":"world"}]}`)
	got, err := TranscriptText(path)
	if err != nil {
		t.Fatalf("TranscriptText failed: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want joined segments", got)
	}
}

func TestTranscriptText_RejectsEmptyDocument(t *testing.T) {
	path := writeFile(t, "t.json", `{"segments":[]}`)
	if _, err := TranscriptText(path); err == nil {
		t.Fatal("expected error for empty transcript, got nil")
	}
}

func TestTranscriptText_RejectsInvalidJSON(t *testing.T) {
	path := writeFile(t, "t.json", `{not json`)
	if _, err := TranscriptText(path); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
