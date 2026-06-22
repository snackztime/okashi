# File-pane polish: launch hub, creation, mouse, clean look — design

**Date:** 2026-06-22
**Status:** Approved (pending spec review)
**Queue:** This batch (file-pane polish) → focus dimming (Plan 2, spec'd) → project rename.

## Goal

Turn the launch screen into a real hub and the file pane into a clean, fully
mouse-driven, navigable browser:

1. **Launch hub** — recents + projects + **New document** / **New project** / Browse.
2. **Creation** — make documents and folders from the hub or the sidebar; the
   `name/` convention makes a folder, with a live prompt hint.
3. **Mouse everywhere** — launch screen click/double-click/wheel (matching the
   file pane), and a **clickable breadcrumb** for folder navigation.
4. **Clean (yazi-style) look** — full-width selection bar, left gutter, dim file
   extensions, scroll indicator.

Documents created without a project live at the okashi workspace root (for now).
Sidebar width stays 32 (configurable width deferred). No Miller panes.

---

## Section 1 — Launch hub: creation flows

`home.go`:
- `homeKind` gains `homeNewDocument` and `homeNewProject` (keep `homeRecentFile`,
  `homeProject`, `homeOpenOther`="Browse all files").
- `buildHomeItems` order: recents, projects, then the **actions group**
  (rendered after a blank line, no header): `New document`, `New project`,
  `Browse all files`. Action icons: new = a "+" glyph (nerd ``, plain `+`),
  browse = folder.
- `openHomeSelection` (returns a `tea.Cmd` now, so the prompt can blink):
  - `homeNewDocument` → `m.files.SetDir(writingDir())`, `screen=screenWriting`,
    start the create prompt in **file** mode.
  - `homeNewProject` → `m.files.SetDir(writingDir())`, `screen=screenWriting`,
    start the create prompt in **folder** mode.
  - Starting the prompt = `m.creatingFile=true`, set `m.creatingFolder`,
    `m.nameInput.SetValue("")`, `m.nameInput.Focus()`, return `textinput.Blink`.

**Testing:** `buildHomeItems` includes the three actions in order; activating
New document/New project sets `screenWriting` + `creatingFile` with the right
`creatingFolder`.

---

## Section 2 — Creation: files, folders, and the prompt hint

Generalize `confirmNewFile` → `confirmCreate` (`main.go`), plus a `creatingFolder
bool` on the model.

- **Decision:** `folder := m.creatingFolder || strings.HasSuffix(name, "/")`;
  strip a trailing `/`.
- **Folder** → `os.MkdirAll(filepath.Join(m.files.dir, name), 0o755)`. Then:
  - If `m.creatingFolder` was set explicitly (the hub's **New project**) →
    `SetDir` *into* the new folder (you're now in the project, ready to add
    docs), focus the sidebar, status "new project N".
  - Otherwise (the sidebar `name/` convention) → refresh the current list
    (`SetDir(m.files.dir)`), `selectName(name)`, stay put, focus the sidebar,
    status "created folder N" (yazi-style: creating a dir doesn't enter it).
- **File** → existing behavior (default `.md`, blank buffer, focus editor),
  created in `m.files.dir`.
- Reset `m.creatingFolder=false` at the end either way.

**Prompt hint (discoverability):**
- `nameInput.Prompt` is cleared; the create prompt is composed in `statusBar`.
- `folderMode := m.creatingFolder || strings.HasSuffix(m.nameInput.Value(), "/")`.
- Render: `"new folder ▸ "` when `folderMode` else `"new file ▸ "`, then
  `m.nameInput.View()`, then a right-aligned hint **`end with / for a folder`**
  (shown while not already in folder mode). So the label live-flips to
  "new folder ▸" the instant the input ends in `/`.

**Sidebar `ctrl+n`** is unchanged except it now routes to `confirmCreate`
(`creatingFolder=false`; a trailing `/` still makes a folder).

**Testing:** `confirmCreate` with `"foo/"` and with `creatingFolder=true` both
make a directory and refresh+select; `"foo"` makes `foo.md`; the status-bar
label flips to "new folder ▸" when the value ends in `/`.

---

## Section 3 — Mouse: launch hub + clickable breadcrumb

### 3a. Launch hub mouse (`updateHome`)

Mirror the file pane: single-click selects, double-click (same item < 400ms)
activates, wheel moves the selection.

The hit-test must agree with what `homeView` draws. To prevent drift, a single
helper owns the layout:

```go
// homeRows returns the rendered content lines, the screen row (within the
// content block) of each selectable item by index, and the total content height.
func homeRows(items []homeItem, sel int, icons iconSet) (lines []string, itemRow []int, height int)
```

- `homeView` calls `homeRows`, then vertically centers: top offset
  `off = max(0, (m.height - height) / 2)`.
- The mouse handler calls `homeRows` too, computes the same `off`, then
  `contentRow = msg.Y - off`, and maps `contentRow` back to an item index via
  `itemRow` (the inverse: the item whose `itemRow[i] == contentRow`). Clicks on
  logo/header/blank rows map to no item (ignored).
- Double-click reuses the existing `lastClickRow`/`lastClickTime`.

**Testing:** `homeRows` returns monotonically increasing item rows; a pure
`homeItemAtY(items, sel, icons, height, y)` returns the right index for an
item's row and -1 for logo/blank rows.

### 3b. Clickable breadcrumb + head-truncation (`filelist` + `main.go`)

- `breadcrumbSegments() []breadcrumbSeg` where
  `breadcrumbSeg{ label, path string }`: the root base (`path=root`) then each
  ancestor dir down to the current (`path` = that dir). Joined by `" / "`.
- **Head-truncation:** if the joined width exceeds the pane width, keep the root
  segment and the rightmost segments that fit, replacing the dropped middle with
  a non-clickable `…` segment → `okashi / … / Drafts`. Visible segments stay
  clickable.
- Render returns each segment's column range; the breadcrumb row reserves space
  on the right for the scroll indicator (§4).
- **Mouse:** a click in the breadcrumb row (screen row 0 of the sidebar,
  `X < sidebarWidth`) maps `X` (minus the sidebar's left inset) to a segment via
  its column range → `m.files.SetDir(seg.path)`. Separators / `…` map to nothing.

**Testing:** `breadcrumbSegments` returns root + ancestors with correct paths;
head-truncation keeps root + tail and inserts `…`; a column-range → segment
mapping returns the right path for an X inside a segment and none for separators.

---

## Section 4 — Clean (yazi-style) look (`filelist.View` + breadcrumb)

All four, applied in `filelist.View()` and the breadcrumb render:

- **Left gutter:** every row is prefixed with one space before the icon (the
  selection bar includes the gutter).
- **Full-width selection bar:** the selected row renders as a solid `accent`
  bar across `f.width` (current `selectedStyle.Width(f.width)`, with a
  contrasting foreground), now including the gutter.
- **Dim file extensions:** for file entries (not dirs / `..`), render the stem
  in the normal foreground and the extension (`.md`) in `subtle`. Skipped for
  the selected row (the whole bar is accent).
- **Scroll indicator:** when `len(entries) > height`, the breadcrumb row shows
  `sel+1/total` (e.g. `3/12`) right-aligned. It lives in row 0, so it adds no
  row and does not change the mouse offset; the breadcrumb truncates to leave
  room for it.

**Testing:** rows carry the gutter; a file row's extension is styled with
`subtle` while its stem isn't; the indicator string is `"<sel+1>/<total>"` and
absent when everything fits.

---

## Out of scope (non-goals)

- Miller / multi-column panes.
- Configurable sidebar width (`OKASHI_SIDEBAR_WIDTH`) — deferred to after the
  real-terminal pass.
- Settings pane, spell-check, syntax control, focus dimming (separate).
- Drag-and-drop, rename/delete in the pane, multi-select.
- File preview in the pane.

## Risks

- **Launch hit-testing drift:** render and hit-test MUST share `homeRows`; a
  separate ad-hoc calc would desync clicks from what's drawn. (Mirrors the
  banner/breadcrumb offset lesson.)
- **Breadcrumb column mapping** must match the rendered segment widths exactly
  (unicode/icon widths via `lipgloss.Width`).
- **Folder creation in the workspace** writes real directories — confine to the
  current pane dir (already within the root) and never above root.
- **Tests must stay hermetic** — any test touching `writingDir()` sets
  `t.Setenv("OKASHI_DIR", t.TempDir())`.

## Build order (plans)

- **Plan 1 — Launch hub + creation:** Sections 1, 2, and 3a (hub actions,
  `confirmCreate` + folder convention + prompt hint, launch-screen mouse).
- **Plan 2 — File-pane look + breadcrumb nav:** Sections 3b and 4 (clickable
  breadcrumb + head-truncation, the four clean-look touches).
