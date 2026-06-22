package textarea

import "testing"

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
