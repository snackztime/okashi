package main

// grammarFinding is one issue located within a source line (RUNE offsets within that
// line), produced by an optional on-device "deep grammar" backend (Tier 2). Rune
// offsets let findings slot straight into the editor's per-line Decorator + click-to-fix.
type grammarFinding struct {
	Line, Start, End int
	Message          string
	Replacements     []string
}

// grammarChecker is an optional on-device deep-grammar backend (NSSpellChecker or Apple
// Intelligence). It exists only in the build-tagged macOS build; the default (pure-Go)
// build's appleGrammarChecker() returns nil and okashi runs the heuristics alone.
type grammarChecker interface {
	Name() string                                // "Apple Intelligence" | "system checker"
	Available() bool                             // runtime availability
	Check(text string) ([]grammarFinding, error) // whole-document → per-line findings
}

// newGrammarChecker is the constructor the model calls at startup. It is a package var
// so tests in the pure-Go build can inject a fake without cgo.
var newGrammarChecker = appleGrammarChecker

// utf16ToRune converts a UTF-16 code-unit offset into a rune offset in s. NSSpellChecker
// reports ranges in UTF-16 code units; the editor works in runes.
func utf16ToRune(s string, u16 int) int {
	units, runes := 0, 0
	for _, r := range s {
		if units >= u16 {
			break
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
		runes++
	}
	return runes
}

// runeOffsetToLine maps an absolute rune offset (newlines counting as one rune) to a
// (lineIndex, runeColumn) pair within lines.
func runeOffsetToLine(lines []string, abs int) (int, int) {
	for i, ln := range lines {
		n := len([]rune(ln))
		if abs <= n {
			return i, abs
		}
		abs -= n + 1 // the '\n'
	}
	last := len(lines) - 1
	if last < 0 {
		return 0, 0
	}
	return last, len([]rune(lines[last]))
}
