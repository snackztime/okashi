package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchText(t *testing.T) {
	text := "The Lighthouse blinked.\nNo lighthouse here, and a LIGHTHOUSE there.\nnothing"
	hits := searchText("a.md", "/p/a.md", text, "lighthouse", 100)
	if len(hits) != 3 {
		t.Fatalf("want 3 case-insensitive hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].line != 0 || hits[0].col != 4 {
		t.Fatalf("first hit line/col = %d/%d, want 0/4", hits[0].line, hits[0].col)
	}
	// two on line 1
	if hits[1].line != 1 || hits[2].line != 1 {
		t.Fatalf("line-1 hits: %+v", hits[1:])
	}
	// limit
	if got := searchText("a.md", "/p/a.md", text, "lighthouse", 2); len(got) != 2 {
		t.Fatalf("limit not honored: %d", len(got))
	}
}

func TestSearchTextRuneColumn(t *testing.T) {
	// multibyte before the match → RUNE column, not byte.
	hits := searchText("a.md", "/p/a.md", "café — lighthouse", "lighthouse", 10)
	if len(hits) != 1 || hits[0].col != 7 { // c a f é space — space = 7 runes
		t.Fatalf("rune column wrong: %+v", hits)
	}
}

func TestSearchProject(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "novel"), 0o755)
	os.WriteFile(filepath.Join(root, "novel", "01.md"), []byte("the lighthouse\n"), 0o644)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("buy a lighthouse lamp\n"), 0o644)
	os.WriteFile(filepath.Join(root, "skip.png"), []byte("lighthouse"), 0o644) // non-doc
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "x.md"), []byte("lighthouse"), 0o644) // hidden dir
	hits := searchProject(root, allowedDocExts, "lighthouse", 100)
	if len(hits) != 2 {
		t.Fatalf("want 2 hits (md + txt, not png/hidden), got %d: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.name == "" || filepath.IsAbs(h.name) {
			t.Fatalf("name should be root-relative: %q", h.name)
		}
	}
}

func TestSearchScreenFlow(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "novel"), 0o755)
	os.WriteFile(filepath.Join(dir, "novel", "01.md"), []byte("The lighthouse blinked.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("a lighthouse lamp\n"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "notes.md"))

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = nm.(model)
	if m.screen != screenSearch {
		t.Fatal("ctrl+f should open the search screen")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("lighthouse")})
	m = nm.(model)
	if len(m.searchHits) != 2 {
		t.Fatalf("project search should find 2, got %d", len(m.searchHits))
	}
	// Tab → document scope (current file = notes.md → 1).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	if m.searchScope != scopeDocument || len(m.searchHits) != 1 {
		t.Fatalf("document scope: scope=%d hits=%d", m.searchScope, len(m.searchHits))
	}
	// Back to project, select the novel hit, jump.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	for i, h := range m.searchHits {
		if filepath.Base(h.file) == "01.md" {
			m.searchSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting || filepath.Base(m.currentFile) != "01.md" {
		t.Fatalf("jump: screen=%d file=%s", m.screen, filepath.Base(m.currentFile))
	}
	if m.searchHighlight != "lighthouse" {
		t.Fatalf("jump should set the highlight, got %q", m.searchHighlight)
	}
	// Editing clears the transient highlight.
	m.focus = focusEditor
	m.editor.Focus()
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = nm.(model)
	if m.searchHighlight != "" {
		t.Fatal("editing should clear the search highlight")
	}
}

func TestSearchExpandingFold(t *testing.T) {
	// Ⱥ (U+023A, 2 bytes) folds to ⱥ (U+2C65, 3 bytes): byte offsets from the fold must
	// NOT index the original — previously panicked.
	hits := searchText("a.md", "/p/a.md", "ȺȺx here", "x", 10)
	if len(hits) != 1 || hits[0].col != 2 { // rune index of 'x' after two Ⱥ
		t.Fatalf("expanding-fold column wrong: %+v", hits)
	}
	// the decorator + highlighter must not panic on the same input
	_ = searchDecorator("ȺȺx", "x")
	_ = highlightQuery("ȺȺx and X again", "x", 40)
}
