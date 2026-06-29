# Passive Cursor-Over Spelling Suggestions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** When spellcheck is on and the cursor sits on a misspelled word, the status bar passively shows its suggestions (`✗ teh → the · ten · tea · ^R`), updating as the cursor moves — keyboard only, no mouse.

**Architecture:** Add a cheap `CurrentLine()` editor accessor so the per-frame lookup is O(line); memoize `spellSuggest`; add a `cursorSpellHint()` helper and a status-bar render branch (below the interactive `m.suggesting` menu, above the normal status).

**Tech Stack:** Go, the vendored `internal/textarea`, existing `spellOK`/`spellSuggest`/`wordUnderCursor`.

**Design spec:** `docs/superpowers/specs/2026-06-29-passive-spell-suggestions-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Status bar renders EVERY frame → the passive lookup must be O(line), not O(buffer). No
  `editor.Value()`/whole-buffer stringify in the per-frame path.
- Passive bar is informational only — it changes no editor behavior; `ctrl+r` is unchanged.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: O(line) foundation — `CurrentLine()`, memoized `spellSuggest`, `wordUnderCursor` rewrite

**Files:** Modify `internal/textarea/textarea.go`, `spell.go`, `main.go`; Test `internal/textarea/currentline_test.go`, `spell_test.go`

**Interfaces:**
- Produces: `(*Model).CurrentLine() string`; memoized `spellSuggest(word string, limit int) []string` (same signature, now cached); `wordUnderCursor()` reads the current line in O(line).

- [ ] **Step 1: Write the failing test (editor)** — create `internal/textarea/currentline_test.go`:

```go
package textarea

import "testing"

func TestCurrentLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(5)
	m.SetValue("first line\nsecond line\nthird")
	m.MoveToLine(1)
	if got := m.CurrentLine(); got != "second line" {
		t.Fatalf("CurrentLine = %q, want \"second line\"", got)
	}
	m.MoveToLine(2)
	if got := m.CurrentLine(); got != "third" {
		t.Fatalf("CurrentLine = %q, want \"third\"", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test ./internal/textarea/ -run TestCurrentLine 2>&1 | tail` → undefined `CurrentLine`.

- [ ] **Step 3: Add `CurrentLine()`** — in `internal/textarea/textarea.go` (near `CursorColumn`):

```go
// CurrentLine returns the text of the cursor's logical line (O(line)).
func (m *Model) CurrentLine() string {
	if m.row < 0 || m.row >= len(m.value) {
		return ""
	}
	return string(m.value[m.row])
}
```

(Confirm the line store is `m.value [][]rune` and current row is `m.row` — same fields Task 2 of the prior cycle used in `ReplaceRange`.)

- [ ] **Step 4: Run the editor test** — `/opt/homebrew/bin/go test ./internal/textarea/ -run TestCurrentLine -v 2>&1 | tail` → PASS.

- [ ] **Step 5: Memoize `spellSuggest` (test first)** — add to `spell_test.go`:

```go
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
```

- [ ] **Step 6: Run to verify it fails** — undefined `suggestCache`.

- [ ] **Step 7: Add the cache** — in `spell.go`, replace `spellSuggest` with a memoized version:

```go
import "strconv" // add to the import block if not present

var (
	suggestMu    sync.Mutex
	suggestCache = map[string][]string{}
)

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
```

- [ ] **Step 8: Rewrite `wordUnderCursor` to be O(line)** — in `main.go` (~line 362), replace the `editor.Value()`/`strings.Split` body so it reads only the current line:

```go
// wordUnderCursor returns the token spanning the editor cursor on its line (O(line)).
func (m *model) wordUnderCursor() (word string, start, end int, ok bool) {
	line := m.editor.CurrentLine()
	col := m.editor.CursorColumn()
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		if col >= s[0] && col <= s[1] {
			return string(runes[s[0]:s[1]]), s[0], s[1], true
		}
	}
	return "", 0, 0, false
}
```

(Remove the now-unused `strings.Split`/`editor.Line()` lines from the old body. If `strings` is still used elsewhere in main.go, keep the import; otherwise drop it.)

- [ ] **Step 9: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run TestCurrentLine -v 2>&1 | tail -3
/opt/homebrew/bin/go test . -run 'TestSpellSuggest|TestSuggest' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/currentline_test.go spell.go spell_test.go main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add internal/textarea/textarea.go internal/textarea/currentline_test.go spell.go spell_test.go main.go
git commit -m "spell: CurrentLine accessor + memoized spellSuggest + O(line) wordUnderCursor (passive-hint foundation)"
```

---

## Task 2: Passive status-bar hint (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `wordUnderCursor`, `spellOK`, `spellSuggest`, `m.analysis.spell` (and the modal flags).

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestCursorSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // inside "teh"

	// Spell OFF → no hint.
	m.analysis.spell = false
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint when spellcheck is off")
	}
	// Spell ON, cursor on misspelled word → hint with suggestions.
	m.analysis.spell = true
	w, sugg, ok := m.cursorSpellHint()
	if !ok || w != "teh" || len(sugg) == 0 {
		t.Fatalf("expected hint for teh, got w=%q sugg=%v ok=%v", w, sugg, ok)
	}
	// Cursor on a correct word → no hint.
	m.editor.SetCursor(5) // inside "cat"
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint on a correctly-spelled word")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — undefined `cursorSpellHint`.

