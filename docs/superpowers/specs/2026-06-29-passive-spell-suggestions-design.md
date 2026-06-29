# Passive cursor-over spelling suggestions — design

**Date:** 2026-06-29
**Status:** Approved
**Context:** Spellcheck underlines already update live as you edit (the per-line decorator re-runs
every render; measured ~153 µs for a 40-line screen — negligible). This adds passive surfacing:
when the cursor sits on a misspelled word, the status bar shows the suggestions automatically,
so you don't have to press `ctrl+r` to see them. Mouse hover is explicitly **out of scope**
(too coordinate-fiddly and un-terminal-like — keyboard/cursor only).

## Performance constraint (must honor)

The status bar renders **every frame**. The current `wordUnderCursor()` calls `editor.Value()` +
`strings.Split` — O(buffer) — which is fine for a one-shot `ctrl+r` keypress but would
re-stringify a whole chapter every frame if used passively. So:
- Add `(*Model).CurrentLine() string` to the vendored editor — returns `string(m.value[m.row])`,
  O(line). Rewrite `wordUnderCursor()` to use `CurrentLine()` + `CursorColumn()` (no whole-buffer
  stringify). This also speeds up the existing `ctrl+r` path.
- Memoize `spellSuggest(word, limit)` (a `map[string][]string` keyed by `word|limit`, mutex-guarded,
  soft-capped at 4096 like `posCache`) so the per-frame suggestion lookup is a cache hit. gospell's
  `Suggest` is heavier than `Spell`; without the cache the passive bar would recompute every frame.

## Behavior

- A helper `cursorSpellHint() (word string, suggestions []string, ok bool)`:
  - returns `ok=false` unless `m.analysis.spell` is on and the screen is the writing screen and no
    modal is active (`!m.renaming && m.goalPromptField==0 && !m.suggesting && !m.previewing &&
    !m.exportPrompt`);
  - `w, _, _, ok := m.wordUnderCursor()`; if `!ok` or `spellOK(w)` → `ok=false`;
  - else `suggestions := spellSuggest(w, 4)`; if empty → `ok=false`; else return `w, suggestions, true`.
- **Status-bar render order** (in the status-line function): `m.suggesting` interactive menu
  (existing) → else `cursorSpellHint()` passive line (new) → else the normal status. The passive
  line: `"✗ " + word + " → " + strings.Join(suggestions, " · ") + "  ·  ^R"`, truncated to width.
- `ctrl+r` is unchanged (opens the interactive pick-menu). The passive bar is informational only;
  arrow keys still move the editor cursor normally (no mode change).

## Out of scope

- Mouse hover / motion tracking (dropped by decision).
- Suppressing the hint while actively typing a partial word (known minor flicker; revisit only if
  it's annoying in practice).
- Inline/popup rendering at the word; auto-correct.

## Testing

- `CurrentLine()` returns the cursor's line; `wordUnderCursor` still returns the right token after
  the rewrite (existing suggest tests stay green).
- `spellSuggest` memoized: second call for the same word populates/serves a cache (`len(cache)>0`),
  results identical.
- `cursorSpellHint`: cursor on a misspelled word with spell ON → returns the word + suggestions;
  spell OFF → not ok; cursor on a correct word → not ok; inside a modal (`m.suggesting`/renaming) →
  not ok.
- Status line: with spell on and cursor on `teh`, the rendered status contains `✗ teh` and `the`;
  moving the cursor onto a correct word reverts to the normal status; the interactive `m.suggesting`
  menu still takes precedence.
