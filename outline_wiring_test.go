package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupManuscript(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("three"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

func TestCtrlLEntersOutlineInManuscript(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("ctrl+l in a manuscript should enter screenOutline, got %v", m.screen)
	}
	if len(m.outline.working) != 2 {
		t.Fatalf("outline should load 2 sections, got %d", len(m.outline.working))
	}
}

func TestCtrlLRejectedOutsideManuscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "loose.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(root)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen == screenOutline {
		t.Fatal("ctrl+l outside a manuscript should not enter the outline")
	}
}

func TestOutlineEnterOpensSection(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select 02-b
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should return to the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("Enter should open the selected section, currentFile = %q", m.currentFile)
	}
}

func TestOutlineEscReturnsToEditor(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("esc from the outline should return to the editor, got %v", m.screen)
	}
}

func TestOutlineHandlesResize(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m = nm.(model)
	if m.outline.width != 70 || m.outline.height != 19 {
		t.Fatalf("resize on the outline should update outline dims to 70x19, got %dx%d", m.outline.width, m.outline.height)
	}
}

func TestOutlineClickSelectsThenDoubleClickOpens(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a (row 0), 02-b (row 1)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// Click row 1 (02-b): mouse Y = header height + 1.
	clickY := outlineHeaderHeight + 1
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.outline.selected != 1 {
		t.Fatalf("click should select row 1, got %d", m.outline.selected)
	}
	// Second click on the same row opens it.
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.screen != screenWriting || m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("double-click should open 02-b.md, screen=%v file=%q", m.screen, m.currentFile)
	}
}
