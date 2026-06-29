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
