package main

import (
	"testing"

	"okashi/internal/textarea"
)

// hasSpan returns true if any decoration has the given rune-range [start,end).
func hasSpan(decos []textarea.Decoration, start, end int) bool {
	for _, d := range decos {
		if d.Start == start && d.End == end {
			return true
		}
	}
	return false
}

func TestSyntaxDecorator(t *testing.T) {
	// Heading: text "Title" at runes 2..7.
	if d := syntaxDecorator("# Title"); !hasSpan(d, 2, 7) {
		t.Fatalf("heading text should be a span [2,7): %+v", d)
	}
	// Bold over the whole **bold** (runes 0..8), not italicized.
	bd := syntaxDecorator("**bold** x")
	if !hasSpan(bd, 0, 8) {
		t.Fatalf("bold span [0,8) missing: %+v", bd)
	}
	// Italic *it* → [0,4).
	if d := syntaxDecorator("*it* y"); !hasSpan(d, 0, 4) {
		t.Fatalf("italic span [0,4) missing: %+v", d)
	}
	// Inline code.
	if d := syntaxDecorator("a `c` b"); !hasSpan(d, 2, 5) {
		t.Fatalf("code span [2,5) missing: %+v", d)
	}
	// Plain prose → nothing.
	if d := syntaxDecorator("just some plain words"); len(d) != 0 {
		t.Fatalf("plain line should have no spans: %+v", d)
	}
}

func TestSyntaxMultibyteOffsets(t *testing.T) {
	// An em-dash (multibyte) before bold: rune offsets must not be byte offsets.
	// "x — **b**"  → runes: x(0) space(1) —(2) space(3) *(4)*(5) b(6) *(7)*(8)
	d := syntaxDecorator("x — **b**")
	if !hasSpan(d, 4, 9) {
		t.Fatalf("bold span after em-dash should be rune [4,9), got %+v", d)
	}
}

func TestSyntaxHeadingMarkersAndLinkURL(t *testing.T) {
	// Heading "# Title": markers "# " at runes [0,2), text "Title" at [2,7).
	h := syntaxDecorator("# Title")
	if !hasSpan(h, 0, 2) {
		t.Fatalf("heading markers should be a [0,2) span (subtle): %+v", h)
	}
	if !hasSpan(h, 2, 7) {
		t.Fatalf("heading text [2,7) span missing: %+v", h)
	}
	// Link "[a](b)": text "[a]" at [0,3), url "(b)" at [3,6).
	l := syntaxDecorator("[a](b)")
	if !hasSpan(l, 0, 3) || !hasSpan(l, 3, 6) {
		t.Fatalf("link should be two spans [0,3)+[3,6) (text cyan, url subtle): %+v", l)
	}
}
