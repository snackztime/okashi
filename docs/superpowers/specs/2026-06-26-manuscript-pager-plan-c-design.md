# Plan C — Manuscript pager: design (finalized)

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Parent spec:** `docs/superpowers/specs/2026-06-22-long-form-projects-design.md` (§5, §8).
This document finalizes the candidate decisions §5 deferred. Where this doc and the
parent differ, **this doc governs for Plan C**.

## Goal

A full-screen, read-through **manuscript pager**: concatenate a manuscript's ordered
sections into one scrollable view (raw prose with chapter dividers and dimmed markdown
syntax), show where you are by word count, and **jump from any line into the editor at
that exact line**. Built on the Plan A data layer and reached from the Plan B outline.

## Scope

**In Plan C:** the `screenManuscript` screen; build (concatenate ordered sections +
line→source map, once); manual scroll + a pager cursor; dimmed-markdown raw-prose render;
a running "words-to-cursor / total" header; jump-to-edit (open the section in the editor
at the mapped line); mouse (click moves cursor, double-click jumps); the outline `m` ↔
pager `o` wiring (parent spec §8); one vendored-textarea addition (`MoveToLine`).

**Deferred (NOT Plan C):** export (§6 → Plan D); scene-break styling; any markdown
*rendering* (glamour); an editable continuous buffer (rejected by the parent spec).

## Architecture

A self-contained `pager.go` holding a `pagerModel`, mirroring `outlineModel`: a **manual
scroller** (not the bubbles `viewport`) so it owns both the cursor line and the exact
line→source map. `main.go` adds `screenManuscript` to the `screen` enum and delegates
key/mouse/`View` to the pager when active. The pager is built **once** on open; scrolling
and cursor movement never rebuild it or re-read files.

