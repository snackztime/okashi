package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func seedCorkManuscript(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	for _, f := range []string{"01-a.md", "02-b.md", "03-c.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("body of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         "The Work",
		Items: []manifestItem{
			{File: "01-a.md", Title: "One"}, {File: "02-b.md", Title: "Two"}, {File: "03-c.md", Title: "Three"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCorkboardEntryRequiresManifest(t *testing.T) {
	// No manifest → refused, stays on the binder.
	m := model{}
	m.outline.dir = t.TempDir()
	m.enterCorkboard()
	if m.screen == screenCorkboard {
		t.Fatal("a non-manifest dir must not enter the corkboard")
	}

	dir := seedCorkManuscript(t)
	m2 := model{}
	m2.outline.dir = dir
	m2.enterCorkboard()
	if m2.screen != screenCorkboard {
		t.Fatalf("a manifest manuscript should enter the corkboard, screen=%v", m2.screen)
	}
	if len(m2.structureItems) != 3 {
		t.Fatalf("staged buffer should hold 3 chapters, got %d", len(m2.structureItems))
	}
}

func TestCorkboardSynopsisEditWritesSidecar(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{}
	m.outline.dir = dir
	m.enterCorkboard()
	m.structureSel = 1 // chapter 02-b.md

	// e → edit; type; esc → commit + immediate sidecar write.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = mm.(model)
	if !m.synEditing {
		t.Fatal("e should open the synopsis editor")
	}
	m.synArea.SetValue("The train is late.")
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.synEditing {
		t.Fatal("esc should commit and close the editor")
	}
	// Persisted to the sidecar and not requiring a manifest commit.
	if loadSynopses(dir)["02-b.md"] != "The train is late." {
		t.Fatalf("synopsis not written to sidecar: %+v", loadSynopses(dir))
	}
	if m.structureDirty {
		t.Fatal("a synopsis edit must not mark the manifest dirty")
	}
}

func TestCorkboardReorderCommitsViaStructurePath(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{}
	m.outline.dir = dir
	m.enterCorkboard()
	m.structureSel = 0

	// J moves chapter 1 down → order becomes b, a, c.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = mm.(model)
	if !m.structureDirty {
		t.Fatal("reorder should mark the manifest dirty")
	}
	// esc → confirm, y → commit via the shared commitStructure path.
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if !m.structureConfirm {
		t.Fatal("esc with a dirty order should raise the commit confirm")
	}
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)

	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 3 || mani.Items[0].File != "02-b.md" || mani.Items[1].File != "01-a.md" {
		t.Fatalf("reorder not committed to the manifest: %+v", mani.Items)
	}
	if m.screen != screenOutline {
		t.Fatal("committing should return to the binder")
	}
}

func TestCorkboardViewWindows(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{width: 90, height: 12}
	m.outline.dir = dir
	m.enterCorkboard()
	out := m.corkboardView()
	if out == "" {
		t.Fatal("corkboard view should render")
	}
}
