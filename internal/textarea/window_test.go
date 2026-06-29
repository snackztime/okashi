package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func bigEditor(lines int) Model {
	m := New()
	m.Prompt = ""
	m.ShowLineNumbers = false
	m.CharLimit = 0
	m.MaxHeight = 0
	m.FocusedStyle.Base = lipgloss.NewStyle()
	m.SetWidth(40)
	m.SetHeight(10)
	m.Focus()
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("w", 3))
		b.WriteByte('\n')
	}
	m.SetValue(b.String())
	return m
}

// The view is exactly `height` rows tall regardless of buffer size or scroll
// spot — matching the old viewport.View() contract (lipgloss.Height == height,
// no trailing blank row) that downstream layouts depend on.
func TestViewEmitsHeightRows(t *testing.T) {
	m := bigEditor(500)
	for _, row := range []int{0, 5, 250, 499} {
		m.row, m.col = row, 0
		m.repositionView()
		v := m.View()
		if got := lipgloss.Height(v); got != m.height {
			t.Fatalf("cursor row %d: view is %d lines tall, want height %d", row, got, m.height)
		}
		if strings.HasSuffix(v, "\n") {
			t.Fatalf("cursor row %d: view has a trailing newline (extra blank row)", row)
		}
	}
}

// The cursor's line content is present in the rendered window wherever the
// cursor is — i.e. the window actually follows the cursor.
func TestCursorLineVisible(t *testing.T) {
	m := bigEditor(500)
	m.SetValue(strings.Repeat("filler\n", 250) + "UNIQUEMARKER here\n" + strings.Repeat("filler\n", 250))
	m.row = 250 // the UNIQUEMARKER line
	m.col = 0
	m.repositionView()
	if !strings.Contains(m.View(), "UNIQUEMARKER") {
		t.Fatal("cursor line not visible in the windowed view")
	}
}

// A far-away line must NOT be rendered (proves we don't style the whole buffer).
func TestOffscreenLineNotRendered(t *testing.T) {
	m := bigEditor(0)
	m.SetValue("TOPMARKER\n" + strings.Repeat("filler\n", 500) + "BOTTOMMARKER\n")
	m.moveToEnd() // cursor at the bottom
	m.repositionView()
	v := m.View()
	if strings.Contains(v, "TOPMARKER") {
		t.Fatal("top line rendered while scrolled to the bottom — not windowed")
	}
	if !strings.Contains(v, "BOTTOMMARKER") {
		t.Fatal("bottom (cursor) line not visible")
	}
}
