package main

import (
	_ "embed"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/client9/gospell"
	"okashi/internal/textarea"
)

//go:embed assets/en.aff
var affData string

//go:embed assets/en.dic
var dicData string

var (
	spellOnce sync.Once
	speller   *gospell.GoSpell
)

var (
	suggestMu    sync.Mutex
	suggestCache = map[string][]string{}
)

func loadSpeller() {
	speller, _ = gospell.NewGoSpellReader(strings.NewReader(affData), strings.NewReader(dicData))
}

var misspellStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Underline(true)

// spellOK reports whether word is spelled correctly (gospell handles case,
// contractions, and possessives).
func spellOK(word string) bool {
	spellOnce.Do(loadSpeller)
	if speller == nil {
		return true
	}
	return speller.Spell(word)
}

// spellSuggest returns up to limit correction candidates, best first (memoized —
// gospell's Suggest is heavier than Spell and this is called per frame by the
// passive status hint).
func spellSuggest(word string, limit int) []string {
	spellOnce.Do(loadSpeller)
	if speller == nil {
		return nil
	}
	key := word + "\x00" + strconv.Itoa(limit)
	suggestMu.Lock()
	if v, ok := suggestCache[key]; ok {
		suggestMu.Unlock()
		return v
	}
	suggestMu.Unlock()

	ss, err := speller.Suggest(word, limit)
	var out []string
	if err == nil {
		out = make([]string, 0, len(ss))
		for _, s := range ss {
			out = append(out, s.Word)
		}
	}
	suggestMu.Lock()
	if len(suggestCache) > 4096 {
		suggestCache = map[string][]string{}
	}
	suggestCache[key] = out
	suggestMu.Unlock()
	return out
}

// wordSpans returns [start,end) rune ranges of word tokens (letters + apostrophe).
func wordSpans(line string) [][2]int {
	var spans [][2]int
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		if unicode.IsLetter(runes[i]) {
			j := i + 1
			for j < len(runes) && (unicode.IsLetter(runes[j]) || runes[j] == '\'') {
				j++
			}
			spans = append(spans, [2]int{i, j})
			i = j
		} else {
			i++
		}
	}
	return spans
}

func isAllCaps(s string) bool { return s == strings.ToUpper(s) && s != strings.ToLower(s) }

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// spellDecorator flags misspelled words (red underline). All-caps acronyms and
// tokens with digits are skipped.
func spellDecorator(line string) []textarea.Decoration {
	var decos []textarea.Decoration
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		word := strings.Trim(string(runes[s[0]:s[1]]), "'")
		if word == "" || isAllCaps(word) || hasDigit(word) {
			continue
		}
		if !spellOK(word) {
			decos = append(decos, textarea.Decoration{Start: s[0], End: s[1], Style: misspellStyle})
		}
	}
	return decos
}
