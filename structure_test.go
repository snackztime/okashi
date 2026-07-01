package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func enterStructureAt(t *testing.T, root, name, first string) model {
	t.Helper()
	dir := filepath.Join(root, name)
	createManuscript(dir, name, first)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()
	return m
}

func TestStructureEnterLoadsChaptersAndExits(t *testing.T) {
	root := t.TempDir()
	m := enterStructureAt(t, root, "novel", "Opening")
	if m.screen != screenStructure {
		t.Fatalf("s should enter structure mode, screen=%v", m.screen)
	}
	if len(m.structureItems) != 1 || m.structureItems[0].Title != "Opening" {
		t.Fatalf("structure should load the manifest items, got %+v", m.structureItems)
	}
	if !strings.Contains(ansiStrip(m.structureView()), "Opening") {
		t.Fatalf("structure view should show the chapter title")
	}
	// esc with no edits exits back to the binder (screenOutline).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("esc (clean) should return to the binder, screen=%v", m.screen)
	}
}

func TestStructureRefusesNonManifest(t *testing.T) {
	root := t.TempDir()
	// a plain folder with a numbered file = legacy manuscript (no manifest)
	dir := filepath.Join(root, "legacy")
	if err := writeFileP(filepath.Join(dir, "01-a.md"), "x"); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.outline.dir = dir
	m.enterStructure()
	if m.screen == screenStructure {
		t.Fatal("a legacy (manifest-less) manuscript must not enter structure mode")
	}
}

func ansiStrip(s string) string { return ansi.Strip(s) }

func writeFileP(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
