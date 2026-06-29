package textarea

import (
	"strings"
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

// TestCursorSentenceSpanWindowSmallerThanBuffer puts the cursor on a middle line
// of a buffer with more lines than the initial 2*radius window covers, so the
// computation runs on a window that does NOT span the whole buffer. The oracle
// is the same whole-buffer currentSentenceSpan; equality proves the windowed
// result is exact even when the window is a strict subset of the buffer.
func TestCursorSentenceSpanWindowSmallerThanBuffer(t *testing.T) {
	m := New()
	m.SetWidth(72)
	m.CharLimit = 0 // don't truncate the multi-line fixture
	m.SetValue(strings.Repeat("Sentence one. Sentence two.\n", 20))
	m.row, m.col = 10, 4
	want0, want1 := currentSentenceSpan(m.Value(), m.cursorRuneOffset())
	got0, got1 := m.cursorSentenceSpan()
	if got0 != want0 || got1 != want1 {
		t.Fatalf("row %d col %d: got [%d,%d), want [%d,%d)", m.row, m.col, got0, got1, want0, want1)
	}
}

// TestCursorSentenceSpanWidensForLongSentence forces the widen-and-retry path:
// a single sentence spans far more source lines than the initial radius (each
// line has no terminator), so the boundary lies outside the first window and the
// loop must widen until it finds the true sentence end. Equality with the
// whole-buffer oracle proves the widening is correct.
func TestCursorSentenceSpanWidensForLongSentence(t *testing.T) {
	m := New()
	m.SetWidth(72)
	m.CharLimit = 0 // don't truncate the multi-line fixture
	// 30 terminator-free lines bracketed by sentence ends: the middle line's
	// sentence runs from the first "." to the last, well past radius=4.
	var b strings.Builder
	b.WriteString("Start.\n")
	for i := 0; i < 30; i++ {
		b.WriteString("a long clause continues across many source lines\n")
	}
	b.WriteString("End.\n")
	m.SetValue(b.String())
	m.row, m.col = 15, 3
	want0, want1 := currentSentenceSpan(m.Value(), m.cursorRuneOffset())
	got0, got1 := m.cursorSentenceSpan()
	if got0 != want0 || got1 != want1 {
		t.Fatalf("row %d col %d: got [%d,%d), want [%d,%d)", m.row, m.col, got0, got1, want0, want1)
	}
}

// TestCursorSentenceSpanWidensRightWithMultibyte is the regression test for the
// rune-index-vs-byte-length bug in the widen guard. currentSentenceSpan returns
// RUNE indices, so the right-edge check must compare s1 against the window's
// RUNE count, not its byte length. With multibyte chars (em-dashes/curly quotes,
// pervasive in okashi prose) byte length > rune count, so the buggy guard never
// fires and returns a right-truncated span. The fixture: a terminator near the
// top (so the LEFT boundary is found inside the window → needLeft false), then a
// long em-dash-joined terminator-free run extending well past the right window
// edge, forcing ONLY the right side to widen.
func TestCursorSentenceSpanWidensRightWithMultibyte(t *testing.T) {
	m := New()
	m.SetWidth(72)
	m.CharLimit = 0 // don't truncate the multi-line fixture
	var b strings.Builder
	b.WriteString("Done.\n")  // row 0: terminator => left boundary lands here
	for i := 0; i < 40; i++ { // 40 terminator-free lines, each with an em-dash
		b.WriteString("a clause — continues onward across the source lines\n")
	}
	b.WriteString("End.\n")
	m.SetValue(b.String())
	// Cursor on row 3: "Done." (left boundary) is inside the initial radius-4
	// window, but the sentence's right boundary ("End.") is ~38 lines below it.
	m.row, m.col = 3, 2
	want0, want1 := currentSentenceSpan(m.Value(), m.cursorRuneOffset())
	got0, got1 := m.cursorSentenceSpan()
	if got0 != want0 || got1 != want1 {
		t.Fatalf("row %d col %d: got [%d,%d), want [%d,%d)", m.row, m.col, got0, got1, want0, want1)
	}
}
