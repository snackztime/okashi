# Sidebar framed-card styling — design

**Date:** 2026-06-29
**Status:** Approved (carry the inspector's framing to the sidebar)
**Context:** The inspector is now a rounded "card" (cycle 5). Carry the same treatment to the
file-pane sidebar for consistency, reusing the existing `framedPanel` helper. Same geometry
discipline (render == reservation == offset; top border shifts the click rows).

## Look

A full rounded border around the sidebar, the **current folder's name** in the top border as the
title, the breadcrumb path kept as the first inner line, then the file list. No rule-style section
headers (the file list has no sections — frame only).

```
╭ my-novel ──────────────────╮
│ ~/writing › my-novel       │   (breadcrumb, clickable segments)
│  01 Opening                │
│  02 The Letter             │
│  03 …                      │
╰────────────────────────────╯
```

## Components

- `framedPanel(filepath.Base(m.files.dir), sideInner, sidebarWidth, bodyH-2)` replaces
  `sidebarStyle.Width(sidebarWidth-1).Height(bodyH-2).Render(sideInner)`. (If `m.files.dir` is empty,
  title falls back to `"Files"`.)
- **Width consistency:** render the sidebar at EXACTLY `sidebarWidth` so render == reservation
  (`editorArea -= sidebarWidth`) == offset. Bump `sidebarWidth` 32→34 (symmetric with the inspector)
  so the inner content width = `sidebarWidth - 4` = 30 (keeps filenames readable). The breadcrumb is
  built at the inner width (`sidebarWidth - 4`), and the file list View at the inner width.
- **Content height:** the frame's top+bottom borders cost 2 rows, so `m.files.height` becomes
  `bodyH - 5` (was `bodyH - 3`: content `bodyH-4` minus the 1 breadcrumb row).

## Geometry & click alignment (the careful part — sidebar is at screen col 0)

The top border is row 0, so both clickable zones shift down one row:
- **Breadcrumb segments:** the row check `if msg.Y == 0` → `if msg.Y == 1`; the X offset
  `col := msg.X - 1` → `col := msg.X - 2` (left border + 1 padding, vs the old padding-only).
- **File rows:** `sidebarRow(msg.Y, 1, m.files.height)` → `sidebarRow(msg.Y, 2, m.files.height)`
  (top border + breadcrumb = 2 banner rows). File selection uses the row only (no X), so the X
  offset is irrelevant there.
- The sidebar is the leftmost column (screen col 0), so there is no panel-left term — only the
  in-panel border/padding offsets above.

## Testing

- A render-based alignment gate (like the inspector's): with the sidebar open, find a file entry's
  on-screen row, click it, and assert that file becomes selected; double-click opens it. Find a
  breadcrumb segment's on-screen column, click it, assert navigation.
- No layout overflow (rune-column width ≤ window width) with sidebar + editor + inspector all open.
- Existing file-pane tests updated to the framed positions (the `+1` row for the top border)
  without weakening assertions; the file list still shows the right entries at the new height.

## Out of scope

- Changing file-pane behavior (selection/open/navigation logic), icons, or sorting — chrome only.
- A title other than the folder name; section headers inside the sidebar.
