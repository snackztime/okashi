package main

import "testing"

func TestSpellOK(t *testing.T) {
	// Morphology/contractions/possessives the old list flagged are now correct.
	for _, w := range []string{"jumps", "emailed", "reconnected", "don't", "it's", "Sarah's", "cafe's"} {
		if !spellOK(w) {
			t.Errorf("spellOK(%q) = false, want true", w)
		}
	}
	for _, w := range []string{"teh", "quikc", "brilig"} {
		if spellOK(w) {
			t.Errorf("spellOK(%q) = true, want false", w)
		}
	}
}

func TestSpellSuggest(t *testing.T) {
	got := spellSuggest("teh", 5)
	found := false
	for _, s := range got {
		if s == "the" {
			found = true
		}
	}
	if !found {
		t.Fatalf("spellSuggest(teh) should include \"the\", got %v", got)
	}
	if len(spellSuggest("the", 5)) >= 0 { // a correct word: suggester may return [] — just must not panic
	}
}

func TestSpellSuggestMemoized(t *testing.T) {
	a := spellSuggest("teh", 4)
	b := spellSuggest("teh", 4)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("memoized spellSuggest should return a stable non-empty list: %v vs %v", a, b)
	}
	if len(suggestCache) == 0 {
		t.Fatal("spellSuggest should populate suggestCache")
	}
}

func TestSpellDecoratorEngine(t *testing.T) {
	// In a normal sentence, only the typo is flagged (not jumps/don't).
	decos := spellDecorator("The fox jumps but teh dog don't care")
	if len(decos) != 1 {
		t.Fatalf("expected exactly 1 flag (teh), got %d: %+v", len(decos), decos)
	}
	runes := []rune("The fox jumps but teh dog don't care")
	if got := string(runes[decos[0].Start:decos[0].End]); got != "teh" {
		t.Fatalf("flagged %q, want \"teh\"", got)
	}
	// All-caps acronym + digit token skipped.
	if d := spellDecorator("NASA sent 3 rockets"); len(d) != 0 {
		t.Fatalf("all-caps/digit tokens should be skipped, got %+v", d)
	}
}
