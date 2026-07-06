package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// outlineOnManuscript returns a model opened in the outline mode of a fresh manifest manuscript.
func outlineOnManuscript(t *testing.T) (model, string) {
	t.Helper()
	dir := seedCorkManuscript(t)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(model)
	m.files.SetDir(dir)
	m.enterOutline()
	return m, dir
}

func TestOutlineMoveBeatKeys(t *testing.T) {
	m, _ := outlineOnManuscript(t)
	m.editor.SetValue("- Alpha\n  - a1\n- Beta\n  - b1")
	m.editor.MoveToLine(2) // on "- Beta"

	nm, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	m = nm.(model)
	if got := m.editor.Value(); !strings.HasPrefix(got, "- Beta\n  - b1\n- Alpha") {
		t.Fatalf("alt+up should move Beta above Alpha, got:\n%s", got)
	}
	if m.editor.Line() != 0 {
		t.Fatalf("cursor should follow the moved beat to line 0, got %d", m.editor.Line())
	}

	nm, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	m = nm.(model)
	if got := m.editor.Value(); !strings.HasPrefix(got, "- Alpha\n  - a1\n- Beta") {
		t.Fatalf("alt+down should restore the order, got:\n%s", got)
	}
}

func TestSplitPrefix(t *testing.T) {
	cases := []struct{ name, digits, rest string }{
		{"02-the-letter.md", "02", "-the-letter.md"},
		{"1-x.md", "1", "-x.md"},
		{"notes.md", "", "notes.md"},
		{"01.md", "01", ".md"},
	}
	for _, c := range cases {
		d, r := splitPrefix(c.name)
		if d != c.digits || r != c.rest {
			t.Errorf("splitPrefix(%q) = (%q,%q), want (%q,%q)", c.name, d, r, c.digits, c.rest)
		}
	}
}

func TestProjectTitle(t *testing.T) {
	cases := map[string]string{
		"my-novel":          "my novel",
		"2024-trip-journal": "2024 trip journal", // leading digits NOT stripped
		"Essays_draft":      "Essays draft",
	}
	for in, want := range cases {
		if got := projectTitle(in); got != want {
			t.Errorf("projectTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
