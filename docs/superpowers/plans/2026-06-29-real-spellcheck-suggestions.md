# Real Spellcheck + Suggestions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Replace the wordlist spellcheck with the `gospell` (hunspell) engine and add a `ctrl+r` bottom-menu of spelling suggestions that replaces the misspelled word under the cursor.

**Architecture:** `spell.go` swaps set-lookup for `gospell.GoSpell` (embedded `en` hunspell dict). The vendored editor gains `CursorColumn()`/`ReplaceRange()` so a chosen suggestion replaces exactly the cursor word. `main.go` adds a modal suggestions menu (like the rename prompt).

**Tech Stack:** Go, `github.com/client9/gospell` (pinned `v0.9.2`), `//go:embed`, the vendored `internal/textarea`.

**Design spec:** `docs/superpowers/specs/2026-06-29-real-spellcheck-suggestions-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`. Network available for `go get`.
- `assets/en.dic`, `assets/en.aff`, `assets/en.LICENSE` are placed by the CONTROLLER (fetched files, not subagent-generatable) — Task 1's implementer treats them as already present and embeds/commits them.
- Trigger key is `ctrl+r` (verified free in both the app and the editor keymap; avoids dead keys ctrl+i/j/m/h).
- gospell handles case/possessive/contraction — pass raw tokens (`don't`, `Sarah's`) to `spellOK`.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: gospell engine swap (`spell.go`)

**Files:** Modify `spell.go`, `spell_test.go`; delete `assets/words.txt`; add `assets/en.dic`/`assets/en.aff`/`assets/en.LICENSE` (controller-placed); modify `go.mod`/`go.sum`

**Interfaces:**
- Produces: `spellOK(word string) bool`; `spellSuggest(word string, limit int) []string`; `spellDecorator(line string) []textarea.Decoration` (unchanged signature); `wordSpans` retained.

- [ ] **Step 1: Add gospell**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/go get github.com/client9/gospell@v0.9.2
ls assets/en.dic assets/en.aff assets/en.LICENSE   # controller-placed; confirm present
```

- [ ] **Step 2: Write the failing test** — replace the body of `spell_test.go`:

```go
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
```

- [ ] **Step 3: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestSpell 2>&1 | tail` → undefined `spellOK`/`spellSuggest`.

- [ ] **Step 4: Rewrite `spell.go`:**

```go
package main

import (
	_ "embed"
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

// spellSuggest returns up to limit correction candidates, best first.
func spellSuggest(word string, limit int) []string {
	spellOnce.Do(loadSpeller)
	if speller == nil {
		return nil
	}
	ss, err := speller.Suggest(word, limit)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Word)
	}
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
```

- [ ] **Step 5: Run tests; gofmt; commit (incl. asset swap)**

```bash
/opt/homebrew/bin/go mod tidy
/opt/homebrew/bin/go test . -run TestSpell -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w spell.go spell_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git rm assets/words.txt
git add spell.go spell_test.go go.mod go.sum assets/en.dic assets/en.aff assets/en.LICENSE
git commit -m "spell: swap wordlist for gospell (hunspell) engine — real morphological checking + suggestions"
```

---

## Task 2: Editor cursor primitives (`internal/textarea`)

**Files:** Modify `internal/textarea/textarea.go`; Test `internal/textarea/replacerange_test.go`

**Interfaces:**
- Produces: `(*Model).CursorColumn() int`; `(*Model).ReplaceRange(start, end int, s string)`.

- [ ] **Step 1: Confirm the cursor field names** — read around `SetCursor`/`InsertString` in `internal/textarea/textarea.go`; the current line is `m.value[m.row]` (`[]rune`), the cursor column is `m.col`. (If the field names differ, adapt the code below to the real names.)

- [ ] **Step 2: Write the failing test** — create `internal/textarea/replacerange_test.go`:

```go
package textarea

import "testing"

func TestReplaceRange(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(4)
	m.SetValue("the cat")
	// cursor is on the only line (row 0); replace "cat" [4,7) with "dog".
	m.ReplaceRange(4, 7, "dog")
	if got := m.Value(); got != "the dog" {
		t.Fatalf("Value = %q, want \"the dog\"", got)
	}
	if got := m.CursorColumn(); got != 7 {
		t.Fatalf("CursorColumn = %d, want 7 (after \"dog\")", got)
	}
}

func TestReplaceRangeMultibyte(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(4)
	m.SetValue("café teh")
	// "teh" is runes [5,8) (é is one rune); replace with "tea".
	m.ReplaceRange(5, 8, "tea")
	if got := m.Value(); got != "café tea" {
		t.Fatalf("Value = %q, want \"café tea\"", got)
	}
}
```

