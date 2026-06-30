//go:build !(darwin && cgo && applegrammar)

package main

import "testing"

// In the default (untagged, pure-Go) build there is no on-device backend.
func TestAppleGrammarCheckerDefaultNil(t *testing.T) {
	if c := appleGrammarChecker(); c != nil {
		t.Fatalf("default build must have no grammar checker, got %#v", c)
	}
	if newGrammarChecker == nil {
		t.Fatal("newGrammarChecker should be wired")
	}
}

func TestUTF16ToRune(t *testing.T) {
	// "café 🎉 x": é is 1 UTF-16 unit, 🎉 is a surrogate pair (2 units).
	s := "café 🎉 x"
	// rune layout: c a f é (space) 🎉 (space) x  → runes 0..7
	if got := utf16ToRune(s, 4); got != 4 { // through "café"
		t.Errorf("utf16ToRune(4)=%d want 4", got)
	}
	// after the emoji (units: café=4, space=1, 🎉=2 → unit 7) → rune index 6
	if got := utf16ToRune(s, 7); got != 6 {
		t.Errorf("utf16ToRune(7)=%d want 6 (surrogate pair counts as 2 units)", got)
	}
}

func TestRuneOffsetToLine(t *testing.T) {
	lines := []string{"hello", "wörld", "third"}
	if li, c := runeOffsetToLine(lines, 0); li != 0 || c != 0 {
		t.Errorf("offset 0 -> (%d,%d) want (0,0)", li, c)
	}
	// "hello"=5 runes, +1 newline = 6; rune 6 is start of line 1
	if li, c := runeOffsetToLine(lines, 6); li != 1 || c != 0 {
		t.Errorf("offset 6 -> (%d,%d) want (1,0)", li, c)
	}
	// rune 8 → line 1 col 2 ("wö|rld")
	if li, c := runeOffsetToLine(lines, 8); li != 1 || c != 2 {
		t.Errorf("offset 8 -> (%d,%d) want (1,2)", li, c)
	}
}
