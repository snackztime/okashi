package main

import (
	"testing"

	"okashi/internal/textarea"
)

func hasGSpan(d []textarea.Decoration, s, e int) bool {
	for _, x := range d {
		if x.Start == s && x.End == e {
			return true
		}
	}
	return false
}

func TestGrammarDecorator(t *testing.T) {
	// Doubled word: "the the cat" → the 2nd "the" (runes 4..7).
	if d := grammarDecorator("the the cat", false); !hasGSpan(d, 4, 7) {
		t.Fatalf("doubled-word span [4,7) missing: %+v", d)
	}
	// a/an: "a apple" → the "a" (runes 0..1).
	if d := grammarDecorator("a apple", false); !hasGSpan(d, 0, 1) {
		t.Fatalf("a/an span [0,1) missing: %+v", d)
	}
	// Double space: "hello  world" → the extra space (runes 5..7 covers "  ").
	if d := grammarDecorator("hello  world", false); len(d) == 0 {
		t.Fatalf("double-space should flag: %+v", d)
	}
	// Space before punctuation: "word ," → the space (rune 4..5).
	if d := grammarDecorator("word , next", false); !hasGSpan(d, 4, 5) {
		t.Fatalf("space-before-punct span missing: %+v", d)
	}
	// Clean line → nothing.
	if d := grammarDecorator("This is a fine sentence.", false); len(d) != 0 {
		t.Fatalf("clean line should have no findings: %+v", d)
	}
}

func TestGrammarTerminalPunctuation(t *testing.T) {
	// A paragraph line lacking terminal punctuation → flag the last char (non-cursor line).
	if d := grammarDecorator("This has no period", false); len(d) == 0 {
		t.Fatal("missing terminal punctuation should flag")
	}
	// Suppressed on the cursor's own line (mid-typing).
	if d := grammarDecorator("This has no period", true); len(d) != 0 {
		t.Fatalf("terminal-punct must be suppressed on the cursor line: %+v", d)
	}
	// Headings and list items are exempt.
	if d := grammarDecorator("# A heading", false); len(d) != 0 {
		t.Fatalf("heading should not be flagged: %+v", d)
	}
	if d := grammarDecorator("- a list item", false); len(d) != 0 {
		t.Fatalf("list item should not be flagged: %+v", d)
	}
}

func TestGrammarDoubledWordMidSentence(t *testing.T) {
	// "He went to the the store": the doubled "the" is at an odd word position —
	// must still be flagged (regression: RE2 has no backreferences).
	d := grammarDecorator("He went to the the store", false)
	runes := []rune("He went to the the store")
	found := false
	for _, x := range d {
		if string(runes[x.Start:x.End]) == "the" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doubled 'the the' mid-sentence not flagged: %+v", d)
	}
	// No false positive when the words differ.
	for _, x := range grammarDecorator("the cat the dog", false) {
		if string([]rune("the cat the dog")[x.Start:x.End]) == "the" {
			t.Fatal("non-adjacent 'the' must not be flagged as doubled")
		}
	}
}
