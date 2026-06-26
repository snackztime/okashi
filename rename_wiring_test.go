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

func TestSidebarRenameSectionTitleOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "02-the-letter.md"), []byte("x"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("02-the-letter.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the telegram")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "02-the-telegram.md")); err != nil {
		t.Fatalf("section rename should keep the 02- prefix: %v", err)
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

func TestOutlineRenameSectionTitle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("x"), 0o644)
	m := sidebarModel(t, proj)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // enter the outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select 02-the-letter
	m = nm.(model)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r in the outline should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the telegram")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "02-the-telegram.md")); err != nil {
		t.Fatalf("outline rename should retitle keeping the prefix: %v", err)
	}
	if m.screen != screenOutline {
		t.Fatalf("after an outline rename we should still be in the outline, got %v", m.screen)
	}
}

func TestConvertPromptOnPlainFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	book := filepath.Join(root, "book")
	os.MkdirAll(book, 0o755)
	os.WriteFile(filepath.Join(book, "Chapter-00.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(book, "Chapter-01.md"), []byte("y"), 0o644)
	m := sidebarModel(t, book)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if !m.convertPrompt {
		t.Fatal("ctrl+l on a plain folder with files should raise the convert prompt")
	}
	if m.screen == screenOutline {
		t.Fatal("must not enter the outline before the user confirms")
	}
}

func TestConvertNumbersFilesAndOpensOutline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	book := filepath.Join(root, "book")
	os.MkdirAll(book, 0o755)
	os.WriteFile(filepath.Join(book, "Chapter-00.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(book, "Chapter-01.md"), []byte("y"), 0o644)
	m := sidebarModel(t, book)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(book, "01-Chapter-00.md")); err != nil {
		t.Fatalf("convert should number the first file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(book, "02-Chapter-01.md")); err != nil {
		t.Fatalf("convert should number the second file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(book, ".backup")); err != nil {
		t.Fatalf("convert should snapshot to .backup/ first: %v", err)
	}
	if m.screen != screenOutline {
		t.Fatalf("convert should open the outline, got screen %v", m.screen)
	}
}

func TestCtrlLNoDocsShowsNothingToConvert(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0o755)
	m := sidebarModel(t, empty)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.convertPrompt || m.screen == screenOutline {
		t.Fatal("ctrl+l on a folder with no documents should neither prompt nor enter the outline")
	}
}

func TestConvertTracksOpenFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	book := filepath.Join(root, "book")
	os.MkdirAll(book, 0o755)
	os.WriteFile(filepath.Join(book, "Chapter-00.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(book, "Chapter-01.md"), []byte("y"), 0o644)
	m := sidebarModel(t, book)
	m.currentFile = filepath.Join(book, "Chapter-00.md") // editing the first chapter

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}) // confirm convert
	m = nm.(model)
	if m.currentFile != filepath.Join(book, "01-Chapter-00.md") {
		t.Fatalf("convert should follow the open file to 01-Chapter-00.md, got %q", m.currentFile)
	}
}
