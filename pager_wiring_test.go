package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func manuscriptModel(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha beta\ngamma"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("delta"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

func TestOutlineMEntersPager(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // pager
	m = nm.(model)
	if m.screen != screenManuscript {
		t.Fatalf("m in the outline should enter the pager, got screen %v", m.screen)
	}
	if len(m.pager.lines) == 0 {
		t.Fatal("the pager should be built on entry")
	}
}

func TestPagerEnterJumpsToEditAtLine(t *testing.T) {
	m, proj := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// Move the cursor to the "gamma" line (header, alpha, gamma -> index 2).
	m.pager.cursor = 2
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should jump into the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("Enter should open the mapped section, currentFile = %q", m.currentFile)
	}
	if m.editor.Line() != 1 {
		t.Fatalf("Enter should place the editor cursor on source line 1 (gamma), got %d", m.editor.Line())
	}
}

func TestPagerOGoesToOutlineEscToEditor(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("o should return to the outline, got %v", m.screen)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // back to pager
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("esc should return to the editor, got %v", m.screen)
	}
}
