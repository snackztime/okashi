package main

import (
	"strings"
	"testing"

	"okashi/internal/textarea"

	"github.com/charmbracelet/lipgloss"
)

// newBenchEditor builds a textarea configured the way okashi does.
func newBenchEditor() textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.SetWidth(72)
	ta.SetHeight(40)
	ta.Focus()
	return ta
}

// editorContent returns `paras` paragraphs of ~60 words each (one source line
// per paragraph), the same shape as the pager corpus.
func editorContent(paras int) string {
	line := strings.TrimSpace(strings.Repeat("lorem ", 60))
	var b strings.Builder
	for p := 0; p < paras; p++ {
		if p > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(line)
	}
	return b.String()
}

// benchEditorView measures one frame render at a given buffer size.
func benchEditorView(b *testing.B, paras int) {
	ed := newBenchEditor()
	ed.SetValue(editorContent(paras))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ed.View()
	}
}

// One chapter (~40 paragraphs ≈ 2.4k words ≈ 10 pages).
func BenchmarkEditorViewChapter(b *testing.B) { benchEditorView(b, 40) }

// The whole 400-page draft in ONE file (~1600 paragraphs ≈ 96k words).
func BenchmarkEditorViewWholeDraft(b *testing.B) { benchEditorView(b, 1600) }

// A mid-size single file (~150 pages) to show the curve.
func BenchmarkEditorViewHalfDraft(b *testing.B) { benchEditorView(b, 600) }
