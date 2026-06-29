package main

import (
	"testing"

	"okashi/internal/textarea"
)

func hasPosSpan(d []textarea.Decoration, s, e int) bool {
	for _, x := range d {
		if x.Start == s && x.End == e {
			return true
		}
	}
	return false
}

func TestPosDecorator(t *testing.T) {
	if d := posDecorator("She quickly ran", true, false, false); !hasPosSpan(d, 4, 11) {
		t.Fatalf("adverb 'quickly' [4,11) missing: %+v", d)
	}
	if d := posDecorator("the red car", false, true, false); !hasPosSpan(d, 4, 7) {
		t.Fatalf("adjective 'red' [4,7) missing: %+v", d)
	}
	dp := posDecorator("it was written", false, false, true)
	if !hasPosSpan(dp, 3, 6) || !hasPosSpan(dp, 7, 14) {
		t.Fatalf("passive 'was'+'written' spans missing: %+v", dp)
	}
	if d := posDecorator("She quickly ran", false, false, false); len(d) != 0 {
		t.Fatalf("no categories → no spans: %+v", d)
	}
}

func TestPosMultibyteOffsets(t *testing.T) {
	// "x — slowly" → x(0) sp(1) —(2) sp(3) slowly(4..10); rune offsets, not bytes.
	if d := posDecorator("x — slowly", true, false, false); !hasPosSpan(d, 4, 10) {
		t.Fatalf("adverb after em-dash should be rune [4,10): %+v", d)
	}
}

func TestPosMemoized(t *testing.T) {
	posTokens("warm up the cache please")
	if len(posCache) == 0 {
		t.Fatal("posTokens should populate posCache")
	}
}
