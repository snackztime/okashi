package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestWordCount(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  spaced   out  words ", 3},
		{"line one\nline two\n\nthree", 5},
	}
	for _, c := range cases {
		if got := wordCount(c.in); got != c.want {
			t.Errorf("wordCount(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestCommafy(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1,000"},
		{1240, "1,240"},
		{1000000, "1,000,000"},
	}
	for _, c := range cases {
		if got := commafy(c.in); got != c.want {
			t.Errorf("commafy(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSignedComma(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "+0"},
		{142, "+142"},
		{1500, "+1,500"},
		{-7, "-7"},
		{-1500, "-1,500"},
	}
	for _, c := range cases {
		if got := signedComma(c.in); got != c.want {
			t.Errorf("signedComma(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStatsText(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("one two three four five")
	m.sessionBaseline = 2 // as if a 2-word file was open when the session "started"

	want := "5 words · +3 session"
	if got := m.statsText(); got != want {
		t.Errorf("statsText() = %q, want %q", got, want)
	}
}

func TestSessionBaselineResetsOnOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ch.md")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := initialModel()
	m.loadFile(path)

	// Opening a 3-word file means 3 words present, but 0 written this session.
	if m.sessionBaseline != 3 {
		t.Fatalf("baseline after open = %d, want 3", m.sessionBaseline)
	}
	if got := m.statsText(); got != "3 words · +0 session" {
		t.Fatalf("stats after open = %q, want %q", got, "3 words · +0 session")
	}
}

func TestStatusBarRightAlignsStats(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = nm.(model)
	m.status = "ready"
	m.editor.SetValue("one two three")
	m.sessionBaseline = 0

	bar := m.statusBar()
	if !strings.HasPrefix(bar, "ready") {
		t.Fatalf("bar should start with the status message: %q", bar)
	}
	if !strings.HasSuffix(bar, m.statsText()) {
		t.Fatalf("bar should end with the stats readout: %q", bar)
	}
	if w := lipgloss.Width(bar); w != m.width-2 {
		t.Fatalf("bar width = %d, want %d (fills the bar minus padding)", w, m.width-2)
	}
}

func TestStatusBarHidesStatsWhenNarrow(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 12, Height: 24})
	m = nm.(model)
	m.status = "a fairly long status message"

	// No room for both — show the status message alone, no truncated stats.
	if got := m.statusBar(); got != m.status {
		t.Fatalf("narrow bar = %q, want just the status message", got)
	}
}

func TestSessionBaselineResetsOnNewFile(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("some pre-existing words")
	m.sessionBaseline = 3

	m.nameInput.SetValue("draft.md")
	m.confirmCreate()

	// A new file starts empty: 0 words, 0 written this session.
	if m.sessionBaseline != 0 {
		t.Fatalf("baseline after new file = %d, want 0", m.sessionBaseline)
	}
	if got := m.statsText(); got != "0 words · +0 session" {
		t.Fatalf("stats after new file = %q, want %q", got, "0 words · +0 session")
	}
}
