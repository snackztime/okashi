# Status bar redesign + full-height panels + keybind help — design

**Date:** 2026-06-29
**Status:** Approved
**Context:** Make the side panels full height; rework the status bar into a clean two-element bar
aligned to the editor text column (stats left, last save/open right); and move the verbose keybind
hints out of the always-on bar into a toggleable cheatsheet.

## 1. Full-height panels

The framed sidebar + inspector render at `bodyH-2` while the editor is `bodyH`, leaving a 2-row gap
at the bottom of each card. Render both panels at **`bodyH`** (full height, flush with the editor;
the status bar already has its own reserved row). The framed content height grows by 2, so
`m.files.height` becomes `bodyH-2` (was `bodyH-4`: panel `bodyH` minus top+bottom border).

## 2. Status bar layout (the normal, non-prompt case)

Today `composeStatus` puts `m.status` (overloaded: save/open messages AND a long keybind hint) on
the left and centers the stats over the editor. Rework the normal case to span the **editor text
column** (the centered measure, not the whole window):
- Compute `editorStart` (= `sidebarWidth` when the sidebar shows, else 0), `editorArea` (from
  `effectivePanels()`), and `cw := min(m.colWidth, editorArea-2)` (the editor text width). The text's
  left column is `textLeft = editorStart + (editorArea - cw)/2`.
- **LEFT** at `textLeft`: the stats — `✓ 1,240 words · +142 session` (dirty mark `✓`/`●` + `statsText()`,
  session delta unchanged).
- **RIGHT**, right-aligned so it ends at `textLeft + cw`: `m.status` (the last save/open or transient
  message), truncated to the space remaining after the stats.
- If stats + status don't both fit, the stats win (status truncates/drops). Prompt cases
  (creatingFile/suggesting/spell-hint/renaming/goal/export) keep their existing full-row takeover.

## 3. Keybind cheatsheet on F1

- `?` is text in the editor, so the toggle is **F1** (universal help key; never typed as prose).
  `m.showHelp bool`; F1 toggles it; F1/esc/any key closes it.
- The current verbose hint string (the long `ctrl+b sidebar · …` that was the default `m.status`)
  becomes the cheatsheet content, formatted as a readable list, rendered in a **centered
  `framedPanel`** titled "Keys" over the body when `m.showHelp` is true (reuse the framing helper).
- **Default `m.status`** changes from the giant hint to short/empty (e.g. `""` or the open filename),
  since the hints now live behind F1. Transient/save/open messages still set `m.status` and show on
  the right.

## Testing

- Full height: with the sidebar/inspector open, the framed panels are `bodyH` tall (bottom border on
  the last body row, no gap); `m.files.height == bodyH-2`; no overflow.
- Status layout: with a file open + edits, the rendered status row has the stats at the editor
  text-left and `m.status` right-aligned within the editor text column; when `m.status` is long it
  truncates rather than overrunning the stats; a narrow window degrades to stats-only.
- F1 help: F1 sets `m.showHelp`, the View shows the "Keys" cheatsheet (contains e.g. "ctrl+b" and
  "sidebar"); F1/esc closes it; while help is open, keys don't edit the buffer; `?` still types "?" in
  the editor (NOT a toggle).
- Existing status/prompt tests updated to the new layout/default without weakening.

## Out of scope

- The clickable spellcheck suggestions (next sub-project — sits on this bar).
- Help content beyond the keybind list; configurable help key; a today-delta (session stays).
