package main

import "testing"

func TestSpellDecorator(t *testing.T) {
	decos := spellDecorator("teh quikc brown fox")
	// "teh" and "quikc" are misspelled; "brown"/"fox" are real words.
	if len(decos) != 2 {
		t.Fatalf("expected 2 misspellings (teh, quikc), got %d: %+v", len(decos), decos)
	}
	// First decoration covers "teh" (0..3).
	if decos[0].Start != 0 || decos[0].End != 3 {
		t.Fatalf("first misspelling span = [%d,%d), want [0,3)", decos[0].Start, decos[0].End)
	}
	// A correctly-spelled line yields nothing.
	if d := spellDecorator("the quick brown fox"); len(d) != 0 {
		t.Fatalf("correct line should have no decorations, got %+v", d)
	}
	// Short words and all-caps are skipped.
	if d := spellDecorator("OK is a NASA go"); len(d) != 0 {
		t.Fatalf("short/all-caps words should be skipped, got %+v", d)
	}
}
