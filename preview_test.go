package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestFootnotesToEndnotes(t *testing.T) {
	out := footnotesToEndnotes("cat[^1] and[^2] cat again[^1].\n\n[^1]: first.\n[^2]: second.\n")
	if strings.Contains(out, "[^1]") || strings.Contains(out, "[^2]:") {
		t.Fatalf("syntax not stripped: %q", out)
	}
	if !strings.Contains(out, "### Notes") || !strings.Contains(out, "1. first") || !strings.Contains(out, "2. second") {
		t.Fatalf("notes section missing: %q", out)
	}
	if !strings.Contains(out, "¹") || !strings.Contains(out, "²") {
		t.Fatalf("superscript markers missing: %q", out)
	}
	if got := footnotesToEndnotes("plain text\n"); got != "plain text\n" {
		t.Fatalf("no-footnote input should be unchanged, got %q", got)
	}
	if !strings.Contains(footnotesToEndnotes("see[^x] here\n"), "[^x]") {
		t.Fatal("orphan reference (no definition) should be kept literal")
	}
}

func TestFootnotesSkipCode(t *testing.T) {
	out := footnotesToEndnotes("```\narr[^1] = x\n```\n\nreal[^1] body.\n\n[^1]: note.\n")
	if !strings.Contains(out, "arr[^1] = x") {
		t.Fatal("footnote-like text inside a code block must survive verbatim")
	}
	if strings.Contains(out, "real[^1]") || !strings.Contains(out, "### Notes") {
		t.Fatal("a real footnote reference outside code should still convert")
	}
	if !strings.Contains(footnotesToEndnotes("use `x[^1]` and real[^1].\n\n[^1]: n.\n"), "`x[^1]`") {
		t.Fatal("inline code span [^1] must survive")
	}
}

func TestPreviewTufteToggle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("# Title\n\nThe cat[^1] sat.\n\n[^1]: a note.\n"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if !m.previewing {
		t.Fatal("ctrl+p should enter preview")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Notes") || !strings.Contains(v, "a note") || strings.Contains(v, "[^1]") {
		t.Fatal("preview should fold footnotes to endnotes")
	}
	if !strings.Contains(v, "Default") {
		t.Fatal("preview header should show the Default style")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m = nm.(model)
	if !m.previewTufte || !strings.Contains(ansi.Strip(m.View()), "Tufte") {
		t.Fatal("t should toggle to the Tufte style")
	}
}
