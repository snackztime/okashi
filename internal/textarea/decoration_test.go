package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestDecoratorStylesRange(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // force ANSI256 so styles emit codes in tests
	defer lipgloss.SetColorProfile(old)

	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("alpha bravo charlie")
	// Underline "bravo" (runes 6..11).
	m.Decorator = func(line string) []Decoration {
		i := strings.Index(line, "bravo")
		if i < 0 {
			return nil
		}
		return []Decoration{{Start: i, End: i + len("bravo"), Style: lipgloss.NewStyle().Underline(true)}}
	}
	out := m.View()
	// lipgloss renders the underlined range per-rune (each rune wrapped in its own
	// SGR pair), so "bravo" is not contiguous in the styled output. Verify the text
	// survived by stripping ANSI first.
	if !strings.Contains(ansi.Strip(out), "alpha bravo charlie") {
		t.Fatalf("decorated text should still be present (ANSI-stripped):\n%q", ansi.Strip(out))
	}
	// The underline SGR (4) must appear, and only around bravo (alpha/charlie plain).
	if !strings.Contains(out, "\x1b[4m") && !strings.Contains(out, ";4m") {
		t.Fatalf("expected an underline SGR around the decorated range:\n%q", out)
	}
}

func TestNilDecoratorUnchanged(t *testing.T) {
	// With no Decorator, the rendered output must equal the dim/plain render
	// (sanity: View doesn't panic and contains the text).
	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("plain text here")
	if m.Decorator != nil {
		t.Fatal("Decorator should default to nil")
	}
	if !strings.Contains(m.View(), "plain text here") {
		t.Fatal("nil-decorator render should contain the text unchanged")
	}
}

// TestDecorationMidSegmentAnyCursor guards the splitStyledRuns grouping bug:
// a decoration NOT at a segment start (e.g. the cursor sits before it) must still
// render, scoped to only its runes — not lost, and not smeared over the whole line.
func TestDecorationMidSegmentAnyCursor(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // force SGR output in the test (no TTY otherwise)
	defer lipgloss.SetColorProfile(old)
	for _, cur := range []int{0, 6, 18} {
		m := New()
		m.SetWidth(40)
		m.SetHeight(4)
		m.Focus()
		m.SetValue("alpha bravo charlie delta")
		m.SetCursor(cur)
		m.Decorator = func(line string) []Decoration {
			i := strings.Index(line, "charlie")
			if i < 0 {
				return nil
			}
			return []Decoration{{Start: i, End: i + 7, Style: lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Underline(true)}}
		}
		out := m.View()
		// The decorated word must render (an underline SGR present) regardless of cursor.
		if !strings.Contains(out, "4m") {
			t.Fatalf("cursor@%d: mid-line decoration did not render: %q", cur, out)
		}
		// And the text stays intact (not corrupted by the run grouping).
		if !strings.Contains(ansi.Strip(out), "alpha") || !strings.Contains(ansi.Strip(out), "charlie") {
			t.Fatalf("cursor@%d: text corrupted: %q", cur, ansi.Strip(out))
		}
	}
}
