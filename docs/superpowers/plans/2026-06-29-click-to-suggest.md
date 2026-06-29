# Click-to-Suggest Spellcheck Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** Click a misspelled word in the editor → its suggestions appear in the bottom bar → click a suggestion to apply it (case-preserved).

**Architecture:** A new `ClickTo` maps an editor-relative display cell to the cursor (mirroring the windowed `View`'s `top`/`locateRow` math). An editor-click handler calls it so the existing `cursorSpellHint` shows suggestions; a status-row hit-test makes those suggestions clickable, reusing `applySuggestion`.

**Tech Stack:** Go, the vendored `internal/textarea`, the existing `cursorSpellHint`/`applySuggestion`/`spellSuggest`.

**Design spec:** `docs/superpowers/specs/2026-06-29-click-to-suggest-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- The editor has NO gutter (`Prompt = ""`, no line numbers), so a click column maps directly to a source column with no prompt offset.
- `ClickTo` must mirror `View`'s top: `top := m.offset; if m.Typewriter && m.height > 0 { top = m.cursorLineNumber() - m.height/2 }`; clamp `target := top+displayRow` to `[0, displayHeight()-1]`.
- Reuse `cursorSpellHint()`/`applySuggestion(i)`/`spellSuggest` — do not duplicate suggestion logic.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: `ClickTo` — map a display cell to the cursor (`internal/textarea`)

**Files:** Modify `internal/textarea/textarea.go`; Test `internal/textarea/clickto_test.go`

**Interfaces:**
- Produces: `(*Model).ClickTo(displayRow, displayCol int)` — sets the cursor from an editor-relative display position.

- [ ] **Step 1: Write the failing test** — create `internal/textarea/clickto_test.go`:

```go
package textarea

import "testing"

func TestClickToPlainLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("alpha bravo charlie")
	// Click on display row 0, col 8 (inside "bravo", which is runes 6..11).
	m.ClickTo(0, 8)
	if r, c := m.Line(), m.CursorColumn(); r != 0 || c != 8 {
		t.Fatalf("ClickTo(0,8) → row %d col %d, want 0,8", r, c)
	}
}

func TestClickToClampsPastEnd(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("short")
	m.ClickTo(0, 30) // past the end of the line
	if c := m.CursorColumn(); c != 5 {
		t.Fatalf("ClickTo past line end → col %d, want 5 (line length)", c)
	}
}

func TestClickToSecondLine(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("first\nsecond line\nthird")
	m.ClickTo(1, 3) // row 1, col 3 → inside "second"
	if r, c := m.Line(), m.CursorColumn(); r != 1 || c != 3 {
		t.Fatalf("ClickTo(1,3) → row %d col %d, want 1,3", r, c)
	}
}

func TestClickToMultibyte(t *testing.T) {
	m := New()
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("café crème brûlée") // multibyte; runes, not bytes
	m.ClickTo(0, 6) // inside "crème" (rune 5..10)
	if c := m.CursorColumn(); c < 5 || c > 10 {
		t.Fatalf("ClickTo on a multibyte line → col %d, want within [5,10]", c)
	}
}
```

(Uses `CursorColumn()`/`Line()` added in earlier cycles. Note: typewriter defaults off in a bare `New()`, so `top == offset == 0` and display row maps 1:1 to source — these tests pin the non-typewriter path; the implementation must ALSO handle typewriter, exercised via the okashi smoke test in Task 2.)

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test ./internal/textarea/ -run TestClickTo 2>&1 | tail` → undefined `ClickTo`.

- [ ] **Step 3: Implement `ClickTo`** — add to `internal/textarea/textarea.go` (near `SetCursor`):

