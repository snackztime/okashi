# Status bar inside the editor column + full-height panels — design

**Date:** 2026-06-29
**Status:** Approved (user-specified)
**Context:** Move the status bar (stats/save-open AND the spelling hint) OUT of the full-width
bottom row and INTO the editor column, with a blank line above it, so the two side panels render
truly full height (the status no longer spans beneath them).

## Layout (writing screen)

- **Panels full height:** the framed sidebar + inspector render at `m.height` (was `bodyH = m.height-1`).
  `m.files.height = m.height - 2` (panel content = `m.height` minus top+bottom border; was `bodyH-2`).
- **Editor column** = `JoinVertical(editorPane, blankLine, statusRow)`, `editorArea` wide, `m.height` tall:
  - `editorPane = Place(editorArea, editorH, Center, Top, pane)` with `editorH = m.height - 2`.
  - `blankLine` = one `editorArea`-wide blank row (the line break).
  - `statusRow = statusStyle.Width(editorArea).Render(statusBar())` — status limited to the editor width.
  - Layout sets `m.editor.SetHeight(m.height - 2)` (was `bodyH`); `m.preview.Height = m.height - 3`.
- `body = JoinHorizontal(Top, sidebar, editorColumn, inspector)`; **return body** — no separate
  full-width status row. (Help/preview/other-screen branches keep their own layout; this is the
  normal writing view only.)

## composeStatus (now relative to the editor column)

The status is rendered at `editorArea` width inside the editor column, so positions are
column-relative (no `editorStart` term): `totalW = editorArea - 2` (status padding); text-left
content col = `(editorArea - cw)/2 - 1`; stats there, status right-aligned to `+cw`. Stats win on
overflow (unchanged logic, just `editorArea` instead of `m.width` and no `editorStart`).

## Click geometry (the careful part)

- **Editor clicks:** the editor pane is rows `0..m.height-3` (then blank `m.height-2`, status
  `m.height-1`). Fire the editor-click→`ClickTo` handler for `msg.Y < m.height-2` (was `< m.height-1`).
- **Suggestion clicks:** the spell hint renders left-aligned inside the editor column, so its content
  starts at `editorStart + 1` (column left + padding). The status-row hit-test offset becomes
  `msg.X - editorStart - 1` (was `msg.X - 1`), where `editorStart = sidebarWidth` if the sidebar shows.
  Row check stays `msg.Y == m.height-1` (status is the column's last row).
- **Panel clicks unchanged:** panels grew only at the bottom; their content positions from the top
  border (row 0) — tab/checkbox/file-row hit-tests are identical (`files.height` just allows 1 more row).

## Testing / verification (gate)

Empirically (rune-column): file rows + inspector tabs/checkboxes still click correctly; an editor
click still lands the cursor in the clicked word; a suggestion click still applies; the status +
spelling hint render only within the editor column (not under the panels); a blank row sits between
the editor text and the status; the total layout is exactly `m.height` rows with panels full height.
