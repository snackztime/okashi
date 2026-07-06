package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func paneManuscript(t *testing.T) (m model, dir string) {
	t.Helper()
	dir = t.TempDir()
	for _, f := range []string{"01-a.md", "02-b.md", "03-c.md"} {
		os.WriteFile(filepath.Join(dir, f), []byte("# H\n\nbody of "+f), 0o644)
	}
	writeManifest(dir, manifest{SchemaVersion: manifestSchemaVersion, Title: "W", Items: []manifestItem{
		{File: "01-a.md", Title: "One"}, {File: "02-b.md", Title: "Two"}, {File: "03-c.md", Title: "Three"},
	}})
	fl := newFilelist()
	fl.root = dir
	fl.width, fl.height = 40, 40
	fl.SetDir(dir)
	m = model{width: 100, height: 30, files: fl, screen: screenWriting, focus: focusSidebar}
	return m, dir
}

func TestTogglePaneCork(t *testing.T) {
	m, _ := paneManuscript(t)
	if m.files.corkMode {
		t.Fatal("starts in list mode")
	}
	m.togglePaneCork()
	if !m.files.corkMode {
		t.Fatal("ctrl+k should turn corkboard on for a manuscript")
	}
	// Non-manuscript: no toggle.
	m2 := model{files: newFilelist()}
	m2.togglePaneCork()
	if m2.files.corkMode {
		t.Fatal("corkboard should not toggle on a non-manuscript folder")
	}
}

func TestPaneReorderStagesAndCommits(t *testing.T) {
	m, dir := paneManuscript(t)
	// select chapter 1 (01-a.md); with a "..", entries = [.., 01,02,03]. Find it.
	m.files.selectByName("01-a.md")

	m.paneReorder(1) // move 01-a down → view order b, a, c
	if !m.paneReorderDirty {
		t.Fatal("reorder should mark dirty")
	}
	if m.files.view.chapters[0].file != "02-b.md" || m.files.view.chapters[1].file != "01-a.md" {
		t.Fatalf("staged order wrong: %v", m.files.view.chapters)
	}
	// Nothing written yet.
	if mani, _, _ := readManifest(dir); mani.Items[0].File != "01-a.md" {
		t.Fatal("staged reorder must not write the manifest before commit")
	}
	// esc → confirm → y commits.
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if !m.paneReorderConfirm {
		t.Fatal("esc should raise the reorder confirm")
	}
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)
	if mani, _, _ := readManifest(dir); mani.Items[0].File != "02-b.md" || mani.Items[1].File != "01-a.md" {
		t.Fatalf("commit should write the new order, got %v", mustItems(dir))
	}
	if m.paneReorderDirty || m.paneReorderConfirm {
		t.Fatal("flags should clear after commit")
	}
}

func TestPaneReorderDiscard(t *testing.T) {
	m, dir := paneManuscript(t)
	m.files.selectByName("01-a.md")
	m.paneReorder(1)
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // confirm
	m = mm.(model)
	mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // discard
	m = mm.(model)
	if mani, _, _ := readManifest(dir); mani.Items[0].File != "01-a.md" {
		t.Fatal("discard must leave the manifest unchanged")
	}
	if m.files.view.chapters[0].file != "01-a.md" {
		t.Fatal("discard must reload the original order into the view")
	}
}

func TestPaneSynopsisEditSaves(t *testing.T) {
	m, dir := paneManuscript(t)
	m.files.selectByName("02-b.md")
	m.startPaneSynopsis()
	if !m.paneSynEditing {
		t.Fatal("e should start the synopsis edit")
	}
	m.synArea.SetValue("The middle chapter.")
	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.paneSynEditing {
		t.Fatal("esc should commit + close")
	}
	if loadSynopses(dir)["02-b.md"] != "The middle chapter." {
		t.Fatalf("synopsis not saved: %v", loadSynopses(dir))
	}
}

func TestPaneReorderNoOpOnResource(t *testing.T) {
	m, _ := paneManuscript(t)
	os.WriteFile(filepath.Join(m.files.dir, "loose.md"), []byte("x"), 0o644)
	m.files.SetDir(m.files.dir)
	m.files.selectByName("loose.md")
	m.paneReorder(1)
	if m.paneReorderDirty {
		t.Fatal("reorder on a Resource must be a no-op")
	}
}

func mustItems(dir string) []manifestItem { m, _, _ := readManifest(dir); return m.Items }
