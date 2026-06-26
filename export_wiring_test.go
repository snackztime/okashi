package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExportSingleDocFromEditor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("She wrote **back**."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	if !m.exportPrompt {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// single-doc title = sectionTitle -> "the letter" -> slug "the-letter"
	rtf := filepath.Join(proj, "export", "the-letter.rtf")
	pdf := filepath.Join(proj, "export", "the-letter.pdf")
	if b, err := os.ReadFile(rtf); err != nil || !bytes.Contains(b, []byte(`\rtf1`)) {
		t.Fatalf("expected an RTF at %s: %v", rtf, err)
	}
	if b, err := os.ReadFile(pdf); err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("expected a PDF at %s: %v", pdf, err)
	}
}

func TestExportWholeManuscriptFromOutline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "my-novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("beta"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE}) // export chooser on the outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}) // tufte
	m = nm.(model)
	// whole-manuscript title = projectTitle("my-novel") = "my novel" -> slug "my-novel"
	if _, err := os.Stat(filepath.Join(proj, "export", "my-novel.pdf")); err != nil {
		t.Fatalf("expected the whole-manuscript PDF: %v", err)
	}
}

func TestExportCancel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "x.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(root)
	m.currentFile = filepath.Join(root, "x.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.exportPrompt {
		t.Fatal("esc should dismiss the export chooser")
	}
	if _, err := os.Stat(filepath.Join(root, "export")); !os.IsNotExist(err) {
		t.Fatal("cancel should write nothing")
	}
}
