# Plan B — Outline view: design (finalized)

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Parent spec:** `docs/superpowers/specs/2026-06-22-long-form-projects-design.md` (§4,
§7, §8). This document **finalizes** the "candidate" decisions that §4 deferred to
the plan, and **narrows scope** for Plan B. Where this doc and the parent spec
differ, **this doc governs for Plan B**.

## Goal

A full-screen **outline view** for a manuscript: see every ordered section with its
title and word count, open one in the editor, **reorder** sections (which renumbers
the files on disk, safely and reversibly), and **add** a new section. Built on the
Plan A data layer (`orderedSections`, `sectionTitle`, `projectWordCount`,
`wordCountCache`, `backupFiles`/`backupStamp`), which already shipped to `main`.

## Scope

**In Plan B:** the `screenOutline` screen; section list with titles + per-section
and total word counts; select; open-in-editor; keyboard **reorder** with deferred,
confirmed commit (renumber-on-disk + backup); **new section** (`n`) inserted after
the selection (renumber-on-disk + backup); mouse (click selects, double-click opens).

**Deferred (not Plan B):**
- **Manuscript view** (`m`) — Plan C. In Plan B, `m` shows a stub status line.
- **Section delete** — its own later concern (the backup helper exists; the destructive
  path is reused, but delete is not built here).
- **Standalone on-demand `b` backup** (parent spec §7) — the destructive ops already
  snapshot before acting, so a manual whole-project backup is left for later.

## Architecture

A self-contained `outline.go` holding an `outlineModel` struct, mirroring how
`filelist` is structured. `main.go` adds `screenOutline` to the `screen` enum
(`main.go:109`) and **delegates** key/mouse/`View` handling to the outline model when
`m.screen == screenOutline`. The renumber logic is a **pure, unit-tested**
`planRenames` function with no disk access; a thin `applyRenames` performs the disk
work. Rationale: keeps the already-large `main.go` from absorbing another subsystem,
and makes the one destructive operation testable in isolation.

`outlineModel` owns:
- `projectDir string` — the manuscript folder (captured from `m.files.dir` on entry).
- `sections []fileEntry` — the **working** (possibly reordered) order of numbered sections.
- `diskOrder []fileEntry` — the order as currently on disk, to detect a pending reorder.
- `loose []fileEntry` — unnumbered files (display only; not reorderable).
- `selected int` — index into the visible rows.
- a reference to the existing `*wordCountCache` (reuse `m.files.wc`, do not allocate a second cache).
- pending-confirm state (whether the apply/discard dialog is showing).

## Entry / exit

- **Enter the outline:** `ctrl+l` from the editor (`screenWriting`). `ctrl+l` is
  unused by the current keymap (`ctrl+b/n/o/p/s/t/d`, `esc`, `tab`, `shift+tab`).
  Allowed **only when `isManuscript(m.files.entries)`** is true; otherwise set a status
  hint ("not a manuscript folder") and stay in the editor.
- On entry, capture `projectDir = m.files.dir`, build `sections`/`loose` via
  `orderedSections`, set `diskOrder = sections`, `selected = 0`.
- **Leave:** `esc` returns to `screenWriting`. If a reorder is pending, the confirm
  gate (below) runs first.
- `m` → status stub: "manuscript view — Plan C".

## Layout & rendering (shared `outlineRows`)

A single `outlineRows(...)` helper produces the ordered list of rows used by **both**
`View` and the mouse hit-test, so a click can never address a different row than the
one drawn (the launch-hub / breadcrumb lesson; see parent spec Risks).

- **Header:** `<de-slugged project title>  ·  <total>w  ·  <N> sections`. Title is the
  de-slugged `filepath.Base(projectDir)`: drop a trailing extension if any and replace
  `-`/`_` with spaces. **Do not** strip a leading digit run here (unlike `sectionTitle`)
  — a folder named `2024-trip-journal` must read as "2024 trip journal", not "trip
  journal". Use a small dedicated `projectTitle(name string)` helper rather than reusing
  `sectionTitle`. Total via `projectWordCount(projectDir, sections, wc)`.
- **Section rows:** one per numbered section: `NN  Title  ········  1,240w` — the
  current on-disk prefix `NN`, the `sectionTitle`, dot leaders, and a right-aligned
  `commafy(count)+"w"` (count via the shared `wordCountCache`). The selected row uses
  `selectedStyle`.
- **Pending-order marker:** when `sections != diskOrder`, show an "● unsaved order"
  indicator in the header (or a status line).
- **Loose group:** after the sections, a dimmed group of loose files (filenames).
  Selectable (so they can be opened) but **not reorderable** and excluded from the
  total. No heavy separator is required; grouping is by position + dim styling.

## Selection & open

- `↑`/`↓` and `j`/`k` move `selected` (clamped across sections + loose rows).
- `Enter` (or double-click): if a reorder is pending, run the confirm gate first;
  then `loadFile(path)` for the selected row, switch to `screenWriting`, focus the
  editor. If the commit renamed the selected file, open its **new** path.
- Single-click selects (sets `selected` from the hit-tested row). Double-click opens
  (reuse the existing `lastClickRow`/`lastClickTime` double-click detection).

## Reorder (deferred commit + confirm gate)

- `J` / `K` **and** `shift+↑` / `shift+↓` move the selected **numbered** section up/down
  within `sections` (in memory only; loose rows are not movable and these keys are
  no-ops there). `selected` follows the moved section. Clamp at the ends.
- Nothing touches disk until commit. While `sections != diskOrder`, the "unsaved
  order" marker shows.