- [ ] **Step 3: Add `cursorSpellHint`** — in `main.go`:

```go
// cursorSpellHint returns the misspelled word under the cursor and its suggestions
// for passive display, or ok=false. Active only when spellcheck is on, on the
// writing screen, and no modal/prompt is up.
func (m *model) cursorSpellHint() (word string, suggestions []string, ok bool) {
	if !m.analysis.spell || m.screen != screenWriting {
		return "", nil, false
	}
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.previewing || m.exportPrompt {
		return "", nil, false
	}
	w, _, _, found := m.wordUnderCursor()
	if !found || spellOK(w) {
		return "", nil, false
	}
	sugg := spellSuggest(w, 4)
	if len(sugg) == 0 {
		return "", nil, false
	}
	return w, sugg, true
}
```

(If the screen-state field/const names differ from `m.screen`/`screenWriting`, read the model and adapt.)

- [ ] **Step 4: Render the passive line** — in the status-line function, AFTER the `if m.suggesting { … }` branch and BEFORE the `if m.renaming { … }` branch, add:

```go
	if w, sugg, ok := m.cursorSpellHint(); ok {
		return "✗ " + w + " → " + strings.Join(sugg, " · ") + "  ·  ^R"
	}
```

(Ensure `strings` is imported in `main.go` — Task 1 may have left it imported; if not, add it.)

- [ ] **Step 5: Add a render-level test** — add to `smoke_test.go`:

```go
func TestStatusShowsSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "✗ teh") || !strings.Contains(out, "the") {
		t.Fatalf("status should show the spell hint for teh:\n%s", out)
	}
}
```

(If `smoke_test.go` lacks the `ansi` import, add `"github.com/charmbracelet/x/ansi"`.)

- [ ] **Step 6: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestCursorSpellHint|TestStatusShowsSpellHint|TestSuggest' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "spell: passive status-bar hint when the cursor is on a misspelled word"
```

---

## Self-Review

**Spec coverage:** O(line) `CurrentLine` + memoized `spellSuggest` + `wordUnderCursor` rewrite → Task 1; `cursorSpellHint` (gated by spell-on + writing screen + no modal) + status render branch + ordering (below `m.suggesting`, above `m.renaming`) → Task 2.

**Placeholder scan:** none — full code. Field-name confirmations (`m.value`/`m.row`; `m.screen`/`screenWriting`) are verification steps, not placeholders.

**Type consistency:** `CurrentLine() string` (Task 1) used by `wordUnderCursor` (Task 1) used by `cursorSpellHint` (Task 2); `spellSuggest(string,int) []string` unchanged signature (now memoized via `suggestCache`); `cursorSpellHint() (string,[]string,bool)` consumed by the status render branch.

**Risk:** the per-frame path must stay O(line) — `wordUnderCursor` now uses `CurrentLine()` (no whole-buffer stringify) and `spellSuggest` is memoized; the render branch must sit AFTER the interactive `m.suggesting` branch so the pick-menu still wins. Existing `ctrl+r` suggest tests must stay green (the `wordUnderCursor` rewrite is behavior-preserving).
