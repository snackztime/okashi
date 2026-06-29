# Windowed editor render — design

**Date:** 2026-06-28
**Status:** Approved (pending spec review)
**Context:** Editing a single very large file lags to unusable. Benchmarks: editor `View()`
costs ~1.5 ms for a chapter (~10 pages) but **~51 ms for a 400-page single file** — linear in
buffer size. CPU profile shows the cost is **per-line lipgloss styling** (`Style.Render` +
`isSet`/`getAsInt`/`Inherit`/`getAsColor` ≈ 31%+), uniseg width calc, and `[]rune→string`,
all multiplied across **every line in the buffer** because `internal/textarea`'s `View()`
iterates all of `m.value` each frame. Wrapping (`ansi.Wrap`) is *not* hot.

This is the "single huge file" case `CLAUDE.md` flagged. The fix is **windowing the render**,
not a gap buffer (a gap buffer speeds edits — not the bottleneck).

## Goal

Make `internal/textarea` `View()` cost **O(visible height)**, independent of buffer size, so a
400-page single file renders at roughly the chapter cost (~1.5 ms) and stays flat as the
buffer grows. **The buffer model (`[][]rune`) and all edit/cursor/selection logic are
untouched** — only rendering and the render-support helpers change. This keeps the
correctness risk off the editing path and makes the change cleanly revertible.

## The per-frame O(buffer) costs to eliminate

1. **(dominant) `View()` styles every line** (`textarea.go` ~1188): the `for l, line := range
   m.value` loop wraps + `style.Render`s every source line and builds the full content string.
2. **Dim sentence span via `m.Value()`** (the `currentSentenceSpan(m.Value(), …)` call in
   `View`): `m.Value()` joins the whole buffer into one string each frame.
3. **Cursor display-row walks** (`cursorLineNumber` ~1398, `repositionView` ~885): `for i := 0;
   i < m.row` loops calling `memoizedWrap` — cheap **iff** the wrap cache holds the lines.
4. **The internal bubbles `viewport`** is handed the full content via `renderViewport`
   (`SetContent(full)`), then clips — so it holds/processes the whole buffer.

## Design (Approach A — windowed render)

Render only the visible window directly, the way `pager.go` already does, and drop the
internal viewport's whole-content role.

- **Visible window.** Track the first visible **display row** (`top`) and `height`. Derive
  `top` from the existing scroll/typewriter offset logic (today expressed via
  `viewport.YOffset` / `repositionView` / the typewriter `SetYOffset(cursorLineNumber())`),
  re-expressed as an explicit display-row integer.
- **Display→source mapping.** Walk source lines accumulating wrapped-row counts
  (`memoizedWrap`) to find the first source line + intra-line wrap piece at `top`, then the
  last source line whose pieces reach `top+height`. With the cache sized to the buffer this
  walk is cached-cheap (summing ints).
- **Render only the window.** Wrap + style + segment-render (`renderSeg`, cursor-line style,
  dim span) **only** the source lines intersecting `[top, top+height)`; skip the leading wrap
  pieces before `top` in the first visible line; stop after exactly `height` rows; pad with
  end-of-buffer rows if the window runs past the last line. Reuse the existing per-segment
  render logic verbatim — only the *set of lines* it runs over changes.
- **No internal viewport for content.** Return the windowed string directly (it is already
  exactly `height` rows). Keep typewriter centring by computing `top` so the cursor row sits
  where typewriter mode wants it.

### Supporting changes (also O(buffer) today)

- **Wrap cache capacity.** Decouple from `MaxHeight`: the resize at `textarea.go` ~1047 is
  gated on `MaxHeight > 0`, so with okashi's `MaxHeight = 0` the cache is stuck at 99 and
  thrashes on a big buffer. Size it to `max(defaultMaxHeight, len(m.value))` (or grow on
  demand) so the mapping walk and cursor-row helpers hit the cache.
- **Dim sentence span without `m.Value()`.** Compute the cursor's sentence span from the
  cursor-local neighborhood (the current and adjacent source lines) instead of joining the
  whole buffer each frame. Behaviour for the visible region must match today's output.

## What stays exactly the same (low-risk boundary)

- `m.value` (`[][]rune`) buffer, and every insert/delete/split/join/cursor/word-motion op.
- The per-segment render (`renderSeg`), cursor-line styling, dim styling, smart quotes,
  `MoveToLine`, focus/blur. The *visible output* is byte-identical to today's for the same
  scroll position; only off-screen lines stop being processed.

## Testing

- **All existing `internal/textarea` tests pass unchanged** (`dim_test`, `editing_test`,
  `moveline_test`, `typewriter_test`) — the behavioural safety net.
- **Windowing correctness:** for a multi-screen buffer, the windowed `View()` output equals
  the corresponding slice of a full reference render at top, middle, and bottom scroll
  positions; the cursor line is always within the window; exactly `height` rows are emitted;
  typewriter centring is preserved; wrapped (multi-row) lines split across the window edge
  render their correct pieces.
- **Performance gate (the acceptance bar):** `BenchmarkEditorViewWholeDraft` drops to roughly
  `BenchmarkEditorViewChapter` (~1.5 ms or better) and is **flat** vs `…HalfDraft` (the
  O(visible) proof). Benchmarks already committed as the before/after guard.

## Rollback

Implemented on its own branch; the render rewrite is an **isolated commit** (separate from the
cache-size and dim-span commits where practical). Because the buffer/edit code is untouched,
reverting is a clean `git revert` of the render commit (or simply not merging the branch).

## Out of scope

- Gap buffer / any buffer data-structure change (addresses edit cost, which is not the
  bottleneck).
- A Fenwick/prefix-sum incremental wrap-height index (Approach B) — a cached linear scan is
  microseconds for realistic manuscripts; the incremental index is YAGNI and adds edit-path
  bug surface.
- Any feature/behaviour change. This is purely a performance rework of rendering.
