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

func TestStructureAddNewBlank(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = nm.(model)
	if !m.structureAdding {
		t.Fatal("a should open the add pick")
	}
	// The first choice is "new blank chapter"; enter selects it.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if len(m.structureItems) != 2 {
		t.Fatalf("new blank should insert a chapter, got %d", len(m.structureItems))
	}
	if len(m.structurePendingNew) != 1 {
		t.Fatalf("a new-blank chapter should be pending-create, got %d", len(m.structurePendingNew))
	}
	if !m.structureDirty {
		t.Fatal("add should mark dirty")
	}
}

func TestStructureAddPromoteResource(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	// a loose Resource on disk, not in the manifest
	os.WriteFile(filepath.Join(dir, "extra-scene.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	choices := m.structureAddChoices()
	// choices[0] = new blank; a resource entry for extra-scene.md must exist
	foundRes := false
	resIdx := 0
	for i, c := range choices {
		if c.file == "extra-scene.md" {
			foundRes = true
			resIdx = i
		}
	}
	if !foundRes {
		t.Fatalf("the loose resource should be offered, choices=%+v", choices)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = nm.(model)
	m.structureAddSel = resIdx
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// extra-scene.md is now a chapter; it is NOT pending-new (it already exists).
	found := false
	for _, it := range m.structureItems {
		if it.File == "extra-scene.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("promoting a resource should add it to items, got %v", titles(m.structureItems))
	}
	if m.structurePendingNew["extra-scene.md"] {
		t.Fatal("a promoted existing file must NOT be pending-create")
	}
}