- **Commit gate:** when leaving (`esc`) or opening (`Enter`) **with a pending reorder**,
  show a confirm prompt:
  > **Apply reordering?**  `y` apply · `n` discard · `esc` keep editing
  - `y` (apply): `backupFiles(projectDir, backupStamp(time.Now()), <all section paths>)`,
    then `applyRenames` to contiguous, zero-padded prefixes; update `m.currentFile` if it
    was renamed; `m.files.SetDir(projectDir)` to refresh the sidebar; complete the
    pending action (leave or open).
  - `n` (discard): set `sections = diskOrder`, complete the pending action.
  - `esc` (keep editing): dismiss the dialog, stay in the outline.
- There is **no separate `u` key** — the discard option in the dialog covers reverting.
  A full snapshot lands in `.backup/` on every apply, so even a confirmed mistake is
  recoverable from disk.

## New section (`n`, insert-after-selected)

- `n` opens the name prompt (reuse the existing `nameInput`/`creatingFile` flow, scoped
  to the outline). The user types a title.
- The new section is inserted at `selected + 1` among the numbered sections; everything
  from there down shifts one slot. This is the **same** backup-then-renumber path as a
  reorder commit, with one inserted entry. The new file is created as `NN-<slug>.md`
  (slug = the typed title lowercased, spaces/`_` → `-`), then the renumber runs so all
  prefixes stay contiguous and consistently padded.
- A backup fires before the renumber (creation + shift of existing files is destructive
  to their names).

## Renumber semantics (`planRenames` / `applyRenames`)

- **Pad width:** `width = max(2, digits(count), widestExistingPrefixWidth)`. Two digits
  normally; the commit that brings the count to 100 widens all prefixes to three in one
  pass; width **auto-grows but never shrinks** (dropping below 100 keeps three digits to
  avoid filename churn).
- **`planRenames(orderedPaths []string, width int) []renameOp`** — pure, no disk.
  Produces, for the target order, the `(oldPath, newPath)` pairs where `newPath` keeps
  the section's title slug and extension but gets prefix `fmt.Sprintf("%0*d", width, i+1)`.
  Pairs whose old == new are omitted. This is the primary unit-test target.
- **`applyRenames(ops []renameOp) error`** — performs the renames with a **two-phase
  temp pass** so order-swaps (e.g. `01`↔`02`) do not collide: rename each source to a
  unique temp name, then temp → final. All paths confined to `projectDir`; reject any
  op whose target escapes it (`filepath.Rel` guard, same as `withinRoot`).
- After applying, if `m.currentFile` matched any `oldPath`, update it to the `newPath`.

## Backups

Reuse Plan A unchanged: `backupStamp(time.Now())` for a filesystem-safe stamp, and
`backupFiles(projectDir, stamp, paths)` to copy the section files into
`<projectDir>/.backup/<stamp>/` **before** any renumber (reorder apply or new-section
insert). `.backup/` stays excluded from the pane (dotfile) and from `isManuscript`.

## Testing (hermetic — `t.TempDir`, `t.Setenv("OKASHI_DIR", …)`)

- `outlineRows` returns one monotonic row per section then the loose group; row count
  matches what `View` renders (hit-test ↔ render parity).
- `planRenames`: reorder `[01,02,03]` moving #3 up yields renames placing the moved
  section in slot 2 with contiguous prefixes `[01,02,03]`; a pure swap `01↔02` produces
  two ops; an already-correct order produces zero ops.
- `applyRenames` two-phase: swapping `01`↔`02` on disk succeeds without a collision and
  leaves exactly the two files with swapped contents.
- Open-file tracking: when the currently-open file is renamed by a commit,
  `m.currentFile` follows to the new path.
- New section: `n` after section 2 in `[01,02,03]` creates a file at slot 3 and
  renumbers the old `[03]` to `[04]`; the new file exists with the typed slug.
- Width: a 99→100 transition widens all prefixes to three digits; a later drop below
  100 keeps three digits.
- Backup: a reorder apply and a new-section insert each leave a `.backup/<stamp>/`
  snapshot of the pre-change section files.
- Guards: loose files are excluded from `sections`/total and are not movable;
  `ctrl+l` is rejected (status hint) when the dir is not a manuscript;
  `applyRenames` rejects a target outside `projectDir`.

## Risks

- **Reorder renames real files** — the one destructive op. Mitigations: deferred,
  **confirmed** commit (no silent rename on a stray `esc`); a `.backup` snapshot fires
  first; renames are confined to `projectDir` and validated against path escape; the
  two-phase temp pass avoids swap collisions; the open file's path is updated after rename.
- **Hit-test drift** — render and mouse share `outlineRows` (single source of layout).
- **Pad-width transition** — the 99→100 widening renames every file at once; it rides
  the same backup-protected renumber path and has an explicit test.
- **Tests stay hermetic** — anything touching project dirs uses `t.TempDir()` /
  `t.Setenv("OKASHI_DIR", …)`.

## Keys (Plan B summary)

| Key | Action |
|-----|--------|
| `ctrl+l` | Editor → Outline (manuscript dirs only) |
| `↑`/`↓`, `j`/`k` | Move selection |
| `J`/`K`, `shift+↑`/`shift+↓` | Move selected section (in-memory reorder) |
| `Enter` / double-click | Open selected section in editor (commit gate first if pending) |
| single-click | Select row |
| `n` | New section (prompt → insert after selection, renumber) |
| `m` | Stub: "manuscript view — Plan C" |
| `esc` | Back to editor (commit gate first if pending) |

Within the commit gate: `y` apply · `n` discard · `esc` keep editing.
