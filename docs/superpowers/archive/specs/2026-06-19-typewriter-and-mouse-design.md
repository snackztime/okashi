# Typewriter scrolling + mouse-driven file pane — design

**Date:** 2026-06-19
**Status:** Approved (pending spec review)
**Roadmap:** Implements #1 (typewriter scroll) and #4 (flat file list); lays groundwork for #2 (focus dimming).

## Goal

Two related improvements to okashi's writing experience:

1. **Typewriter scrolling** — pin the current line to the vertical center of the
   writing pane (WordGrinder-style), toggleable with `ctrl+t`.
2. **Mouse on the file pane** — replace the stock `bubbles/filepicker` with a
   small file-list component we own, so the mouse can scroll, select, and open
   files.

## Background / constraints discovered

- `bubbles/textarea` keeps the caret visible by edge-anchoring its **private**
  internal viewport (`repositionView` → `cursorLineNumber()`). The viewport
  offset and the absolute wrapped cursor row are not exposed, and re-deriving
  the wrap from outside is fragile. → True centered scrolling requires owning
  that reposition logic.
- `bubbles/filepicker` has **no** mouse handling (only `KeyMsg`/`WindowSizeMsg`)
  and a private selection index + scroll offset. → Clean click-to-select can't
  be bolted on from outside.

---

## Section 1 — Typewriter scrolling

### Approach: vendor `textarea` + minimal additive patch

Copy the `bubbles/textarea` package into `internal/textarea/` (pinned to the
currently-used v0.20.0), with a header comment recording the upstream version
and the exact local change. Editing behavior (undo, soft-wrap, unicode,
clipboard) is preserved verbatim; only scroll positioning changes.

**The patch (additive):**

- Add an exported field `Typewriter bool`.
- When `Typewriter` is true, the content fed to the internal viewport is padded
  with `Height/2` blank rows **above and below** the document, and the viewport
  offset is set so the caret's absolute wrapped row lands on screen-center:

  ```
  content  = [H/2 blank rows] + docRows + [H/2 blank rows]
  YOffset  = cursorRow                       // clamped to [0, len(content)-H]
  ```

  With symmetric padding, `YOffset = cursorRow` centers **every** line,
  including the first (blank space above) and last (blank space below) — the
  true always-centered feel while writing at EOF. When `Typewriter` is false,
  the original edge-anchored behavior is used unchanged.

- Add exported helpers:
  - `SetTypewriter(bool)` — sets the flag and repositions immediately (so the
    toggle takes effect without waiting for the next keystroke).
  - `ViewportYOffset() int` — read the current offset (for tests).

### okashi integration

- `model` gains `typewriter bool` (default **on** — it's the signature feature).
- `ctrl+t` toggles it: calls `editor.SetTypewriter(m.typewriter)` and updates the
  status hint (`… · ctrl+t typewriter …`).
- `applyTypewriterScroll()` (currently a no-op) is removed; the vendored
  textarea now owns this.

### Testing

- In `internal/textarea`, a focused test: set a multi-line value, enable
  `Typewriter`, move the caret to row k, assert `ViewportYOffset()` equals the
  expected centered offset at top (clamped to 0), middle (= k), and EOF.
- Pure-arithmetic, no terminal needed.

---

## Section 2 — Custom `filelist` component (replaces filepicker)

### Component

A small component we fully own, in the `main` package as `filelist.go`
(app-specific, no need for a separate module path):

```go
type fileEntry struct { name string; isDir bool }

type filelist struct {
    dir       string
    entries   []fileEntry   // ".." first (unless at root), dirs, then files
    selected  int
    offset    int           // scroll window top
    width, height int
}
```

- `SetDir(path)` — `os.ReadDir`, filter (dirs always; files matching the same
  allowed extensions as today: `.md .txt .wg .markdown`), sort dirs-first then
  alphabetical, prepend `".."` when not at filesystem root. Resets `selected`/
  `offset`.
- Keyboard `Update`: up/down move `selected` (clamped); enter opens
  (file → signal parent; dir/`..` → `SetDir`); left/backspace → up a dir.
- Mouse helpers (driven by the parent, which owns absolute coordinates):
  - `ScrollBy(n)` — move selection by n (wheel).
  - `SelectRow(visibleRow)` — set `selected = offset + visibleRow` (clamped).
  - `Activate()` — open the selected entry (double-click / enter).
- `View()` renders `entries[offset : offset+height]`, highlighting `selected`,
  keeping the selection inside the scroll window.

### Mouse interactions (per approved choices)

- **Wheel** over the pane → `ScrollBy(±1)` and focus the pane.
- **Single left-click** → focus the pane + `SelectRow(rowUnderCursor)`.
- **Double-click** (same row within 400ms) → `Activate()` (open file / enter dir).
- Keyboard navigation unchanged.

### Coordinate mapping (parent)

The parent translates a `tea.MouseMsg`'s absolute Y into a list row:

```
visibleRow = mouse.Y - bannerHeight        // sidebar content starts just below the banner
```

(bannerHeight = `lipgloss.Height(bannerView)`; the sidebar style adds no top
border/padding, so no extra offset. Calibrated against the real render during
implementation.) Clicks/wheels with `mouse.X >= sidebarWidth`, or when the
sidebar is hidden, are ignored by the pane. Double-click timing uses
`time.Now()` with `lastClickRow`/`lastClickTime` on the model.

### okashi integration

- Replace the `filepicker` field with `filelist`; drop the `bubbles/filepicker`
  dependency (`go mod tidy`).
- `initialModel` → `fl.SetDir(writingDir())`.
- `Update` routing: when focus is the sidebar, route keys to `filelist`; on its
  "open file" signal, `loadFile` + focus editor (same as today). Add a
  `tea.MouseMsg` branch implementing the interactions above.
- `View`/`layout`: render `fl.View()` in the sidebar; size `fl` from `bodyH`.

### Testing

- `filelist` over a temp dir (mix of dirs, matching files, non-matching files):
  read/filter/sort correctness, `".."` presence/absence at root.
- Selection clamping, scroll-window math, `SelectRow` mapping.
- Double-click → open behavior at the model level (two `SelectRow`+activate
  within the window opens; outside the window does not).
- Coordinate mapping as a pure helper: Y → visibleRow.

---

## Out of scope (non-goals)

- Mouse in the **editor** pane (caret-by-click) — not requested.
- Focus dimming (#2) and rope-buffer hardening (#5) — separate roadmap items.
- Drag-select, mouse pane-resizing, multi-select in the file pane.

## Risks

- **Vendored textarea drift:** mitigated by a version/patch header comment; we
  re-sync deliberately on future Charm bumps.
- **Mouse Y calibration:** the banner-height offset must be verified in a real
  terminal; isolated in one pure helper so it's easy to adjust.
- **EOF/BOF padding interaction with very small terminals** (Height < 2): clamp
  padding so a 1–2 row pane degrades gracefully to non-centered.
