package main

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

//go:embed assets/words.txt
var wordsFile string

var (
	spellOnce sync.Once
	spellSet  map[string]struct{}
)

func loadSpellSet() {
	spellSet = make(map[string]struct{}, 240000)
	for _, w := range strings.Split(wordsFile, "\n") {
		if w != "" {
			spellSet[w] = struct{}{}
		}
	}
}

var misspellStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Underline(true)

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

// spellDecorator flags misspelled words (red underline). Words in the embedded
// list, shorter than 3 letters, or all-caps (acronyms) are skipped.
func spellDecorator(line string) []textarea.Decoration {
	spellOnce.Do(loadSpellSet)
	var decos []textarea.Decoration
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		w := runes[s[0]:s[1]]
		word := strings.Trim(string(w), "'")
		if len([]rune(word)) < 3 {
			continue
		}
		if word == strings.ToUpper(word) { // all-caps acronym
			continue
		}
		if _, ok := spellSet[strings.ToLower(word)]; ok {
			continue
		}
		decos = append(decos, textarea.Decoration{Start: s[0], End: s[1], Style: misspellStyle})
	}
	return decos
}
