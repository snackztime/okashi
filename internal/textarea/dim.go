package textarea

import (
	"strings"
)

// currentSentenceSpan returns the [start,end) rune range of the sentence under
// cursorOffset. A sentence runs from after the previous terminator (.!?) +
// whitespace (or paragraph start) to the next terminator (inclusive). A blank
// line ("\n\n") is a hard boundary on both sides.
func currentSentenceSpan(text string, cursorOffset int) (int, int) {
	r := []rune(text)
	n := len(r)
	if n == 0 {
		return 0, 0
	}
	if cursorOffset < 0 {
		cursorOffset = 0
	}
	if cursorOffset > n {
		cursorOffset = n
	}

	isTerm := func(c rune) bool { return c == '.' || c == '!' || c == '?' }
	isWS := func(c rune) bool { return c == ' ' || c == '\t' || c == '\n' }
	paraBreak := func(i int) bool {
		return r[i] == '\n' && ((i > 0 && r[i-1] == '\n') || (i+1 < n && r[i+1] == '\n'))
	}

	start := 0
	for i := cursorOffset - 1; i >= 1; i-- {
		if paraBreak(i) {
			start = i + 1
			break
		}
		if isTerm(r[i-1]) && isWS(r[i]) {
			j := i
			for j < n && isWS(r[j]) {
				j++
			}
			start = j
			break
		}
	}

	end := n
	for i := cursorOffset; i < n; i++ {
		if paraBreak(i) {
			end = i
			break
		}
		if isTerm(r[i]) {
			end = i + 1
			break
		}
	}
	if start > end {
		start = end
	}
	return start, end
}

// cursorSentenceSpan returns the same absolute [span0,span1) as
// currentSentenceSpan(m.Value(), m.cursorRuneOffset()) but scans only a bounded
// window of source lines around the cursor, avoiding an O(buffer) m.Value() join
// every frame. okashi:dim
func (m Model) cursorSentenceSpan() (int, int) {
	const radius = 4 // sentences never span more than a few source lines here
	lo := max(0, m.row-radius)
	hi := min(len(m.value), m.row+radius+1)

	// Absolute rune offset of the first rune of line `lo`.
	base := 0
	for i := 0; i < lo; i++ {
		base += len(m.value[i]) + 1
	}
	// Join the window exactly as Value() would (newline between lines).
	var b strings.Builder
	for i := lo; i < hi; i++ {
		if i > lo {
			b.WriteByte('\n')
		}
		b.WriteString(string(m.value[i]))
	}
	local := m.cursorRuneOffset() - base
	s0, s1 := currentSentenceSpan(b.String(), local)
	return s0 + base, s1 + base
}

// dimRun is a maximal run of characters that are all in- or out-of-span.
type dimRun struct {
	text string
	dim  bool
}

// splitDimRuns groups seg (whose first rune is at absolute offset absStart) into
// runs marked dim when outside [span0,span1).
func splitDimRuns(seg []rune, absStart, span0, span1 int) []dimRun {
	var runs []dimRun
	i := 0
	for i < len(seg) {
		off := absStart + i
		dim := off < span0 || off >= span1
		j := i + 1
		for j < len(seg) {
			o := absStart + j
			if (o < span0 || o >= span1) != dim {
				break
			}
			j++
		}
		runs = append(runs, dimRun{text: string(seg[i:j]), dim: dim})
		i = j
	}
	return runs
}