- [ ] **Step 3: Run to verify it fails** — `/opt/homebrew/bin/go test ./internal/textarea/ -run TestReplaceRange 2>&1 | tail` → undefined `ReplaceRange`/`CursorColumn`.

- [ ] **Step 4: Implement** — add to `internal/textarea/textarea.go` (near `SetCursor`):

```go
// CursorColumn returns the cursor's rune column on the current logical line.
func (m *Model) CursorColumn() int { return m.col }

// ReplaceRange replaces runes [start,end) on the current line with s and places
// the cursor just after the inserted text. Out-of-range args are clamped.
func (m *Model) ReplaceRange(start, end int, s string) {
	line := m.value[m.row]
	if start < 0 {
		start = 0
	}
	if end > len(line) {
		end = len(line)
	}
	if start > end {
		return
	}
	repl := []rune(s)
	newLine := make([]rune, 0, len(line)-(end-start)+len(repl))
	newLine = append(newLine, line[:start]...)
	newLine = append(newLine, repl...)
	newLine = append(newLine, line[end:]...)
	m.value[m.row] = newLine
	m.col = start + len(repl)
	m.SetCursor(m.col) // re-clamp + refresh cursor/viewport state
}
```

(Read how `InsertString` invalidates the wrap cache; if it calls a helper after mutating `m.value`, call the same helper here so the windowed `View` re-wraps the edited line. `SetCursor` already triggers the needed cursor refresh.)

- [ ] **Step 5: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestReplaceRange|TestCursor' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/replacerange_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add internal/textarea/textarea.go internal/textarea/replacerange_test.go
git commit -m "textarea: CursorColumn + ReplaceRange (replace a rune range on the cursor line)"
```

---

## Task 3: Suggestions menu (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `spellOK`/`spellSuggest`/`wordSpans` (Task 1), `editor.CursorColumn`/`editor.ReplaceRange`/`editor.Line`/`editor.Value` (Task 2).

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestSuggestMenuReplacesWord(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // cursor inside "teh"
	// ctrl+r opens the menu.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(model)
	if !m.suggesting {
		t.Fatalf("ctrl+r on a misspelled word should open the menu; status=%q", m.status)
	}
	if len(m.suggestions) == 0 || m.suggestions[0] != "the" {
		t.Fatalf("expected suggestions led by \"the\", got %v", m.suggestions)
	}
	// enter applies the top suggestion.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.suggesting {
		t.Fatal("enter should close the menu")
	}
	if got := m.editor.Value(); got != "the cat" {
		t.Fatalf("after applying, value = %q, want \"the cat\"", got)
	}
}

func TestSuggestMenuCorrectWordNoMenu(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("the cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(model)
	if m.suggesting {
		t.Fatal("ctrl+r on a correct word should NOT open the menu")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — undefined `m.suggesting`.

- [ ] **Step 3: Add state + word-under-cursor + matchCase** — to the `model` struct add:

```go
	suggesting             bool
	suggestions            []string
	suggestIndex           int
	suggestStart, suggestEnd int
	suggestWord            string
```

And helpers (near the other model methods):

```go
// wordUnderCursor returns the token spanning the editor cursor on its line.
func (m *model) wordUnderCursor() (word string, start, end int, ok bool) {
	lines := strings.Split(m.editor.Value(), "\n")
	row := m.editor.Line()
	if row < 0 || row >= len(lines) {
		return "", 0, 0, false
	}
	col := m.editor.CursorColumn()
	runes := []rune(lines[row])
	for _, s := range wordSpans(lines[row]) {
		if col >= s[0] && col <= s[1] {
			return string(runes[s[0]:s[1]]), s[0], s[1], true
		}
	}
	return "", 0, 0, false
}

