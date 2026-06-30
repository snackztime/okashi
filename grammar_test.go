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
	// Doubled word: "the the cat" → both words "the the" (runes 0..7), fix → "the".
	if d := grammarDecorator("the the cat", false); !hasGSpan(d, 0, 7) {
		t.Fatalf("doubled-word span [0,7) missing: %+v", d)
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
	// must still be flagged with the single-word fix (regression: RE2 has no backrefs).
	found := false
	for _, f := range grammarFindings("He went to the the store", false) {
		if f.Message == "repeated word" && len(f.Replacements) > 0 && f.Replacements[0] == "the" {
			found = true
		}
	}
	if !found {
		t.Fatal("doubled 'the the' mid-sentence not flagged with fix 'the'")
	}
	// No false positive when the words differ.
	for _, f := range grammarFindings("the cat the dog", false) {
		if f.Message == "repeated word" {
			t.Fatal("non-adjacent 'the' must not be flagged as doubled")
		}
	}
}

func TestGrammarDoubledWordWhitelist(t *testing.T) {
	// Legitimately-doubled words must NOT be flagged...
	for _, s := range []string{"I had had enough.", "He knew that that was wrong."} {
		for _, f := range grammarFindings(s, false) {
			if f.Message == "repeated word" {
				t.Fatalf("%q: valid doubled word must not be flagged: %+v", s, f)
			}
		}
	}
	// ...but an accidental double still is, with the single-word fix.
	flagged := false
	for _, f := range grammarFindings("the the cat", false) {
		if f.Message == "repeated word" && len(f.Replacements) > 0 && f.Replacements[0] == "the" {
			flagged = true
		}
	}
	if !flagged {
		t.Fatal("'the the' must still be flagged")
	}
}

func TestGrammarClosingQuoteRune(t *testing.T) {
	// A letter followed by a closing quote, no terminal punctuation: flag the LETTER.
	s := `She said hello"`
	got := ""
	for _, x := range grammarDecorator(s, false) {
		got = string([]rune(s)[x.Start:x.End])
	}
	if got != "o" {
		t.Fatalf("%q: want the letter 'o' flagged, got %q", s, got)
	}
	// Terminal punctuation before the quote → no flag.
	if d := grammarDecorator(`She said hello."`, false); len(d) != 0 {
		t.Fatalf("terminal punct before closing quote should not flag: %+v", d)
	}
}

func TestGrammarRulesHaveFixes(t *testing.T) {
	want := []struct {
		line, span, fix, msg string
	}{
		{"I could of gone", "could of", "could have", "should be “have”"},
		{"that is very unique", "very unique", "unique", "redundant — “unique” is absolute"},
		{"I have alot", "alot", "a lot", "nonstandard — two words"},
		{"a apple", "a", "an", "use “an” before a vowel sound"},
		{"an cat here", "an", "a", "use “a” before a consonant sound"},
		{"the the cat", "the the", "the", "repeated word"},
	}
	for _, w := range want {
		var got *grammarFinding
		for i := range grammarFindings(w.line, false) {
			f := grammarFindings(w.line, false)[i]
			if string([]rune(w.line)[f.Start:f.End]) == w.span {
				got = &f
				break
			}
		}
		if got == nil {
			t.Errorf("%q: no finding spanning %q", w.line, w.span)
			continue
		}
		if got.Replacements[0] != w.fix || got.Message != w.msg {
			t.Errorf("%q span %q → fix %q msg %q, want fix %q msg %q",
				w.line, w.span, got.Replacements[0], got.Message, w.fix, w.msg)
		}
	}
}

func TestGrammarNoFalsePositives(t *testing.T) {
	// Correct prose that earlier rules wrongly flagged with a bad auto-fix.
	clean := []string{
		"He may of course be right.",
		"They could of course do that.",
		"I will see you in May of 2020.",
		"a unique opportunity",
		"a university degree",
		"a one-time event",
		"I waited an hour",
		"an honest answer",
	}
	for _, s := range clean {
		if f := grammarFindings(s, true); len(f) != 0 {
			t.Errorf("%q should be clean, got %+v", s, f)
		}
	}
	// Real errors must still fire with the right fix.
	real := []struct{ line, fix string }{
		{"I could of gone", "could have"},
		{"a apple", "an"},
		{"an cat", "a"},
	}
	for _, r := range real {
		ok := false
		for _, f := range grammarFindings(r.line, true) {
			if len(f.Replacements) > 0 && f.Replacements[0] == r.fix {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%q should flag fix %q", r.line, r.fix)
		}
	}
}
