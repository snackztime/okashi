package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeInto sends each rune of s to the model as a key message.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	return m
}

func sidebarModel(t *testing.T, dir string) model {
	t.Helper()
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(dir)
	m.focus = focusSidebar
	return m
}

func TestSidebarRenameLooseFileKeepsExt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("hi"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "notes")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "notes.md")); err != nil {
		t.Fatalf("expected renamed notes.md (ext kept): %v", err)
	}
}

func TestRenameRefusedForManifestChapter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":1,"title":"N","items":[{"file":"the-letter.md","title":"The Letter"}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("the-letter.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if m.renaming {
		t.Fatal("r on a manifest chapter must NOT start a rename (title is manifest-owned)")
	}
	if _, err := os.Stat(filepath.Join(proj, "the-letter.md")); err != nil {
		t.Fatalf("the chapter file must be untouched: %v", err)
	}
}

func TestRenameAllowedForResourceInManuscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, "notes.md"), []byte("y"), 0o644) // unlisted = Resource
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":1,"title":"N","items":[{"file":"a.md","title":"One"}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("notes.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a Resource (unlisted file) should start a plain rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "scratch")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "scratch.md")); err != nil {
		t.Fatalf("Resource rename should work like a loose-file rename: %v", err)
	}
}

func TestRenameAllowedForLegacySection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "legacy")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-opening.md"), []byte("x"), 0o644) // numbered, no manifest
	m := sidebarModel(t, proj)
	m.files.selectName("01-opening.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a legacy numbered section should start a retitle (O1: legacy ergonomics kept)")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the dawn")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-the-dawn.md")); err != nil {
		t.Fatalf("legacy retitle must preserve the numeric prefix: %v", err)
	}
}

func TestSidebarRenameFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "oldname"), 0o755)
	m := sidebarModel(t, root)
	m.files.selectName("oldname")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "newname")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "newname")); err != nil {
		t.Fatalf("folder rename should rename the directory: %v", err)
	}
}

func TestSidebarRenameRefusesCollision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "b.md"), []byte("y"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("a.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "b")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// Both originals must still exist — no overwrite.
	if b, _ := os.ReadFile(filepath.Join(root, "b.md")); string(b) != "y" {
		t.Fatal("rename onto an existing name must not overwrite it")
	}
	if _, err := os.Stat(filepath.Join(root, "a.md")); err != nil {
		t.Fatal("the source must be left intact when the rename is refused")
	}
}

func TestSidebarRenameTracksOpenFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("x"), 0o644)
	m := sidebarModel(t, root)
	m.currentFile = filepath.Join(root, "draft.md")
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "final.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.currentFile != filepath.Join(root, "final.md") {
		t.Fatalf("open file path should follow the rename, got %q", m.currentFile)
	}
}

func TestCtrlLOnNonManuscriptStaysPut(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	plain := filepath.Join(root, "plain")
	os.MkdirAll(plain, 0o755)
	os.WriteFile(filepath.Join(plain, "a.md"), []byte("x"), 0o644) // unnumbered, no manifest
	m := sidebarModel(t, plain)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen == screenOutline {
		t.Fatal("ctrl+l on a non-manuscript folder must not enter the outline")
	}
}
