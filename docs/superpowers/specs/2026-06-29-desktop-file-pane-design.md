# Desktop file-pane feature set — design

**Date:** 2026-06-29
**Status:** Approved
**Context:** Round the file pane into a desktop-style manager: in-place rename, in-place create
(with a clickable `+`), delete (confirmed), and duplicate — all in-place, NO fragile floating
context menus. (Supersedes the standalone in-place-rename spec.)

## Shared: in-place input in a file-list row

Rename and create both edit the existing `m.nameInput` and render it INSIDE the file list rather
than the bottom bar:
- `m.renamingInPane bool` (set in `startRename`, cleared on confirm/cancel). `m.creatingFile`
  already exists.
- `filelist.View(editRow int, editField string)` — `editRow >= 0` & rename → that row renders
  `editField`; create → `editField` is shown as a NEW row at the top (after `..`). `editRow = -1`,
  `editField = ""` disables it. The model passes `m.nameInput.View()` when renaming/creating;
  `startRename`/create set `m.nameInput.Width = m.files.width`.
- `statusBar`: only the OUTLINE rename/create still uses the bottom bar
  (`if (m.renaming||m.creatingFile) && !m.renamingInPane && !m.creatingInPane`); file-pane edits
  are in-row. (Add `m.creatingInPane bool`, true for file-pane `ctrl+n`/`+`.)
- Enter → existing `confirmRename`/`confirmCreate`; Esc cancels.

## 1. In-place rename (right-click + r/F2)

- Triggers (all call the existing `startRename`, which validates + blocks manifest chapters):
  the existing `r` (sidebar focus); a `MouseButtonRight` press in the sidebar → `sidebarRow` →
  `selectRow` + `startRename`; `tea.KeyF2` (global, writing screen) → focus sidebar + `startRename`.
- The field renders in the selected row; manifest chapters / `..` refuse with the status note.

## 2. In-place create + clickable `+`

- `framedPanel(title, inner, width, height, action string)` — gains a trailing `action` glyph
  rendered at the RIGHT of the top border (`╭ title ─fill  + ╮`); `""` = none (the inspector passes
  `""`). The sidebar passes `"+"`.
- A `MouseButtonLeft` press on the `+` cell (top border row, the panel's `+` column — computed from
  the panel width) starts in-place create; `ctrl+n` does too. Both: `m.creatingFile = true`,
  `m.creatingInPane = true`, the new-row field appears; type a trailing `/` for a folder (existing
  `confirmCreate` convention).

## 3. Delete (confirmed)

- `tea.KeyDelete` (sidebar focus) on the selected entry → a bottom-bar confirm
  (`delete 'name'? [y]es · [esc] cancel`, `m.deleting bool` + target). `y` → `os.Remove` (file) /
  `os.RemoveAll` (dir) → `refreshAfterRename`-style refresh + clamp selection; any other key cancels.
- Blocked for manifest chapters (deleting one desyncs the shared manifest) and `..`/`manifest.json`
  with the status note. Allowed for loose files, legacy chapters, folders, resources.

## 4. Duplicate

- `d` (sidebar focus) on the selected file → copy its bytes to a free `name copy.ext` (e.g.
  `draft copy.md`, then `draft copy 2.md`) in the same dir → refresh + select the copy. Files only
  (skip dirs/`..`/manifest chapters for v1).

## Keymap additions (and `helpText`)

`F2` rename · right-click rename · `+`/`ctrl+n` new · `Delete` delete · `d` duplicate. Add these to
the F1 cheatsheet.

## Testing

- In-place render: renaming/creating shows the input (cursor) in the row / new row, not the bottom
  bar; Enter applies via the existing confirm; Esc cancels.
- Rename triggers: right-click & F2 open the field on the right row; manifest chapter refuses.
- Create: clicking the `+` (computed top-border column) and `ctrl+n` both open the new-row field; a
  trailing `/` makes a folder; the file is created on Enter.
- Delete: `Delete` → confirm → `y` removes the file (gone from disk + list); Esc keeps it; manifest
  chapter refuses.
- Duplicate: `d` → `name copy.ext` exists with the same bytes and is selected; a second `d` →
  `name copy 2.ext`.
- Existing rename/create/confirm tests stay green; panel click geometry (the `+` is on row 0, file
  rows below) stays aligned — gated by a render-based alignment check.

## Out of scope

- Outline-view in-place edit; drag-and-drop/move; multi-select; a floating right-click menu;
  undo/trash (delete is permanent with a confirm).
