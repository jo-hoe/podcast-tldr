// Package prompt loads the summarization prompt template and extracts readable
// transcript text from the JSON produced by the transcribe stage.
package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Template is the summarization instruction loaded from the prompt file. It is the
// system/user instruction; the transcript text is supplied separately.
type Template struct {
	Instruction string
}

// Load reads the prompt template file. A missing or empty file is an error: the
// summary quality depends entirely on the prompt, so we fail loudly rather than
// silently summarizing with no guidance.
func Load(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt template %s: %w", path, err)
	}
	instruction := strings.TrimSpace(string(data))
	if instruction == "" {
		return nil, fmt.Errorf("prompt template %s is empty", path)
	}
	return &Template{Instruction: instruction}, nil
}

// transcriptDocument is the transcribe-stage JSON shape. Both fields are optional;
// text is preferred and segments are the fallback.
type transcriptDocument struct {
	Text     string              `json:"text"`
	Segments []transcriptSegment `json:"segments"`
}

type transcriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// TranscriptText reads a transcript JSON file and returns its plain text. It uses
// the top-level "text" field when present, otherwise joins the segment texts. It
// is defensive: unknown fields are ignored and whitespace is normalized.
func TranscriptText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read transcript %s: %w", path, err)
	}

	var doc transcriptDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("failed to parse transcript %s: %w", path, err)
	}

	text := joinTranscript(doc)
	if text == "" {
		return "", fmt.Errorf("transcript %s contained no text", path)
	}
	return text, nil
}

func joinTranscript(doc transcriptDocument) string {
	if t := strings.TrimSpace(doc.Text); t != "" {
		return t
	}
	parts := make([]string, 0, len(doc.Segments))
	for _, seg := range doc.Segments {
		if s := strings.TrimSpace(seg.Text); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}
