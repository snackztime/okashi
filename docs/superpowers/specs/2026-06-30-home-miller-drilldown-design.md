# Home-screen Miller launcher (three framed columns) — design

**Date:** 2026-06-30
**Status:** Approved
**Context:** Give file-selection granularity (open a specific chapter/file from a project,
Ulysses/iA-style) **on the launch screen** (`ctrl+o`), not in the writing view — keeping the
editor uncluttered. Replaces the rejected in-editor "preview snippet sidebar" idea. Extends
the two-column home redesign (`home.go`).

## Layout

Centered logo, three **framed columns** (using the editor's `framedPanel`, titles in the
border), then the centered actions. The three boxes are centered **as a group**; the logo and
actions are centered over the same total width. The whole block is `lipgloss.Place`d centered.

```
                          o k a s h i
                          ───────────

  ╭ RECENT ──────╮ ╭ LIBRARY ─────╮ ╭ FILES ─────────────╮
  │ ch3.md       │ │ ★ my-novel   │ │ 01-opening    1,204│
  │ notes.md     │ │ › stories    │ │   The morning fog… │
  │ draft.md     │ │ › poems      │ │ 02-arrival      892│
  │ a-day.md     │ │ FOLDERS      │ │   He stepped off…  │
  │ scene2.md    │ │ › research   │ │ notes.md        340│
  ╰──────────────╯ ╰──────────────╯ ╰────────────────────╯

          + New document     + New project     Browse all files
```

- **RECENT** (left): recent documents, always visible — quick resume. Default focus on entry.
- **LIBRARY** (middle): a `PROJECTS` section (manuscript folders) then a `FOLDERS` section
  (non-manuscript/"category" folders). The **selected** library item drives the Files column
  (independent of which column has focus — standard Miller behavior).
- **FILES** (right): the selected library item's documents, each on **two lines** — `name …
  word-count`, then a **dimmed 1-line snippet** of the opening prose.
- **Logo** unboxed/centered above; **actions** unboxed/centered below.

Visual consistency with the editor: the three boxes use the same `framedPanel` (rounded
border, dim title segment) as the writing-view sidebar/inspector. The logo and actions stay
clean (no box).

## Columns

### RECENT
Recent files (most-recent-first), one per line (name only, optional dim word-count). `enter`
or double-click opens the file in the editor. Empty → a dim `(no recent files)` placeholder.

### LIBRARY
Top-level subdirs of the workspace, classified once per build (cheap; a handful of dirs):
- **PROJECTS** — manuscripts (a `manifest.json` OR ≥1 numerically-prefixed file — reuse
  okashi's manuscript detection / `resolveManuscript`). Alpha-sorted.
- **FOLDERS** — non-manuscript "category" folders. Alpha-sorted. Header omitted when none.

Section headers (`PROJECTS`/`FOLDERS`) are dim, non-selectable. The library always has a
selected item (default: the first project, else first folder) so FILES is always populated.
`enter` on a library item **opens the container** in the editor (sidebar at that dir — current
project-open behavior). `right` moves focus into FILES.

### FILES
The selected library item's documents, recomputed whenever the library selection changes,
into `m.homeFiles []homeFileItem`:

```go
type homeFileItem struct {
    name    string // chapter title for manuscript chapters, else filename
    path    string
    words   int    // shared wordCountCache
    snippet string // 1-line opening prose (snippet cache)
}
```
- **Project** → `resolveManuscript` order: chapters (view order/titles) then loose `.md`.
- **Folder** → its `allowedDocExts` files, name-sorted.
- Each renders as two lines: `name` + right-aligned word-count, then a dim snippet line.
- `enter` or double-click opens the file. Empty → dim `(empty)` placeholder. `left` → LIBRARY.

## Snippet cache (`snippet.go`)

Mirrors `wordCountCache`: maps path → (mtime, snippet). `snippet(path)`:
1. Read the **first ~400 bytes only** (never the whole file).
2. Strip a leading YAML frontmatter block (`---`…`---`), ATX headings (`#…` lines),
   blockquote/list markers (`>`,`-`,`*`,`+`,`N.`), inline emphasis/code/link syntax
   (`*_`` ``[]()`), and collapse whitespace/newlines to single spaces.
3. Return the first non-empty prose, trimmed to ~80 runes (the renderer truncates with `…`).

