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

func TestOutlineReorderCommitsOnEscConfirm(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// Move section 1 (01-a) down past 02-b.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	if !m.outline.dirty() {
		t.Fatal("after J the outline should be dirty")
	}
	// esc -> confirm gate appears, no disk change yet.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if !m.outline.confirm {
		t.Fatal("esc with a pending reorder should raise the confirm gate")
	}
	if _, err := os.Stat(filepath.Join(proj, "01-b.md")); !os.IsNotExist(err) {
		t.Fatal("disk must not change before the gate is confirmed")
	}
	// y -> apply: a now becomes section 02, b becomes 01.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-b.md")); err != nil {
		t.Fatalf("after confirm, 02-b should be renumbered to 01-b: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "02-a.md")); err != nil {
		t.Fatalf("after confirm, 01-a should be renumbered to 02-a: %v", err)
	}
	if m.screen != screenWriting {
		t.Fatalf("apply should complete the pending exit, got screen %v", m.screen)
	}
}

func TestOutlineReorderDiscard(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // discard
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-a.md")); err != nil {
		t.Fatalf("discard must leave the disk untouched: %v", err)
	}
	if m.screen != screenWriting {
		t.Fatalf("discard should complete the exit, got %v", m.screen)
	}
}

func TestOutlineReorderTracksOpenFile(t *testing.T) {
	m, proj := setupManuscript(t)
	m.currentFile = filepath.Join(proj, "01-a.md") // 01-a is open in the editor
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}}) // a moves to slot 2
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if m.currentFile != filepath.Join(proj, "02-a.md") {
		t.Fatalf("the open file path should follow the rename to 02-a.md, got %q", m.currentFile)
	}
}

func TestOutlineNewSectionInsertsAfterSelection(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a, 02-b ; select 01-a
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// n -> prompt; type a title; enter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = nm.(model)
	for _, r := range "scene two" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// New section is slot 2; old 02-b shifts to 03-b.
	if _, err := os.Stat(filepath.Join(proj, "02-scene-two.md")); err != nil {
		t.Fatalf("expected new 02-scene-two.md after the selection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "03-b.md")); err != nil {
		t.Fatalf("expected 02-b renumbered to 03-b: %v", err)
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

func TestOutlineGateEscKeepsEditing(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}}) // reorder -> dirty
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // raise the gate
	m = nm.(model)
	if !m.outline.confirm {
		t.Fatal("esc with a pending reorder should raise the confirm gate")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // esc inside the gate
	m = nm.(model)
	if m.outline.confirm {
		t.Fatal("esc inside the gate should dismiss it")
	}
	if m.screen != screenOutline {
		t.Fatalf("esc inside the gate should keep editing the outline, got screen %v", m.screen)
	}
}

func TestOutlineReorderApplyOpensSelectedSection(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a, 02-b
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// Move 01-a down: working becomes [02-b, 01-a]; selection follows the moved 'a' to row 1.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	// Enter raises the gate; y applies AND must open the selected section at its new name.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.outline.confirm {
		t.Fatal("enter with a pending reorder should raise the confirm gate")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("apply should return to the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "02-a.md") {
		t.Fatalf("apply+open should open the selected section at its renamed path 02-a.md, got %q", m.currentFile)
	}
}

func TestOutlineReorderDiscardOpensSelectedSection(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a, 02-b
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}}) // selection follows 'a' to row 1
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // gate
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // discard + open
	m = nm.(model)
	if m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("discard+open should open the selected section 'a' at its unchanged name 01-a.md, got %q", m.currentFile)
	}
}
