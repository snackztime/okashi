package textarea

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
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
	// A sentence's terminator may be followed by closing quotes/brackets before the whitespace
	// (dialogue ends `…go."`), so those must count as part of the sentence end.
	isClose := func(c rune) bool {
		switch c {
		case '"', '\'', ')', ']', '}', '»', '”', '’':
			return true
		}
		return false
	}
	paraBreak := func(i int) bool {
		return r[i] == '\n' && ((i > 0 && r[i-1] == '\n') || (i+1 < n && r[i+1] == '\n'))
	}

	start := 0
	for i := cursorOffset - 1; i >= 1; i-- {
		if paraBreak(i) {
			start = i + 1
			break
		}
		if !isWS(r[i]) {
			continue
		}
		// r[i] is whitespace: a sentence ends here if the run just before it is a terminator,
		// optionally followed by closing quotes/brackets.
		k := i - 1
		for k >= 0 && isClose(r[k]) {
			k--
		}
		if k >= 0 && isTerm(r[k]) {
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
			for end < n && isClose(r[end]) { // include trailing closing quotes/brackets
				end++
			}
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
// every frame. The window starts small (radius 4 covers normal prose in one
// pass) and widens only when a found boundary sits at the window edge while the
// buffer extends past it — so the result always equals the whole-buffer
// computation, yet stays O(sentence). okashi:dim
func (m Model) cursorSentenceSpan() (int, int) {
	full := len(m.value)
	for radius := 4; ; radius *= 2 {
		lo := max(0, m.row-radius)
		hi := min(full, m.row+radius+1)

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
		joined := b.String()
		s0, s1 := currentSentenceSpan(joined, m.cursorRuneOffset()-base)
		// If a boundary sits at the window edge while the buffer extends past it,
		// the true sentence may continue beyond the window — widen and retry.
		// currentSentenceSpan returns RUNE indices, so compare s1 against the
		// window's rune count, not len(joined) (bytes) — multibyte chars (curly
		// quotes, em-dashes) would otherwise make this check never fire.
		needLeft := s0 == 0 && lo > 0
		needRight := s1 == utf8.RuneCountInString(joined) && hi < full
		if (!needLeft && !needRight) || (lo == 0 && hi == full) {
			return s0 + base, s1 + base
		}
	}
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

// styledRun is a maximal run of characters that all render with the same style.
type styledRun struct {
	text  string
	style lipgloss.Style
}

// splitStyledRuns splits seg (first rune at absolute offset absStart) into runs,
// choosing each rune's style by precedence: a covering decoration > dim > normal.
// decos are ABSOLUTE-offset ranges (the View converts line-relative → absolute).
func splitStyledRuns(seg []rune, absStart, span0, span1 int, dimOn bool, normal, dimStyle lipgloss.Style, decos []Decoration) []styledRun {
	// styleAt returns the style AND a stable grouping key. Runs must be grouped by
	// this key, NOT by lipgloss.Style.String(): String() renders the EMPTY string,
	// which is "" for every style, so comparing it merges the whole segment into one
	// run — decorations vanish unless they begin the segment. Key: decoration index
	// ≥0, -1 dim, -2 normal.
	styleAt := func(off int) (lipgloss.Style, int) {
		for i, d := range decos {
			if off >= d.Start && off < d.End {
				return d.Style, i
			}
		}
		if dimOn && (off < span0 || off >= span1) {
			return dimStyle, -1
		}
		return normal, -2
	}
	var runs []styledRun
	i := 0
	for i < len(seg) {
		st, key := styleAt(absStart + i)
		j := i + 1
		for j < len(seg) {
			if _, k := styleAt(absStart + j); k != key {
				break
			}
			j++
		}
		runs = append(runs, styledRun{text: string(seg[i:j]), style: st})
		i = j
	}
	return runs
}
