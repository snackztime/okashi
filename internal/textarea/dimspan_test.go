package textarea

import (
	"testing"
)

// cursorSentenceSpan must equal the whole-buffer computation it replaces, for
// the cursor placed in various positions of a multi-line buffer.
func TestCursorSentenceSpanMatchesFullValue(t *testing.T) {
	m := New()
	m.SetWidth(72)
	m.SetValue("First sentence here. Second one follows! And a third?\n\nNew paragraph starts. It has two sentences.\n")
	for _, pos := range []struct{ row, col int }{
		{0, 3}, {0, 25}, {0, 50}, {2, 5}, {2, 30},
	} {
		m.row, m.col = pos.row, pos.col
		want0, want1 := currentSentenceSpan(m.Value(), m.cursorRuneOffset())
		got0, got1 := m.cursorSentenceSpan()
		if got0 != want0 || got1 != want1 {
			t.Fatalf("row %d col %d: got [%d,%d), want [%d,%d)", pos.row, pos.col, got0, got1, want0, want1)
		}
	}
}