Invalidate on `os.Stat` mtime change. Lazy: only computed for files in the rendered FILES
window, so opening home never reads more than the visible slice.

## Navigation

Selection is `(region, index)` + `homeLastCol`, extending the current model. Regions:
`regionRecent`, `regionLibrary`, `regionFiles`, `regionActions`. The library selection is
held separately (it drives FILES even when focus is elsewhere).

- **left/right** move focus across the columns: RECENT ↔ LIBRARY ↔ FILES (clamping index to
  the target column's length); from any column, continuing past the edge stays put.
- **up/down** move within the focused column (skipping section headers in LIBRARY). In
  LIBRARY, moving the selection **recomputes FILES**. `down` past a column's bottom → ACTIONS;
  `up` from ACTIONS → the last column.
- **enter**: RECENT/FILES → open the file in the editor; LIBRARY → open the container
  (sidebar); ACTIONS → the action.
- **tab/shift+tab** cycle the non-empty regions.
- **Mouse**: click selects (and, in LIBRARY, repopulates FILES); a second click / click on the
  already-selected file opens it; wheel scrolls the focused column. Header lines aren't
  clickable.

## Rendering & hit-test (`home.go`)

- Build each column's inner content (lines), wrap it in `framedPanel(title, inner, colW,
  colH, "")`, and `lipgloss.JoinHorizontal` the three with a fixed gap. Column heights equal
  (the tallest column; shorter ones pad). `colH` is bounded to the screen; long lists scroll
  within the box (windowed per column, like the file pane).
- Extend `homeContent()` to return the assembled block lines + `homeCell`s in **block-relative
  coords**, accounting for each box's border offset (content starts at +1,+1 inside its
  frame). A two-line FILES row emits **two cells** mapping to the same `(regionFiles, index)`
  so a click on either line hits the file. Section headers/placeholders emit no cell.
- `homeView()` centers the block with `lipgloss.Place`; `homeItemAt` reverses the same offset
  (unchanged in spirit — it scans cells). No writing-view / `sidebarWidth` changes.

## Responsive fallback

Compute the three-box total width; if it exceeds the screen, degrade in order: (1) drop RECENT
(keep the LIBRARY→FILES selector); (2) if still too wide, show a single column (LIBRARY, then
FILES on drill via `right`). Always keep the actions reachable. (The home screen has the full
width — three ~22–32-col boxes fit comfortably at ≥90 cols.)

## Model changes

- Library items: `[]homeItem` tagged `homeProject`/`homeFolder`; `buildHomeItems` classifies
  subdirs. Recent items stay `homeRecentFile`. Actions unchanged.
- Add `m.homeFiles []homeFileItem` + `recomputeHomeFiles()` (from the selected library item),
  called on library-selection change and on `ctrl+o`/startup; a `librarySelected int` index.
- Add a `snippetCache` to the model (alongside `m.files.wc`).
- `openHomeSelection` gains the RECENT-file and FILES cases (open by path).

## Edge cases

- No projects / no folders / no recents → those sections/columns omit or show a placeholder;
  default focus lands on the first present, populated region; actions always reachable.
- A project that fails to resolve → treated as a folder (flat list) with a status note.
- Library item with no documents → FILES shows the `(empty)` placeholder; `right`/`enter` into
  it is a no-op.

## Out of scope

- In-editor sidebar snippets (explicitly dropped).
- More than two library levels (project → subfolder → files); folders are flat here.
- Editing/reordering from home (it stays a launcher).
- Multi-line snippets (one line only).

## Testing

- `snippet`: frontmatter/heading/list/emphasis stripping; whitespace collapse; ~80-rune cap;
  mtime invalidation; reads only the first ~400 bytes (a huge file isn't fully read).
- library classification (manuscript vs folder); `recomputeHomeFiles` for project/folder/empty;
  recent listing.
- nav: moving the library selection repopulates FILES; left/right cross the three columns;
  `enter` opens file vs opens container; actions reachable; empty-state landings.
- render == hit-test: every cell — including both lines of a two-line FILES row and across the
  three framed boxes — round-trips through `homeItemAt`; headers/placeholders aren't clickable.
- framed layout: the three boxes are centered as a group, equal height; the logo and actions
  center over the same width; responsive drop of RECENT when too narrow.
