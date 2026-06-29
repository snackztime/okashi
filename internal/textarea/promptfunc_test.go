package textarea

import "testing"

// Regression for the windowed View(): the absolute display-row index passed to a
// prompt func must not be shifted by the blank pad rows typewriter mode prepends
// above row 0. Before the fix, displayLine started at 0 and the blank-row loop
// advanced it, so the first content row received height/2 instead of 0.
func TestPromptFuncIndexNotShiftedByTypewriterPad(t *testing.T) {
	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.Typewriter = true
	m.SetWidth(40)
	m.SetHeight(10) // cursor at the top → ~5 blank pad rows above
	var idxs []int
	m.SetPromptFunc(2, func(lineIdx int) string {
		idxs = append(idxs, lineIdx)
		return "> "
	})
	m.SetValue("alpha\nbeta\ngamma\n")
	m.row, m.col = 0, 0 // put the caret on the first line so typewriter pads above
	m.Focus()
	_ = m.View()

	if len(idxs) == 0 {
		t.Fatal("prompt func was never called")
	}
	if idxs[0] != 0 {
		t.Fatalf("first prompt index = %d, want 0 — blank typewriter pad rows must not shift the prompt index", idxs[0])
	}
}
