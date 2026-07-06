package main

import (
	"os"
	"path/filepath"
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

func TestOutlinePromoteBeat(t *testing.T) {
	m, dir := outlineOnManuscript(t) // seedCorkManuscript starts with 3 chapters
	m.editor.SetValue("- New Chapter\n  - a note\n  - two")
	m.editor.MoveToLine(0)

	nm, _ := m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = nm.(model)

	mani, present, err := readManifest(dir)
	if err != nil || !present {
		t.Fatalf("manifest unreadable: %v", err)
	}
	if len(mani.Items) != 4 {
		t.Fatalf("promote should append one chapter (want 4 items), got %d", len(mani.Items))
	}
	last := mani.Items[3]
	if last.Title != "New Chapter" {
		t.Fatalf("new chapter title = %q, want %q", last.Title, "New Chapter")
	}
	if syn := loadSynopses(dir)[last.File]; syn != "a note\ntwo" {
		t.Fatalf("synopsis seed = %q, want %q", syn, "a note\ntwo")
	}
	if line0 := strings.SplitN(m.editor.Value(), "\n", 2)[0]; line0 != "- [x] New Chapter" {
		t.Fatalf("beat should be marked promoted, got %q", line0)
	}
	// A second promote on the same (now [x]) beat is a no-op.
	m.editor.MoveToLine(0)
	nm, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = nm.(model)
	if mani2, _, _ := readManifest(dir); len(mani2.Items) != 4 {
		t.Fatalf("double-promote must not add a chapter, got %d items", len(mani2.Items))
	}
}

func TestOutlinePromoteNeedsManuscript(t *testing.T) {
	dir := t.TempDir() // plain folder, no manifest
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(model)
	m.files.SetDir(dir)
	m.enterOutline()
	m.editor.SetValue("- A beat")
	m.editor.MoveToLine(0)

	nm, _ = m.updateOutline(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = nm.(model)
	if m.status == "" || !strings.Contains(m.status, "manuscript") {
		t.Fatalf("promote in a non-manuscript should warn, status=%q", m.status)
	}
	if _, present, _ := readManifest(dir); present {
		t.Fatal("promote must not create a manifest in a plain folder")
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
