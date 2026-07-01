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

func TestStructureReorderRemoveRetitle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	// add two more chapters to the manifest on disk so we have [One, Two, Three]
	writeManifest(dir, manifest{SchemaVersion: 1, Title: "Novel", Items: []manifestItem{
		{File: "01-one.md", Title: "One"}, {File: "02-two.md", Title: "Two"}, {File: "03-three.md", Title: "Three"},
	}})
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	// Move chapter 1 (One) down with J → [Two, One, Three]
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	if m.structureItems[0].Title != "Two" || m.structureItems[1].Title != "One" {
		t.Fatalf("J should move the selected chapter down, got %v", titles(m.structureItems))
	}
	if !m.structureDirty {
		t.Fatal("reorder should mark dirty")
	}
	// The cursor follows the moved item (now at index 1).
	if m.structureSel != 1 {
		t.Fatalf("cursor should follow the moved item, sel=%d", m.structureSel)
	}

	// Remove the selected chapter (One) with x → [Two, Three]
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = nm.(model)
	if len(m.structureItems) != 2 || titlesJoined(m.structureItems) != "Two|Three" {
		t.Fatalf("x should remove the selected chapter, got %v", titles(m.structureItems))
	}

	// Retitle the selected chapter with r + type + enter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.structureRenaming {
		t.Fatal("r should open the retitle field")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "Second")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.structureItems[m.structureSel].Title != "Second" {
		t.Fatalf("retitle should update the buffer title, got %q", m.structureItems[m.structureSel].Title)
	}
}

func titles(items []manifestItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}
func titlesJoined(items []manifestItem) string { return strings.Join(titles(items), "|") }
