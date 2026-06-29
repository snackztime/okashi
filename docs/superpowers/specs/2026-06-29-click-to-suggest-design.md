# Click-to-suggest spellcheck — design

**Date:** 2026-06-29
**Status:** Approved (bottom clickable bar, click-the-word trigger)
**Context:** Spellcheck already underlines misspellings and (cursor-over) shows suggestions in the
status bar. Add the mouse path: **click a misspelled word → its suggestions appear in the bottom
bar → click a suggestion to apply.** No floating popup (terminal overlay compositing is the
fragile part we're avoiding); the suggestion bar is a fixed, reliably-clickable location.

## Reuse what exists

`cursorSpellHint()` already returns `(word, suggestions, ok)` and the status bar renders
`✗ teh → the · ten · tea · ^R` whenever the cursor is on a misspelled word. So the mouse flow is:
1. Click a word in the editor → move the cursor there → if misspelled, the bar shows its suggestions
   (existing behavior, once the cursor lands).
2. Click a suggestion in that bar → `applySuggestion(i)` (existing) replaces the word, case-preserved.

The two new pieces are (a) **editor-click → cursor position** and (b) **clickable suggestions**.

## 1. Editor click → cursor (`internal/textarea`)

Add `(*Model).ClickTo(displayRow, displayCol int)` — map a position relative to the editor's
top-left to a buffer cursor, mirroring `View`'s window math:
- Compute `top` the same way `View` does (the tracked `offset`, or for focused typewriter
  `cursorLineNumber() - height/2`); `target := top + displayRow`, clamped to `[0, displayHeight())`.
- `l, wl, lineOffset := m.locateRow(target)` → source line `l`, wrapped-piece index, and the source
  rune offset where that piece starts.
- The source column = `lineOffset + displayCol`, clamped to the wrapped piece's width (so a click
  past the end of a soft-wrapped row lands at the piece end, not the next source column) and to
  `len(m.value[l])`.
- Set `m.row = l`, `m.col = col`, then `SetCursor(col)` (re-clamp + refresh). Handle the leading
  blank rows when `top < 0` (typewriter near the top): a click in those rows maps to row 0.

Unit-tested directly (no whole-buffer scan): a click on a known word's display cell lands the
cursor inside that word, including a soft-wrapped line and a multibyte line.

## 2. Editor-click handler (`main.go`)

In the `MouseMsg` block, when a left-click press lands in the **editor area** (not the sidebar or
inspector columns) on the writing screen:
- Compute the editor's screen origin: `editorStart` (sidebar width or 0), text left =
  `editorStart + (editorArea - cw)/2`, top = 0 (editor is top-aligned). `displayRow := msg.Y`,
  `displayCol := msg.X - textLeft` (ignore clicks left of the text).
- `m.editor.ClickTo(displayRow, displayCol)`; `m.focus = focusEditor`; `m.editor.Focus()`.
- After this, `cursorSpellHint()` naturally shows suggestions if the clicked word is misspelled.

## 3. Clickable suggestions in the bottom bar (`main.go`)

- A helper `spellHintHitTest(localX int) (suggestionIndex int, ok bool)` reconstructs the rendered
  hint layout (`"✗ " + word + " → " + suggestions joined by " · " + "  ·  ^R"`) and returns which
  suggestion column-range `localX` falls in. (Same suggestions as `cursorSpellHint`, so the layout
  is reproducible.)
- In the `MouseMsg` block, a left-click on the **status row** (`msg.Y == m.height-1`) while
  `cursorSpellHint()` is active: map `msg.X` to a content column (minus the 1-col status padding),
  hit-test via `spellHintHitTest`; on a hit, `applySuggestion(thatIndex)`.
- `ctrl+r` and the cursor-over hint are unchanged; this only adds the click affordance.

## Testing

- `ClickTo`: clicking a cell inside a word positions the cursor in that word (plain, soft-wrapped,
  multibyte lines); clicking past line end clamps to the line end; clicking below content clamps.
- Editor click: a `MouseMsg` over a misspelled word moves the cursor into it and
  `cursorSpellHint()` then returns that word's suggestions (render-based: the status row shows the
  hint after the click).
- Suggestion click: with the hint showing, a click on the column of "the" applies "the" (buffer
  becomes corrected, case-preserved); a click off any suggestion does nothing.
- Existing spellcheck/cursor-hint/ctrl+r tests stay green.

## Out of scope

- Hover (motion) triggering — click-to-target only.
- A floating popup overlay; multi-line click selection; drag.
- Mapping clicks inside the side panels to spelling (panels keep their own click behavior).
