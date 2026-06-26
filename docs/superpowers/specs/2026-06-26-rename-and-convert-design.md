# Rename + Convert-to-manuscript: design

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Context:** okashi can create files (`ctrl+n`) and, since long-form Plan A/B, detect
and outline manuscripts. It cannot **rename** anything, and it cannot turn a plain
folder of chapter files into a manuscript from the CLI — both currently force a trip
to Finder. This spec adds both, sharing one small set of rename helpers.

## Goal

Two related file/project operations, built on the existing name-prompt + `os.Rename`
+ backup machinery:

1. **Rename** (`r`) — rename the selected loose file, folder, or section title.
2. **Convert folder → manuscript** — number a plain folder's files so the outline
   turns on; folded into the existing `ctrl+l` affordance.

## Definitions / existing pieces reused

- `splitPrefix(name) (digits, rest)`, `padWidth(count, existingWidth)`, `renameOp{from,to}`,
  `applyRenames(dir, ops)` (two-phase, preflight-validated), `slugify(title)`,
  `sectionTitle(name)`, `projectTitle(name)`, `orderedSections`, `isManuscript`,
  `backupFiles(dir, stamp, paths)` / `backupStamp(time.Now())` — all already in the tree.
- The name prompt: `nameInput textinput.Model` plus a mode flag (the `creatingFile` /
  `outlineCreating` pattern). Rename adds a `renaming` mode.

## Section 1 — Rename (`r`), context-sensitive

`r` renames the **selected** item — "rename what you see". Available:
- In the **sidebar** when it has focus (skip the `..` parent row).
- In the **outline** (the selected section, or a loose file row).

Behaviour by selection type:

- **Numbered section** (`02-the-letter.md`): rename the **title only**. The prompt
  pre-fills with the current de-slugged title (`the letter`); on confirm the file is
  renamed keeping its numeric prefix and extension, with the new title slugified:
  `sectionRetitle("02-the-letter.md", "the telegram") → "02-the-telegram.md"`. The
  ordering/renumber system stays in sole control of the prefix.
- **Loose file** (`notes.md`): rename the **filename**. Prompt pre-fills with the
  current name; if the typed name has no extension, the original extension is kept
  (`looseRename`). Reject a name containing a path separator or `.`/`..` (same guard
  as `confirmCreate`).
- **Folder** (category or project, e.g. `book`): rename the **directory** to the
  typed name (same separator guard).

Rules for all three:
- **Collision-safe:** if the target name already exists in the directory, refuse with
  a status message — never overwrite.
- **Open-file tracking:** if the renamed file (or section) is the one open in the
  editor, `m.currentFile` updates to the new path.
- After a rename, the sidebar (`SetDir`) and, if active, the outline (`load`) refresh.
- A single rename is one atomic `os.Rename` and is trivially reversible (rename back),
  so it does **not** take a `.backup` snapshot. (Convert, which renames many files at
  once, does — Section 2.)

Pure helpers (package `main`, unit-testable):
- `func sectionRetitle(name, newTitle string) string` — keep prefix+ext, slugify title.
- `func looseRename(oldName, typed string) string` — keep original ext when omitted.
- (Folder rename needs no planner — the new name is the validated typed string.)

**Testing:** `sectionRetitle` keeps the prefix and extension and slugifies; `looseRename`
restores a missing extension and leaves an explicit one alone; a confirm-rename onto an
existing name is refused (target unchanged); renaming the open file updates `currentFile`.

## Section 2 — Convert folder → manuscript (via `ctrl+l`)

`ctrl+l` from the editor gains a branch:
- If `isManuscript(m.files.entries)` → open the outline (unchanged).
- Else if the current folder has **≥1 document file** (an `allowedDocExts` file, none
  numbered) → prompt **"Make this a manuscript? (y / n)"**. On `y`: snapshot all the
  folder's doc files to `.backup/<stamp>/`, then number them contiguously in their
  current (sidebar/alphabetical) order — keeping each existing name as the title —
  and open the outline. On `n`/`esc`: cancel, stay in the editor.