// matchCase applies orig's capitalization pattern to sugg.
func matchCase(orig, sugg string) string {
	if orig == "" || sugg == "" {
		return sugg
	}
	if isAllCaps(orig) {
		return strings.ToUpper(sugg)
	}
	or := []rune(orig)
	if unicode.IsUpper(or[0]) {
		sr := []rune(sugg)
		sr[0] = unicode.ToUpper(sr[0])
		return string(sr)
	}
	return sugg
}
```

(Ensure `strings` and `unicode` are imported in `main.go`.)

- [ ] **Step 4: Handle `ctrl+r` + the modal menu** — in `Update`'s key handling for the writing screen, add a `ctrl+r` case (guarded by not already in `m.renaming`/`m.goalPromptField`/`m.suggesting`):

```go
		case "ctrl+r":
			w, s, e, ok := m.wordUnderCursor()
			if !ok {
				m.status = "no word under cursor"
				return m, nil
			}
			if spellOK(w) {
				m.status = "‘" + w + "’ looks correct"
				return m, nil
			}
			sugg := spellSuggest(w, 7)
			if len(sugg) == 0 {
				m.status = "no suggestions for ‘" + w + "’"
				return m, nil
			}
			m.suggesting = true
			m.suggestions = sugg
			m.suggestIndex = 0
			m.suggestWord = w
			m.suggestStart, m.suggestEnd = s, e
			return m, nil
```

And a modal block (place near the `if m.renaming {` block, BEFORE the editor receives keys):

```go
	if m.suggesting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.suggesting = false
				m.status = "suggestion cancelled"
				return m, nil
			case "left":
				if m.suggestIndex > 0 {
					m.suggestIndex--
				}
				return m, nil
			case "right":
				if m.suggestIndex < len(m.suggestions)-1 {
					m.suggestIndex++
				}
				return m, nil
			case "enter":
				m.applySuggestion(m.suggestIndex)
				return m, nil
			default:
				if len(key.String()) == 1 && key.String() >= "1" && key.String() <= "9" {
					i := int(key.String()[0] - '1')
					if i < len(m.suggestions) {
						m.applySuggestion(i)
					}
				}
				return m, nil
			}
		}
		return m, nil
	}
```

And the apply method:

```go
func (m *model) applySuggestion(i int) {
	if i < 0 || i >= len(m.suggestions) {
		m.suggesting = false
		return
	}
	chosen := matchCase(m.suggestWord, m.suggestions[i])
	m.editor.ReplaceRange(m.suggestStart, m.suggestEnd, chosen)
	m.status = "‘" + m.suggestWord + "’ → ‘" + chosen + "’"
	m.suggesting = false
}
```

- [ ] **Step 5: Render the menu** — in the status-line function, before the `if m.renaming {` branch:

```go
	if m.suggesting {
		parts := make([]string, len(m.suggestions))
		for i, s := range m.suggestions {
			if i == m.suggestIndex {
				parts[i] = selectedStyle.Render(s)
			} else {
				parts[i] = s
			}
		}
		return "suggest ▸ " + strings.Join(parts, " · ")
	}
```

- [ ] **Step 6: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run TestSuggest -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "spell: ctrl+r spelling-suggestions menu (replace cursor word, case-preserving)"
```

---

## Self-Review

**Spec coverage:** engine swap + `spellOK`/`spellSuggest` + decorator skip rules + asset swap → Task 1; `CursorColumn`/`ReplaceRange` → Task 2; `wordUnderCursor` + `ctrl+r` + modal menu + `applySuggestion` + `matchCase` + render → Task 3.

**Placeholder scan:** none — full code. Task 2 Step 1 asks the implementer to confirm `m.row`/`m.col`/`m.value` field names against the real file (a verification step, not a placeholder).

**Type consistency:** `spellOK(string) bool`, `spellSuggest(string,int) []string`, `isAllCaps`/`hasDigit` (Task 1) used by Task 3's handler/matchCase; `CursorColumn() int`/`ReplaceRange(int,int,string)` (Task 2) used by `wordUnderCursor`/`applySuggestion` (Task 3); `m.suggesting`/`suggestions`/`suggestIndex`/`suggestStart`/`suggestEnd`/`suggestWord` consistent across struct, handler, apply, render.

**Cross-task / risk:** Task 1 depends on controller-placed `assets/en.{dic,aff,LICENSE}`; if absent the embed fails to compile — controller must place them before dispatch. Task 2's field-name assumption (`m.row`/`m.col`/`m.value`) must be verified against the file. The `ctrl+r` modal must be checked BEFORE the editor consumes keys (same ordering as `m.renaming`). gospell pulls `golang.org/x/text`/`x/sync` bumps — `go mod tidy` + commit go.sum.
