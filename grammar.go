package main

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

var grammarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")).Underline(true) // green — distinct from the red spellcheck underline

var (
	gramWord        = regexp.MustCompile(`\p{L}+`)
	gramDoubleSpc   = regexp.MustCompile(`\S(  +)\S`)
	gramSpaceBefore = regexp.MustCompile(`(\s)[,.;:!?]`)
	gramAVowel      = regexp.MustCompile(`(?i)\ba(\s+)[aeiou]`)
	gramAnConsonant = regexp.MustCompile(`(?i)\ban(\s+)[bcdfgjklmnpqrstvwxyz]`)
)

// doubledWordOK lists words that legitimately repeat in correct English
// ("I had had enough", "he knew that that was wrong"). They are exempt from the
// doubled-word rule to avoid false positives on valid prose.
var doubledWordOK = map[string]bool{
	"had": true, "that": true, "is": true, "do": true, "who": true,
}

// grammarDecorator flags safe mechanical grammar/spacing issues on one line.
// isCursorLine suppresses the missing-terminal-punctuation rule (you're typing it).
func grammarDecorator(line string, isCursorLine bool) []textarea.Decoration {
	runes := []rune(line)
	occupied := make([]bool, len(runes))
	var decos []textarea.Decoration
	// b2r maps a byte offset in line to a rune index.
	b2r := func(b int) int { return len([]rune(line[:b])) }
	add := func(rs, re int) {
		if rs < 0 || re > len(runes) || rs >= re {
			return
		}
		for i := rs; i < re; i++ {
			if occupied[i] {
				return
			}
		}
		for i := rs; i < re; i++ {
			occupied[i] = true
		}
		decos = append(decos, textarea.Decoration{Start: rs, End: re, Style: grammarStyle})
	}

	// Doubled word: flag a word that equals its predecessor (case-insensitive,
	// only whitespace between). Scanning adjacent word pairs catches duplicates at
	// any position — Go's RE2 has no backreferences, so a \1 regex won't work.
	words := gramWord.FindAllStringIndex(line, -1)
	for i := 1; i < len(words); i++ {
		between := line[words[i-1][1]:words[i][0]]
		cur := line[words[i][0]:words[i][1]]
		if strings.TrimSpace(between) == "" &&
			strings.EqualFold(line[words[i-1][0]:words[i-1][1]], cur) &&
			!doubledWordOK[strings.ToLower(cur)] {
			add(b2r(words[i][0]), b2r(words[i][1]))
		}
	}
	// a → an before a vowel: flag the "a".
	for _, m := range gramAVowel.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[0]), b2r(m[0])+1)
	}
	// an → a before a consonant: flag the "an".
	for _, m := range gramAnConsonant.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[0]), b2r(m[0])+2)
	}
	// Double space between non-space chars: flag the run of spaces.
	for _, m := range gramDoubleSpc.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[2]), b2r(m[3]))
	}
	// Space before punctuation.
	for _, m := range gramSpaceBefore.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[2]), b2r(m[3]))
	}
	// Missing terminal punctuation (non-cursor paragraph lines only).
	if !isCursorLine {
		t := strings.TrimRight(line, " \t")
		tr := []rune(t)
		if len(tr) > 0 && !strings.HasPrefix(strings.TrimSpace(t), "#") && !listItemRe.MatchString(line) {
			li := len(tr) - 1 // rune index (into t, == into line's prefix) of the char to test
			last := tr[li]
			// a closing quote/paren may follow terminal punctuation — test the char before it
			if (last == '"' || last == '\'' || last == ')' || last == '”' || last == '’') && len(tr) > 1 {
				li = len(tr) - 2
				last = tr[li]
			}
			if !strings.ContainsRune(".!?:…", last) && unicode.IsLetter(last) {
				add(li, li+1) // underline the offending letter, not the trailing quote
			}
		}
	}
	return decos
}