```go
// ClickTo positions the cursor from a display cell relative to the editor's
// top-left (the same window the View renders). Mirrors View's top math.
func (m *Model) ClickTo(displayRow, displayCol int) {
	if displayCol < 0 {
		displayCol = 0
	}
	if len(m.value) == 0 {
		return
	}
	top := m.offset
	if m.Typewriter && m.height > 0 {
		top = m.cursorLineNumber() - m.height/2
	}
	target := top + displayRow
	if target < 0 {
		target = 0
	}
	if dh := m.displayHeight(); dh > 0 && target >= dh {
		target = dh - 1
	}
	l, wl, _ := m.locateRow(target)
	if l >= len(m.value) {
		l = len(m.value) - 1
	}
	wrapped := m.memoizedWrap(m.value[l], m.width)
	pieceStart, pieceLen := 0, 0
	for i := 0; i < len(wrapped); i++ {
		if i == wl {
			pieceLen = len(wrapped[i])
			break
		}
		pieceStart += len(wrapped[i])
	}
	col := pieceStart + displayCol
	if col > pieceStart+pieceLen {
		col = pieceStart + pieceLen
	}
	if col > len(m.value[l]) {
		col = len(m.value[l])
	}
	m.row = l
	m.col = col
	m.SetCursor(col)
}
```

(If field names differ — `m.value`/`m.row`/`m.col`/`m.offset`/`m.height`/`m.Typewriter` — read the struct and adapt. `locateRow`/`memoizedWrap`/`displayHeight`/`cursorLineNumber`/`SetCursor` all exist.)

- [ ] **Step 4: Run the tests** — `/opt/homebrew/bin/go test ./internal/textarea/ -run TestClickTo -v 2>&1 | tail -12` → PASS; then the FULL `internal/textarea` suite stays green.

- [ ] **Step 5: gofmt; commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/clickto_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add internal/textarea/textarea.go internal/textarea/clickto_test.go
git commit -m "textarea: ClickTo — map an editor display cell to the cursor (for click-to-position)"
```

---

## Task 2: Editor-click positions the cursor (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `m.editor.ClickTo` (Task 1), `m.effectivePanels`, `m.cursorSpellHint`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestEditorClickShowsSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("the teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.sidebarVisible = false
	m.inspector.visible = false
	m.layout()
	// "the teh cat": "teh" is runes 4..7 on row 0. Editor text starts at editorStart + (editorArea-cw)/2.
	_, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	textLeft := (editorArea - cw) / 2 // editorStart 0 (no sidebar)
	nm, _ = m.Update(tea.MouseMsg{X: textLeft + 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	w, _, ok := m.cursorSpellHint()
	if !ok || w != "teh" {
		t.Fatalf("clicking 'teh' should land the cursor in it; hint word=%q ok=%v (col=%d)", w, ok, m.editor.CursorColumn())
	}
}
```

- [ ] **Step 2: Run to verify it fails** — editor clicks currently do nothing → cursor stays at 0 → hint is not "teh".

- [ ] **Step 3: Add the editor-click handler** — in `main.go`'s `MouseMsg` block, after the sidebar/inspector click handling, add a left-click-press case for the editor area:

```go
		// Click in the editor area positions the cursor (enables click-to-suggest).
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			showSidebar, showInspector, editorArea := m.effectivePanels()
			editorStart := 0
			if showSidebar {
				editorStart = sidebarWidth
			}
			inEditor := msg.X >= editorStart && (!showInspector || msg.X < m.width-inspectorWidth) && msg.Y < m.height-1
			if inEditor && !m.previewing {
				cw := min(m.colWidth, editorArea-2)
				textLeft := editorStart + (editorArea-cw)/2
				m.editor.ClickTo(msg.Y, msg.X-textLeft)
				m.focus = focusEditor
				m.editor.Focus()
				return m, nil
			}
		}
```

(Place this so it runs only when the click is NOT in the sidebar/inspector columns. `msg.Y < m.height-1` excludes the status row — handled in Task 3.)

