# Grammar Tier 1 (heuristic) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** A pure-Go, offline "Grammar" toggle in the Analysis tab that highlights safe mechanical errors — doubled words, double/odd spacing, missing terminal punctuation, a/an.

**Architecture:** `grammarDecorator(line, isCursorLine)` is a per-line decorator (like spell/POS); a `Grammar` checkbox in the Analysis tab toggles it; `applyDecorator` composes it (spell first, then grammar, then POS).

**Tech Stack:** Go, `regexp`, lipgloss, the `textarea.Decoration` hook + the existing Analysis tab.

**Design spec:** `docs/superpowers/specs/2026-06-29-grammar-tier1-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- RUNE offsets (multibyte-safe). False-positive-safe rules only (no context confusables — those are Tier 2/LanguageTool).
- `grammarStyle` = a distinct underline (orange `#ffb86c` is taken by passive — use magenta `#ff79c6`).
- Compose order in `applyDecorator`: spellcheck spans FIRST (wins overlaps), then grammar, then POS.
- The "missing terminal punctuation" rule is suppressed on the cursor's own line (don't nag mid-typing) and on markdown headings / list items / blank lines.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: `grammarDecorator` (`grammar.go`)

**Files:** Create `grammar.go`, `grammar_test.go`

**Interfaces:**
- Produces: `grammarDecorator(line string, isCursorLine bool) []textarea.Decoration`; `grammarStyle`.

- [ ] **Step 1: Write the failing test** — create `grammar_test.go`:

```go
package main

import "testing"

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
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestGrammar 2>&1 | tail` → undefined `grammarDecorator`.

- [ ] **Step 3: Create `grammar.go`:**

```go
package main

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

var grammarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff79c6")).Underline(true) // magenta

var (
	gramDoubleWord = regexp.MustCompile(`(?i)\b(\p{L}+)(\s+)(\p{L}+)\b`)
	gramDoubleSpc  = regexp.MustCompile(`\S(  +)\S`)
	gramSpaceBefore = regexp.MustCompile(`(\s)[,.;:!?]`)
	gramAVowel     = regexp.MustCompile(`(?i)\ba(\s+)[aeiou]`)
	gramAnConsonant = regexp.MustCompile(`(?i)\ban(\s+)[bcdfgjklmnpqrstvwxyz]`)
)

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

	// Doubled word: flag the second word when it equals the first (case-insensitive).
	for _, m := range gramDoubleWord.FindAllStringSubmatchIndex(line, -1) {
		w1 := strings.ToLower(line[m[2]:m[3]])
		w2 := strings.ToLower(line[m[6]:m[7]])
		if w1 == w2 {
			add(b2r(m[6]), b2r(m[7]))
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
			last := tr[len(tr)-1]
			// allow a closing quote/paren after terminal punctuation
			if last == '"' || last == '\'' || last == ')' || last == '”' || last == '’' {
				if len(tr) > 1 {
					last = tr[len(tr)-2]
				}
			}
			if !strings.ContainsRune(".!?:…", last) && unicode.IsLetter(last) {
				end := len(strings.TrimRight(line, " \t"))
				add(b2r(end)-1, b2r(end))
			}
		}
	}
	return decos
}
```

(Reuses `listItemRe` from main.go. The occupied-range rule prevents double-covering, like `syntaxDecorator` did.)

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run TestGrammar -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w grammar.go grammar_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add grammar.go grammar_test.go
git commit -m "grammar: heuristic grammarDecorator (doubled words, spacing, a/an, missing terminal punctuation)"
```

---

## Task 2: Grammar toggle in the Analysis tab (`inspector.go`, `main.go`)

**Files:** Modify `inspector.go`, `main.go`; Test `inspector_test.go`, `smoke_test.go`

**Interfaces:** Consumes `grammarDecorator` (Task 1); changes `analysisState`, `analysisRowY`, `inspectorAnalysisRowAtY`, the Analysis body render, `applyDecorator`, the Analysis click switch.

- [ ] **Step 1: Write the failing test** — add to `inspector_test.go`:

```go
func TestAnalysisGrammarRow(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	out := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{grammar: true})
	if !strings.Contains(out, "Grammar") || !strings.Contains(out, "[x] Grammar") {
		t.Fatalf("Analysis tab should show a checked Grammar row:\n%s", out)
	}
}

