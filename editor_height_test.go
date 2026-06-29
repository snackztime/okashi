package main

import (
	"strings"
	"testing"
)

// Regression: the editor must NOT cap content height. The vendored textarea
// defaults MaxHeight to 99; okashi sets it to 0 (unlimited). With the cap,
// loading a chapter over 99 lines silently truncated it (~2991 words of prose)
// and Enter was blocked at the cap. See main.go (ta.MaxHeight = 0).
func TestEditorHasNoHeightCap(t *testing.T) {
	m := initialModel()
	if m.editor.MaxHeight != 0 {
		t.Fatalf("editor MaxHeight = %d, want 0 (unlimited) — long chapters get truncated", m.editor.MaxHeight)
	}

	// A 300-line document must survive SetValue intact (default cap was 99).
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteString("a line of prose\n")
	}
	m.editor.SetValue(b.String())
	if got := strings.Count(m.editor.Value(), "\n"); got < 299 {
		t.Fatalf("SetValue kept %d of 300 newlines — content was truncated by a height cap", got)
	}
}