`pagerModel` owns:
- `dir string` — the manuscript folder.
- `lines []pagerLine` — the built, wrapped, mapped content (below).
- `total int` — total manuscript word count.
- `cursor int` — index into `lines` (the highlighted line).
- `offset int` — index of the top visible line.
- `width, height int` — the measure width (matches the editor's `colWidth`) and the
  visible body height.

```go
type pagerLine struct {
    text     string // one display row, already wrapped to the measure width
    file     string // source section file ("" for a chapter-rule header line)
    src      int    // 0-based source line within file (-1 for a header line)
    header   bool   // a "── Title ──" chapter rule
    cumWords int    // running word count from the manuscript start through this line
}
```

## Section 1 — Build (once, on open)

`func (p *pagerModel) load(dir string, width int)`:
- Read the dir, `orderedSections` (loose excluded). For each section in order:
  - Emit one **header** line `── <sectionTitle> ──` (`header:true`, `file:""`, `src:-1`).
  - Read the file; for each 0-based source line, **word-wrap it to `width`** and emit one
    `pagerLine` per wrapped row, all carrying the same `(file, src)`. A blank source line
    emits one empty `pagerLine`.
- Maintain a running word counter: each emitted line's `cumWords` is the cumulative word
  count of all body text from the manuscript start through that line (header lines add 0;
  reuse the existing `wordCount` on the source line's text).
- Set `total` to the final cumulative count, `cursor = 0`, `offset = 0`.

Wrapping to `width` (not relying on the renderer to wrap) is what keeps the line→source
map exact: every `lines[i]` is already ≤ `width`, so rendering never reflows and the
display-line index *is* the map index.

**Testing:** building `[01-a.md, 02-b.md]` yields a header line then the wrapped body of
each, in order; a long source line wraps to multiple `pagerLine`s that all map back to the
same `(file, src)`; `cumWords` is monotonic non-decreasing and ends at `total`; loose files
never appear; each file is read **exactly once** during build.

## Section 2 — Render (cost O(visible height))

`func (p pagerModel) View() string`:
- **Header line:** `<projectTitle(dir)> · <commafy(lines[cursor].cumWords)> / <commafy(total)>w`.
- **Body:** render only `lines[offset : offset+height]`. For each row:
  - a `header` line → styled as a chapter rule (`accent`).
  - the `cursor` line → `selectedStyle`.
  - otherwise → the prose with **markdown syntax dimmed**: leading `#` heading marks and
    inline `*`/`_`/`` ` `` emphasis marks colored `subtle`. Dimming is colour-only — it
    never changes a line's length or wrapping, so the map stays exact.
- A `pagerHeaderHeight` const (the header rows) is the single offset shared by the render
  and the mouse hit-test.

**Testing:** the header shows the running/total counts; the cursor row is highlighted;
rendering a window never reads files; markdown marks are dimmed without altering the line
text length.

## Section 3 — Navigation & jump-to-edit

- `↑`/`↓`, `j`/`k`: move `cursor` by ±1 (clamped); `offset` follows to keep the cursor in
  the visible window.
- `pgup`/`pgdn`: move `cursor` by ±`height` (clamped); wheel scrolls likewise.
- **Single-click:** set `cursor` to the hit-tested line (`offset + (mouseY - pagerHeaderHeight)`).
- **Enter / double-click — jump-to-edit:** for `lines[cursor]`, resolve `(file, src)`. For a
  **header** line, use the section file's first body line (`src = 0`). `loadFile(file)`,
  then place the editor cursor on line `src` (Section 4), switch to `screenWriting`, focus
  the editor. If the file fails to load, stay with a status message.
- `o`: back to the outline (`screenOutline`, reload it). `esc`: back to the editor.

**Testing:** `↑↓`/page math keeps the cursor within `[0, len(lines)-1]` and the cursor
visible; the line→source map resolves a known manuscript line back to the right `(file,
line)`; Enter on it sets `m.currentFile` to that section and the editor cursor to the
mapped line; Enter on a header opens that section at line 0.

## Section 4 — Editor line positioning (vendored textarea)

Jump-to-edit must place the editor cursor on a specific source line. The vendored
`internal/textarea` exposes `Line()`, `CursorUp`, `CursorDown`, `SetCursor(col)` — but no
"go to line N". Add:

```go
// MoveToLine moves the cursor to the start of line n (clamped to the buffer).
func (m *Model) MoveToLine(n int)
```

The okashi model calls it after `loadFile` to land on the mapped line. (Implementation:
set the cursor row to `n` clamped to `[0, lineCount-1]`, column 0, mirroring the existing
`CursorDown`/`SetCursor` internals.)

**Testing:** after `SetValue` of a 5-line buffer, `MoveToLine(3)` makes `Line() == 3`;
`MoveToLine(99)` clamps to the last line; `MoveToLine(-1)` clamps to 0.

## Section 5 — Wiring & keys (parent spec §8)

- New `screenManuscript`; `model.pager pagerModel`; dispatch in `Update`/`View` when active.
- The outline's `m` (today: `m.status = "manuscript view — Plan C"`) instead builds the
  pager (`m.pager.load(m.outline.dir, m.colWidth)`), sets `screenManuscript`, and seeds a
  status hint (`↑↓ scroll · enter edit here · o outline · esc editor`).
- From the pager: `o` → outline, `esc` → editor, `enter`/double-click → jump-to-edit.
- No direct editor→pager key: `ctrl+m` is Enter in terminals and plain `m` is editor text;
  the route is `ctrl+l` (outline) → `m` (pager). The pager, like the outline, is only
  reachable inside a manuscript.

## Section 6 — Performance (acceptance criteria — the load-bearing requirement)

The pager is the one screen that holds the whole manuscript, so these are explicit,
tested requirements:
- **Build once:** `load` reads each section file exactly once; scrolling and cursor
  movement perform **no** file I/O and do **not** rebuild `lines`.
- **No per-frame rendering work proportional to length:** `View` renders only
  `lines[offset:offset+height]`; cost is O(visible height), independent of `len(lines)`.
- **No glamour:** the body is raw prose with colour-only dimming; nothing reflows.

**Testing (synthetic scale):** build a manuscript of ~300 sections (or a few thousand
total lines) under `t.TempDir()`; assert the build completes, `total`/`cumWords` are
consistent, and a sequence of cursor/scroll moves afterward reads no files (e.g. via a
read-counting wrapper or by asserting behaviour is identical after the source files are
removed post-build).

## Risks

- **Line-map drift** — the map is exact only because the build pre-wraps to the measure
  width and the renderer never re-wraps; the dimming pass is colour-only. Both are tested.
- **Hit-test drift** — render and mouse share `pagerHeaderHeight` and the same `offset`.
- **Editor cursor positioning** — the single vendored change (`MoveToLine`) is unit-tested
  in isolation before it's wired.
- **Tests stay hermetic** — `t.TempDir()` / `t.Setenv("OKASHI_DIR", …)`; the pure build
  takes the width in.

## Build order

A single plan:
- Vendored `MoveToLine` + test.
- `pagerModel` build (`load`, `pagerLine`, wrap, map, cumWords) + tests.
- Render (`View`, dimmed syntax, running header, `pagerHeaderHeight`) + tests.
- Wiring (`screenManuscript`, outline `m` → pager, nav keys, jump-to-edit, `o`/`esc`) + tests.
- Mouse (click cursor, double-click jump) + tests.
- Docs/status.