- [ ] **Step 4: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestEditorClick|TestCursorSpellHint|TestMouse' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "editor: click positions the cursor (so clicking a misspelled word shows its suggestions)"
```

---

## Task 3: Clickable suggestions in the bottom bar (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `m.cursorSpellHint`, `m.applySuggestion`, `m.suggestions`/`m.suggestStart` etc.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestClickSuggestionApplies(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // cursor in "teh" → hint active
	w, sugg, ok := m.cursorSpellHint()
	if !ok || len(sugg) == 0 {
		t.Fatalf("precondition: hint for %q should be active", w)
	}
	// The hint renders "✗ teh → the · ...". Click the first suggestion's column.
	// prefix = "✗ "(2) + "teh"(3) + " → "(3) = 8; first suggestion starts at content col 8;
	// status padding adds 1 → screen col 9. Click mid-suggestion.
	idx := strings.Index(ansi.Strip(m.statusBar()), sugg[0])
	if idx < 0 {
		t.Fatalf("suggestion %q not in the rendered hint", sugg[0])
	}
	nm, _ = m.Update(tea.MouseMsg{X: idx + 1 + 1, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if got := m.editor.Value(); got != sugg[0]+" cat" {
		t.Fatalf("clicking suggestion %q should apply it: value=%q", sugg[0], got)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — clicking the status row does nothing yet.

- [ ] **Step 3: Add the hint hit-test + status-row click** — in `main.go`:

```go
// spellHintSuggestionAtX maps a content column on the spell-hint status row to a
// suggestion index. The hint is "✗ " + word + " → " + sugg joined by " · " + "  ·  ^R".
func spellHintSuggestionAtX(word string, sugg []string, localX int) (int, bool) {
	col := lipgloss.Width("✗ "+word+" → ")
	for i, s := range sugg {
		w := lipgloss.Width(s)
		if localX >= col && localX < col+w {
			return i, true
		}
		col += w + lipgloss.Width(" · ")
	}
	return 0, false
}
```

And in the `MouseMsg` block, a left-click on the status row when the hint is active:

```go
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 {
			if w, sugg, ok := m.cursorSpellHint(); ok {
				if i, hit := spellHintSuggestionAtX(w, sugg, msg.X-1); hit { // -1 for status left padding
					m.openSpellMenuAndApply(i, sugg)
					return m, nil
				}
			}
		}
```

The apply path must set up the same state `applySuggestion` expects (the word span under the cursor), then apply. Add a small helper that reuses `wordUnderCursor` + `applySuggestion`:

```go
// openSpellMenuAndApply applies suggestion i for the misspelled word under the cursor.
func (m *model) openSpellMenuAndApply(i int, sugg []string) {
	w, s, e, ok := m.wordUnderCursor()
	if !ok {
		return
	}
	m.suggestions = sugg
	m.suggestWord = w
	m.suggestStart, m.suggestEnd = s, e
	m.applySuggestion(i)
}
```

(Place the status-row click check BEFORE the editor-area click from Task 2 — the status row is `msg.Y == m.height-1`, which Task 2 already excludes, so order is safe either way. Confirm `applySuggestion`/`wordUnderCursor`/the `suggest*` fields exist from the earlier spellcheck cycle.)

- [ ] **Step 4: Run; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestClickSuggestion|TestSuggest|TestCursorSpellHint|TestEditorClick' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "spell: click a suggestion in the bottom bar to apply it"
```

---

## Self-Review

**Spec coverage:** `ClickTo` display→cursor mapping → Task 1; editor-click → cursor + focus (so the hint shows) → Task 2; `spellHintSuggestionAtX` + status-row click → `applySuggestion` → Task 3.

**Placeholder scan:** none — full code. Field-name confirmations (`m.value`/`m.row`/etc.; the `suggest*` fields) are verification steps.

**Type consistency:** `ClickTo(displayRow, displayCol int)` (Task 1) called by the editor-click handler (Task 2); `spellHintSuggestionAtX(word string, sugg []string, localX int) (int, bool)` + `openSpellMenuAndApply(i int, sugg []string)` (Task 3) reuse `cursorSpellHint`/`wordUnderCursor`/`applySuggestion`/`suggest*` from the spellcheck cycle.

**Risk:** Task 1 is editor-core — `ClickTo` must mirror `View`'s `top` math exactly (typewriter vs offset, leading-blank via clamp) and clamp the column to the wrapped piece + line; the unit tests pin the non-typewriter path and the Task-2 smoke test exercises the real (typewriter-on) editor. The editor-click handler must fire ONLY in the editor column (not sidebar/inspector/status) — the `inEditor` bounds + `msg.Y < m.height-1` guard that; the controller will empirically verify a click on a known misspelled word lands the cursor in it and the suggestion click corrects it.
```
