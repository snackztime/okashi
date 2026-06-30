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
// ("I had had enough", "he knew that that was wrong"). Exempt from the doubled-word rule.
var doubledWordOK = map[string]bool{
	"had": true, "that": true, "is": true, "do": true, "who": true,
}

// grammarPhrase is a case-insensitive pattern → replacement (regexp template, so $1 can
// echo a captured group) with a reason. Covers verb-of slips, redundancies, and common
// nonstandard forms — ported in spirit from write-good/proselint rule sets.
type grammarPhrase struct {
	pat  *regexp.Regexp
	repl string
	msg  string
}

var grammarPhrases = []grammarPhrase{
	{regexp.MustCompile(`(?i)\b(could|would|should|must|might|may) of\b`), "$1 have", "should be “have”"},
	{regexp.MustCompile(`(?i)\bvery unique\b`), "unique", "redundant — “unique” is absolute"},
	{regexp.MustCompile(`(?i)\bend result\b`), "result", "redundant"},
	{regexp.MustCompile(`(?i)\bpast history\b`), "history", "redundant"},
	{regexp.MustCompile(`(?i)\bfree gift\b`), "gift", "redundant"},
	{regexp.MustCompile(`(?i)\badded bonus\b`), "bonus", "redundant"},
	{regexp.MustCompile(`(?i)\bclose proximity\b`), "proximity", "redundant"},
	{regexp.MustCompile(`(?i)\bPIN number\b`), "PIN", "redundant"},
	{regexp.MustCompile(`(?i)\bATM machine\b`), "ATM", "redundant"},
	{regexp.MustCompile(`(?i)\balot\b`), "a lot", "nonstandard — two words"},
	{regexp.MustCompile(`(?i)\birregardless\b`), "regardless", "nonstandard"},
}

// closingQuote reports whether r is a closing quote/paren that may follow end punctuation.
func closingQuote(r rune) bool {
	return r == '"' || r == '\'' || r == ')' || r == '”' || r == '’'
}

// grammarFindings returns heuristic grammar/spacing findings for ONE line: rune-range
// offsets within that line, each with a clickable replacement and a reason. isCursorLine
// suppresses the missing-terminal-punctuation rule (you are still typing the line).
// grammarDecorator (underlines) and the passive hint / click-to-fix all share this.
func grammarFindings(line string, isCursorLine bool) []grammarFinding {
	runes := []rune(line)
	occupied := make([]bool, len(runes))
	var out []grammarFinding
	b2r := func(b int) int { return len([]rune(line[:b])) }
	add := func(rs, re int, msg string, repl string) {
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
		out = append(out, grammarFinding{Start: rs, End: re, Message: msg, Replacements: []string{repl}})
	}

	// Doubled word: underline both words, fix → the single word.
	words := gramWord.FindAllStringIndex(line, -1)
	for i := 1; i < len(words); i++ {
		between := line[words[i-1][1]:words[i][0]]
		w1 := line[words[i-1][0]:words[i-1][1]]
		w2 := line[words[i][0]:words[i][1]]
		if strings.TrimSpace(between) == "" && strings.EqualFold(w1, w2) && !doubledWordOK[strings.ToLower(w2)] {
			add(b2r(words[i-1][0]), b2r(words[i][1]), "repeated word", w1)
		}
	}
	// a → an before a vowel sound.
	for _, m := range gramAVowel.FindAllStringSubmatchIndex(line, -1) {
		s := b2r(m[0])
		fix := "an"
		if runes[s] == 'A' {
			fix = "An"
		}
		add(s, s+1, "use “an” before a vowel sound", fix)
	}
	// an → a before a consonant sound.
	for _, m := range gramAnConsonant.FindAllStringSubmatchIndex(line, -1) {
		s := b2r(m[0])
		fix := "a"
		if runes[s] == 'A' {
			fix = "A"
		}
		add(s, s+2, "use “a” before a consonant sound", fix)
	}
	// Double space → single space.
	for _, m := range gramDoubleSpc.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[2]), b2r(m[3]), "double space", " ")
	}
	// Space before punctuation → remove the space.
	for _, m := range gramSpaceBefore.FindAllStringSubmatchIndex(line, -1) {
		add(b2r(m[2]), b2r(m[3]), "space before punctuation", "")
	}
	// Phrase rules (verb-of, redundancies, common slips).
	for _, p := range grammarPhrases {
		for _, m := range p.pat.FindAllStringIndex(line, -1) {
			matched := line[m[0]:m[1]]
			add(b2r(m[0]), b2r(m[1]), p.msg, p.pat.ReplaceAllString(matched, p.repl))
		}
	}
	// Missing terminal punctuation (non-cursor paragraph lines only).
	if !isCursorLine {
		t := strings.TrimRight(line, " \t")
		tr := []rune(t)
		if len(tr) > 0 && !strings.HasPrefix(strings.TrimSpace(t), "#") && !listItemRe.MatchString(line) {
			li := len(tr) - 1
			last := tr[li]
			if closingQuote(last) && len(tr) > 1 {
				li = len(tr) - 2
				last = tr[li]
			}
			if !strings.ContainsRune(".!?:…", last) && unicode.IsLetter(last) {
				add(li, li+1, "missing end punctuation", string(last)+".")
			}
		}
	}
	return out
}

// grammarDecorator flags heuristic grammar issues on one line as green underlines.
func grammarDecorator(line string, isCursorLine bool) []textarea.Decoration {
	var decos []textarea.Decoration
	for _, f := range grammarFindings(line, isCursorLine) {
		decos = append(decos, textarea.Decoration{Start: f.Start, End: f.End, Style: grammarStyle})
	}
	return decos
}
