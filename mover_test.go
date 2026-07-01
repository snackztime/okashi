package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// moverModelAt builds a model whose file pane is at root with `sel` selected.
func moverModelAt(t *testing.T, root string, selName string) model {
	t.Helper()
	t.Setenv("OKASHI_DIR", root)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.files.root = root
	m.files.SetDir(root)
	m.files.selectName(selName)
	return m
}

func TestMoverEnterFromFilePane(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	if m.screen != screenMover {
		t.Fatalf("M should enter the mover, screen=%v", m.screen)
	}
	if m.moverSource != filepath.Join(root, "stray.md") || m.moverIsDir {
		t.Fatalf("source should be the selected file, got %q isDir=%v", m.moverSource, m.moverIsDir)
	}
	// The destination browser lists a "move into" row + the subfolder(s).
	out := ansi.Strip(m.moverView())
	if !strings.Contains(out, "move into") || !strings.Contains(out, "research") {
		t.Fatalf("mover view should show the destination browser (move-into + subfolders):\n%s", out)
	}
}

func TestMoverDrillIntoSubfolderAndBack(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research", "deep"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	// entries: [move-here, ▸ research]  (no ".." at the source root)
	// select "research" and drill in.
	researchIdx := -1
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "research" {
			researchIdx = i
		}
	}
	if researchIdx < 0 {
		t.Fatalf("research folder should be listed: %+v", m.moverEntries)
	}
	m.moverSel = researchIdx
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into research
	m = nm.(model)
	if m.moverDestDir != filepath.Join(root, "research") {
		t.Fatalf("drilling should move destDir into research, got %q", m.moverDestDir)
	}
	// Now there is a ".." row and a "deep" subfolder.
	hasUp, hasDeep := false, false
	for _, e := range m.moverEntries {
		if e.kind == moverUp {
			hasUp = true
		}
		if e.kind == moverFolder && e.name == "deep" {
			hasDeep = true
		}
	}
	if !hasUp || !hasDeep {
		t.Fatalf("drilled browser should show '..' + 'deep', got %+v", m.moverEntries)
	}
	// Select ".." and go back to root; ".." must not escape the source root.
	for i, e := range m.moverEntries {
		if e.kind == moverUp {
			m.moverSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverDestDir != root {
		t.Fatalf("'..' should return to root, got %q", m.moverDestDir)
	}
}