func TestAnalysisRowYWithGrammar(t *testing.T) {
	// 5 checkboxes now: Spellcheck, Grammar, Adverb, Adjective, Passive.
	for i := 0; i < 5; i++ {
		y := analysisRowY(i)
		if r, ok := inspectorAnalysisRowAtY(y); !ok || r != i {
			t.Fatalf("analysisRowY(%d)=%d → inspectorAnalysisRowAtY=%d,%v", i, y, r, ok)
		}
	}
}
```

Add to `smoke_test.go`:

```go
func TestGrammarToggleClick(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("the the cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabAnalysis
	m.layout()
	// Click the Grammar row (index 1).
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: analysisRowY(1), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.grammar {
		t.Fatal("clicking the Grammar row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("grammar on should set the editor Decorator")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — undefined `analysisState.grammar` / row mismatch.

- [ ] **Step 3: `inspector.go`** — `analysisState{ spell, grammar, adverb, adjective, passive bool }`; `analysisRowY(i)` → `[5]int{4, 5, 8, 9, 10}[i]` (Spellcheck 4, Grammar 5, blank 6, Syntax header 7, Adverb 8, Adjective 9, Passive 10); `inspectorAnalysisRowAtY` loops `i < 5`. The `tabAnalysis` body becomes:
  ```
  [x/ ] Spellcheck
  [x/ ] Grammar           (label in grammarStyle)
                          (blank)
  Syntax                  (header)
  [x/ ] Adverb / Adjective / Passive/weak
  ```

- [ ] **Step 4: `main.go`** — `applyDecorator` composes grammar:
  ```go
  func (m *model) applyDecorator() {
      a := m.analysis
      posOn := a.adverb || a.adjective || a.passive
      row := m.editor.Line()
      grammarOn := a.grammar
      build := func(line string) []textarea.Decoration {
          var d []textarea.Decoration
          if a.spell {
              d = append(d, spellDecorator(line)...)
          }
          if grammarOn {
              d = append(d, grammarDecorator(line, line == m.editor.CurrentLine())...) // NOTE: see below
          }
          if posOn {
              d = append(d, posDecorator(line, a.adverb, a.adjective, a.passive)...)
          }
          return d
      }
      if a.spell || grammarOn || posOn {
          m.editor.Decorator = build
      } else {
          m.editor.Decorator = nil
      }
  }
  ```
  The cursor-line suppression: the Decorator gets a line string, not its index. Compare by content is wrong if two lines are identical. Instead, capture the cursor line TEXT once: `cursorLine := m.editor.CurrentLine()` and pass `isCursorLine := grammarOn && line == cursorLine` — acceptable (identical duplicate lines both suppress, a benign edge). Use `_ = row`. (If you prefer exactness, add a `Decorator` variant keyed by row — out of scope; content compare is fine for v1.)

  The Analysis click switch maps rows: `0 spell, 1 grammar, 2 adverb, 3 adjective, 4 passive` → flip + `applyDecorator`.

- [ ] **Step 5: Run; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestAnalysisGrammarRow|TestAnalysisRowYWithGrammar|TestGrammarToggleClick|TestAnalysis|TestPOSToggle|TestSpellcheckToggle' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go main.go inspector_test.go smoke_test.go
git commit -m "grammar: Analysis-tab Grammar checkbox toggles the heuristic grammar decorator"
```

---

## Self-Review

**Spec coverage:** the heuristic rules (doubled word, spacing, a/an, terminal punctuation w/ cursor-line + heading/list exemptions) → Task 1; the Grammar toggle + analysisState/row geometry + applyDecorator compose + click → Task 2.

**Placeholder scan:** none — full code. The cursor-line-suppression note explains a deliberate v1 simplification (content compare), not a gap.

**Type consistency:** `grammarDecorator(string, bool) []textarea.Decoration` (Task 1) used by `applyDecorator` (Task 2); `analysisState{spell,grammar,adverb,adjective,passive}` + `analysisRowY` `[5]int{4,5,8,9,10}` consistent across render, hit-test, and the click switch.

**Risk:** the Analysis row geometry changes (5 checkboxes; Syntax section shifts down) — `analysisRowY`/`inspectorAnalysisRowAtY` + the click switch must agree; the controller re-verifies click alignment empirically (rune-column) for all 5 checkboxes. The byte→rune `b2r` in grammar.go is the multibyte guard.
