# Search (project-wide + in-document) — design

**Date:** 2026-06-30
**Status:** Approved
**Context:** The clearest long-form gap — okashi has no find anywhere. A 400-page work split
into chapter files needs "where did I mention the lighthouse?" across files, plus find within
the current document. One unified search surface with a **scope toggle** covers both.

## Surface

A dedicated **`screenSearch`** (sibling of `screenOutline`/`screenManuscript`), opened with
**`ctrl+f`** from the writing screen. Layout: a query input row, a scope indicator, then a
scrollable results list; a footer hint.

```
  Search ▸ lighthouse                          Project ⇄ This document   (Tab)
  ─────────────────────────────────────────────────────────────────────
  03-arrival.md : 12   …saw the lighthouse blink twice across…
  03-arrival.md : 48   …the lighthouse keeper had warned them…
  07-storm.md   : 5    …no lighthouse could pierce that fog…
  ─────────────────────────────────────────────────────────────────────
  18 matches in 6 files · ↑↓ select · ⏎ open · Tab scope · esc back
```

## Behavior

- **Query input** (`bubbles/textinput`): case-insensitive **substring** match (lean — no
  regex/whole-word in v1). Results recompute on a short debounce after typing (or on Enter).
- **Scope toggle (`Tab`)**: `Project` (default) searches all document files under the
  workspace root recursively; `This document` searches only the open file. The indicator
  shows the active scope.
- **Results:** each match = `display-name : line` + the matching line, head/tail-trimmed
  around the match (the match itself style-highlighted), ansi-truncated to width. Grouped/
  sorted by file then line. Capped at **200 matches** (footer notes truncation if hit).
- **Navigate:** `up/down` (+ wheel) move the selection, windowed to the visible height
  (line→result map like the pager). `enter` opens the result's file (if not already open) and
  **moves the cursor to that line** (`editor.MoveToLine` + column of the match), entering the
  writing screen focused on the editor. `esc` returns to writing without moving.
- **In-document highlight:** after jumping (either scope), the matched range on the cursor
  line is briefly emphasized — reuse the `textarea.Decorator` with a transient
  `searchHitStyle` for the current query's matches on the visible lines, cleared on the next
  edit or on a new search. (This gives the "find" feel; classic n/N cycling is a later
  enhancement — re-opening `ctrl+f` keeps the last query.)

## Engine (`search.go`)

```go
type searchHit struct{ file, name string; line, col int; context string }
func searchProject(root string, allowed map[string]bool, query string, limit int) []searchHit
func searchText(name, path, text, query string, limit int) []searchHit // one document
```
- `searchProject` walks `root` (filepath.WalkDir), skipping hidden dirs/files and non-`allowed`
  extensions, reading each file once, scanning lines for the case-folded substring, emitting
  hits with the chapter/display name (via the dir's `resolveManuscript` title when available,
  else filename) until `limit`.
- On-demand only (a key press), never per-frame. Reading a project's files on search is fine
  (it's what export already does); large binaries are excluded by the extension filter.

## Model / wiring (`main.go`)

- `screenSearch` constant; `searchQuery` input; `searchScope` (project|document); `searchHits
  []searchHit`; `searchSel`, `searchOffset`.
- `ctrl+f` (writing screen) → enter `screenSearch`, focus the query input, seed with the
  current selection/word if any.
- `updateSearch` handles typing (debounced recompute), Tab (scope), up/down/wheel, enter
  (jump), esc (back). `searchView` renders the input + windowed results.
- Jump: `m.loadFile(hit.file path)` if different, `m.editor.MoveToLine(hit.line)`, set the
  cursor column, `m.screen = screenWriting`, focus editor; set the transient search decorator.

## Edge cases

- Empty query → no results, neutral hint. No matches → "(no matches)".
- Query matching a huge number → capped at 200 with a "+ more" note.
- A file deleted between listing and read → skipped.
- `This document` scope with no open file → falls back to Project (or a note).
- Multibyte: match column is a RUNE column (for `MoveToLine`/cursor + the highlight).

## Out of scope (later)

- Replace (search-and-replace, esp. across files) — risky, deferred; this is navigate-only.
- Regex / whole-word / case-sensitive toggles.
- n/N in-editor cycling without the search screen.

## Testing

- `searchText`: substring case-insensitivity; rune column on multibyte lines; multiple hits
  per line; the context trim around the match; limit cap.
- `searchProject`: walks subdirs, excludes hidden + non-doc files, uses chapter titles, stops
  at limit; a deleted file mid-walk is skipped.
- wiring: `ctrl+f` enters the screen; Tab flips scope + recomputes; enter jumps to the right
  file+line (cursor on the match) and returns to writing; esc cancels; the search decorator
  highlights the query on the landing line and clears on edit.