- Else (no doc files) → status "nothing to convert" (today's "not a manuscript" hint).

Numbering keeps the whole existing name as the title portion:
`planConvert([Chapter-00.md, Chapter-01.md], width=2) →
 [Chapter-00.md→01-Chapter-00.md, Chapter-01.md→02-Chapter-01.md]`.
Displayed title via `sectionTitle("01-Chapter-00.md")` = "Chapter 00". The user can then
reorder/retitle in the outline (Plan B) — `r` retitles, `J`/`K` reorders.

**Open-file tracking:** convert returns the old→new path map (as `commitReorder` does);
if the file currently open in the editor was among those numbered, `m.currentFile`
follows to its new `NN-` name.

Pure helper:
- `func planConvert(files []fileEntry, width int) []renameOp` — sibling of `planRenames`,
  but it **inserts** the `NN-` prefix + separator on unnumbered files
  (`fmt.Sprintf("%0*d-%s", width, i+1, e.name)`). Width = `padWidth(len(files), 0)`.
  Applied via the existing `applyRenames` (two-phase; here every op is a fresh `NN-` name
  so there are no collisions), after a `backupFiles` snapshot.

**Testing:** `planConvert` prefixes every file with a contiguous zero-padded number,
keeping the original name as the rest, and the result parses as a manuscript
(`isManuscript` true; `sectionTitle` de-slugs to the original name); converting a folder
on disk leaves a `.backup/<stamp>/` snapshot and numbered files; `ctrl+l` on a plain
folder-with-files raises the prompt and, on `y`, lands in the outline with the sections
numbered; `ctrl+l` on an empty/no-doc folder shows the "nothing to convert" status.

## Section 3 — Keys & prompt routing

- `r` — rename the selected item (sidebar focus, or outline). No-op on the `..` row.
- `ctrl+l` — outline if a manuscript; else the convert prompt (Section 2).
- The rename prompt reuses `nameInput` with a `renaming` mode and a captured
  `renameTarget` (the selected entry's path + kind), so typed input goes to the prompt
  (not nav keys), `enter` confirms, `esc` cancels — mirroring `creatingFile` /
  `outlineCreating`.
- The convert prompt is a small `y`/`n` confirm (a `convertPrompt` bool), not a text
  input.
- Status hints updated; `r` added to the writing-screen status string and the README
  keymap, and the outline status hint.

## Out of scope (non-goals)

- **Bulk multi-select rename** — one item at a time.
- **Smart number extraction** on convert (`Chapter-00` → `00`) — convert keeps the name
  as the title; retitling is the outline's job.
- **Undo** beyond the existing `.backup/` snapshot on convert and the inherent
  reversibility of a single rename.
- **Renaming the workspace root** or escaping the project — renames are confined to the
  current directory (reuse the `applyRenames`/`withinRoot` confinement for convert;
  the single-rename guards reject path separators).

## Risks

- **Rename touches real files.** Mitigations: collision check before `os.Rename` (no
  overwrite); path-separator guard; the open file's path is updated; convert snapshots
  to `.backup/` first and reuses the validated two-phase `applyRenames`.
- **Convert on a large folder** renames every file — it rides the same backup-protected
  `applyRenames` path as reorder and is covered by a test.
- **Tests stay hermetic** — `t.TempDir()` / `t.Setenv("OKASHI_DIR", …)`; pure helpers
  take values in.

## Build order

A single plan (these share one helper set and the prompt plumbing):
- Pure helpers (`sectionRetitle`, `looseRename`, `planConvert`) + tests.
- Rename wiring (`r` in sidebar + outline, prompt routing, collision/open-file tracking).
- Convert wiring (`ctrl+l` branch, y/n prompt, backup + number + open outline).
- Docs/status.
