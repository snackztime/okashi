package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCurrentSentenceSpan(t *testing.T) {
	cases := []struct {
		name               string
		text               string
		cursor             int
		wantStart, wantEnd int
	}{
		{"first sentence", "Hello world. Goodbye now.", 2, 0, 12},
		{"second sentence", "Hello world. Goodbye now.", 16, 13, 25},
		{"on terminator", "Hello world. Goodbye now.", 11, 0, 12},
		{"paragraph boundary", "One.\n\nTwo here.", 8, 6, 15}, // wantEnd corrected: 14→15 (terminator-inclusive per spec; see task-1-report.md)
		{"spans single newline", "A long\nsentence done.", 2, 0, 21},
		{"empty", "", 0, 0, 0},
		{"no terminator", "just some words", 5, 0, 15},
	}
	for _, c := range cases {
		gs, ge := currentSentenceSpan(c.text, c.cursor)
		if gs != c.wantStart || ge != c.wantEnd {
			t.Errorf("%s: currentSentenceSpan(%q,%d) = (%d,%d), want (%d,%d)",
				c.name, c.text, c.cursor, gs, ge, c.wantStart, c.wantEnd)
		}
	}
}

func TestSplitDimRuns(t *testing.T) {
	// seg "AB CD" starting at absolute offset 10; span [12,14) covers " C".
	runs := splitDimRuns([]rune("AB CD"), 10, 12, 14)
	// offsets: A=10 B=11 (dim) space=12 C=13 (bright) D=14 (dim)
	want := []dimRun{{"AB", true}, {" C", false}, {"D", true}}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs, want %d: %+v", len(runs), len(want), runs)
	}
	for i := range want {
		if runs[i] != want[i] {
			t.Fatalf("run %d = %+v, want %+v", i, runs[i], want[i])
		}
	}
}

func TestDimAppliesOutOfSpan(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // force ANSI256 so styles emit codes in tests
	defer lipgloss.SetColorProfile(old)

	ta := New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(60)
	ta.SetHeight(5)
	ta.DimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ta.SetValue("First one. Second two.")
	ta.SetCursor(2) // inside "First one."

	dimSeq := ta.DimStyle.Render("x")
	dimCode := dimSeq[:strings.Index(dimSeq, "x")] // the opening SGR of DimStyle

	ta.Dim = false
	if strings.Contains(ta.View(), dimCode) {
		t.Fatal("no dim styling expected when Dim is off")
	}
	ta.Dim = true
	if !strings.Contains(ta.View(), dimCode) {
		t.Fatal("expected the out-of-span text to carry the dim style when Dim is on")
	}
}

// TestPieceStartTracksSource guards the dim render's offset invariant: summing
// wrapped-piece lengths (pieceStart) must index the source line correctly, so
// dimming boundaries stay aligned on soft-wrapped lines.
func TestPieceStartTracksSource(t *testing.T) {
	srcs := []string{
		"The quick brown fox jumps over the lazy dog and then keeps running onward",
		"alpha beta gamma delta epsilon zeta eta theta iota kappa lambda",
		"aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk",
	}
	for _, s := range srcs {
		for _, w := range []int{10, 16, 24} {
			src := []rune(s)
			pieceStart := 0
			for k, p := range wrap(src, w) {
				for j := 0; j < len(p); j++ {
					if idx := pieceStart + j; idx < len(src) && p[j] != src[idx] {
						t.Fatalf("offset drift w=%d piece %d char %d: piece=%q src[%d]=%q (src=%q)",
							w, k, j, string(p[j]), idx, string(src[idx]), s)
					}
				}
				pieceStart += len(p)
			}
		}
	}
}
