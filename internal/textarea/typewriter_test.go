package textarea

import (
	"fmt"
	"strings"
	"testing"
)

// build a 40-line, narrow-content textarea so each logical line is one row
// (cursorLineNumber == Line()).
func newTA() Model {
	ta := New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(40)
	ta.SetHeight(10) // viewport height 10, center row = 5
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	ta.SetValue(strings.Join(lines, "\n"))
	return ta
}

func TestTypewriterCentersCaret(t *testing.T) {
	ta := newTA()
	ta.Typewriter = true

	// Move caret to the top, then to a few known rows, and assert the offset
	// centers that row (offset == row, since lines don't wrap).
	for i := 0; i < 40; i++ {
		ta.CursorUp()
	}
	for _, want := range []int{0, 10, 20} {
		for ta.Line() < want {
			ta.CursorDown()
		}
		_ = ta.View() // centering is applied during render
		if got := ta.ViewportYOffset(); got != want {
			t.Fatalf("caret on line %d: YOffset = %d, want %d", ta.Line(), got, want)
		}
	}
}

func TestTypewriterOffDoesNotCenter(t *testing.T) {
	ta := newTA()
	ta.Typewriter = false
	for i := 0; i < 40; i++ {
		ta.CursorUp()
	}
	_ = ta.View()
	if got := ta.ViewportYOffset(); got != 0 {
		t.Fatalf("typewriter off, caret at top: YOffset = %d, want 0", got)
	}
}
