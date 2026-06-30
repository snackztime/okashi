# In-place file rename — design

**Date:** 2026-06-29
**Status:** Approved (right-click + r/F2; field renders in the row)
**Context:** Renaming already works (`r` → `nameInput` → `confirmRename`, with the manifest-chapter
block) but the editable field shows in the bottom bar. Move it INTO the file row, and add
right-click + F2 triggers.

## Behavior

- **Triggers** (all call the existing `startRename`, which validates and blocks manifest chapters):
  - Existing `r` in sidebar focus (unchanged).
  - **Right-click** a file row → select that row + `startRename`.
  - **F2** (global, writing screen) → focus the sidebar + `startRename` on the selected entry.
- **In-place field:** while renaming a file-pane entry, the file-list row being renamed draws the
  `nameInput` (value + cursor) instead of the filename; the bottom-bar `rename ▸ …` is suppressed.
- Enter confirms, Esc cancels (unchanged `confirmRename`). Manifest chapters / `..` still refuse with
  the existing status note (no field opens).
- Outline-section rename (`startRenameOutline`) is unchanged (stays in the bottom bar) — only the
  file pane goes in-place.

## Components

- **`m.renamingInPane bool`** — set `true` in `startRename`, `false` in `startRenameOutline`.
  `startRename` also sets `m.nameInput.Width = m.files.width` so the field fits the pane.
- **`filelist.View(renameIdx int, renameField string)`** — when rendering row `i == renameIdx`,
  draw `renameField` (the `nameInput.View()`, ansi-truncated to `f.width`) as the row instead of the
  filename. `renameIdx = -1` disables it (all other call sites pass `-1, ""`).
- **View call site (main.go):** `renameIdx, renameField := -1, ""; if m.renaming && m.renamingInPane
  { renameIdx = m.files.selected; renameField = m.nameInput.View() }`.
- **statusBar:** `if m.renaming && !m.renamingInPane { return "rename ▸ " + … }` (bottom bar only for
  the outline rename now).
- **Triggers (main.go MouseMsg + key switch):** a `MouseButtonRight` press in the sidebar →
  `sidebarRow` → `selectRow` + `startRename`; a `tea.KeyF2` case → focus sidebar + `startRename`.
- Add `F2  rename file` (and a note that right-click renames) to `helpText`.

## Testing

- `startRename` sets `renamingInPane=true`; the file-list View then renders the input (cursor) at the
  selected row and NOT in the bottom bar; a manifest chapter refuses (no field, status note).
- Right-click a file row → that row is selected and `m.renaming` + `renamingInPane` true, field shows.
- F2 → same as `r` (sidebar focus + field in row).
- Enter applies the new name (existing confirmRename); Esc cancels; outline rename still uses the bar.
- Existing rename/confirm tests stay green (the engine is unchanged).

## Out of scope

- In-place rename in the outline view; drag-rename; right-click context menu (just direct rename).
