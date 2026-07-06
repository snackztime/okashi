# Outline mode — design spec

**Date:** 2026-07-06
**Status:** approved (brainstormed 2026-07-06)

## Summary

A full-screen, editor-first **outline mode** for okashi: a brainstorming surface where you draft
and rearrange beats and notes as plain Markdown, with a one-way bridge to **promote** a beat into a
manuscript chapter. The outline is *pre-structure ideation*; the corkboard remains the *manuscript
structure* surface. They are separate workflows connected only by promote — **no two-way sync.**

## Why (the division of labor)

The outline and the corkboard are **not** two views of one dataset. The outline legitimately holds
things the manuscript never will — un-promoted beats, brainstorming notes, groupings ("Act I"),
ideas you'll cut or merge as *thinking*. A chapter can also exist with no beat. The sets only
partially overlap, so a live two-way binding would collapse the outline into a mirror of the
corkboard and destroy its value as a *looser-than-structure* scratchpad. The only coupling is a
single fire-and-forget action: promote a beat → create a chapter.

## Decisions (locked in brainstorming)

1. **Two-level model:** a top-level bullet is a **beat**; its indented sub-bullets are **notes**.
2. **Store:** the existing per-folder `outline.md`, plain Markdown (`- beat`, `  - note`). Grep-able,
   no lock-in. The inspector's existing read-only Outline tab (`readOutlineDoc`) keeps working.
3. **Promote target:** the current manuscript. Promote appends a chapter to *this* folder's
   `manifest.json` and seeds its synopsis from the beat's notes.
4. **After promote:** the beat is marked `- [x] <title>` (GFM task item) — non-destructive, keeps the
   outline a living map, and blocks double-promote. No back-sync.
5. **Feel:** editor-first — you type freely (real editor, list-continuation); structural *commands* act
   on the bullet structure.
6. **v1 scope:** outliner **and** promote together.

## Architecture

### Entry / exit
- **`ctrl+l` opens the full-screen outline mode** (`screenOutline`), replacing today's inline
  `outline.md` toggle — a clean parallel to `ctrl+k` → corkboard.
- On entry: `m.save()` the current buffer, load the current folder's `outline.md` (create it seeded
  `"- "` if missing), record the return file (reuse `outlineReturnFile`), set `screen = screenOutline`.
- On `esc`: save `outline.md`, reload the manifest/pane (so promoted chapters appear), load the return
  file, `screen = screenWriting`.

### Editor reuse
Reuse the vendored `internal/textarea` editor (`m.editor`) — real typing, list-continuation, measure
width. Focus dimming is **off** in outline mode (bullets, not prose). A dedicated `updateOutline`
handles the structure keys + `esc` and delegates all other keys to `m.editor.Update`. `updateOutline`
also handles `tea.WindowSizeMsg`; other non-key messages are ignored (mirrors `updateCorkboard`).
`outline.md` is saved on `esc`, after each structural op, and on the editor's existing idle-autosave
(the autosave path checks `screen == screenOutline` and writes `outline.md`).

### Components / files
- **`outline.go`** (rework): `screenOutline` handling — `enterOutline`, `updateOutline`, `outlineView`,
  and the structure ops. (Keeps the existing `splitPrefix`/`projectTitle`/`slugify`/`readEntries`
  helpers that already live here.)
- **`outline_parse.go`** (new): the pure two-level parser + block helpers (no Bubble Tea deps, fully
  unit-testable).
- **`main.go`**: `screenOutline` constant; route `screen == screenOutline` to `updateOutline`/
  `outlineView`; repoint the `ctrl+l` case to `enterOutline`; the autosave tick saves `outline.md`
  when in outline mode.
- **Tests:** `outline_parse_test.go` (parser + move), `outline_test.go` (promote + wiring).

## The two-level parser (`outline_parse.go`)

```
type outlineBlock struct { start, end int } // [start,end) line indices of a beat + its notes
```

- `beatBlocks(lines []string) []outlineBlock` — a **beat** starts at a top-level list marker
  (`-`/`*`/`+` followed by a space, at indent 0). A block runs from its beat line up to (not
  including) the next top-level marker or EOF. Lines before the first beat = **preamble** (not part of
  any block).
- `blockAt(lines []string, line int) (outlineBlock, bool)` — the block containing `line`; `ok=false`
  in the preamble.
- `beatTitle(line string) string` — strip the leading marker, an optional `[ ]`/`[x]` task box, and
  surrounding space. `beatIsPromoted(line string) bool` — true iff the beat carries `[x]`.
- `beatNotes(lines []string, b outlineBlock) []string` — the block's non-beat lines, each stripped of
  its leading indent + marker, blanks dropped.

## Structure ops

### Move beat block — `alt+↑` / `alt+↓`
Find `blockAt(cursorLine)`. Swap the whole block's line range with the adjacent block; keep the cursor
on the moved beat's title line; rewrite the editor value. No-op with a status nudge when in the
preamble or when there is no neighbor in that direction.

### Promote beat — `alt+↵` (with `ctrl+↵` accepted where the terminal distinguishes it)
Acts on `blockAt(cursorLine)`. Guards (each → clear status, no write):
- not on a beat (preamble),
- already promoted (`beatIsPromoted`),
- empty title,
- current folder has no `manifest.json` (non-manuscript — promote needs a manuscript).

On success (`promoteBeat` in `outline.go`, using existing primitives):
1. `file := uniqueChapterFile(dir, taken)` — okashi's birth-stable convention (`untitled.md`,
   `untitled-2.md`, …). **The filename is deliberately non-descriptive; the title lives in the
   manifest.** `taken` = the manifest's current files.
2. `atomicWrite(dir/file, []byte(""), 0644)` — an empty chapter body (matches `createChapter`).
3. Read `manifest.json`, append `manifestItem{File: file, Title: beatTitle}`, `writeManifest` (atomic;
   v1 shape — no schema change).
4. Seed the synopsis: write `beatNotes` joined by `\n` into `.okashi-synopsis.json` for `file`
   (via `saveSynopses`, pruned against the manifest's chapter set). Skip if there are no notes.
5. Rewrite the beat's title line to `- [x] <title>` (preserve the original marker char + indent).
   Save `outline.md`.

Because filenames are non-descriptive by convention, the mark is a **checkbox only** — no "→ file"
arrow (it would just read "→ untitled.md"). The manifest/corkboard show the real title everywhere.

## Chrome (`outlineView`)
Full-screen, sidebar hidden. Header `OUTLINE · <folder title>`. Editor body at the measure width, as in
the writing view. Footer: `alt+↑/↓ move beat · alt+↵ promote · esc done`. Promoted `[x]` lines render
as raw Markdown in v1 (no special styling).

## Error handling
- `outline.md` unreadable/uncreatable on entry → status note, stay on the writing screen.
- Every guard above fails safe (status, no partial write).
- All writes atomic (`atomicWrite`/`writeManifest`/`saveSynopses`).
- Promote is idempotent per beat via the `[x]` guard; a stale `[x]` (chapter later deleted in the
  corkboard) is harmless — it's a text record, not a live link.

## Testing
- **Parser (`outline_parse_test.go`):** `beatBlocks` boundaries (multiple beats, notes attach to their
  beat, preamble excluded, `*`/`+` markers, trailing notes at EOF); `beatTitle`/`beatIsPromoted`/
  `beatNotes` (task box stripped, indents/markers stripped, blanks dropped); move-beat (swap up/down
  correct, cursor follows, no-op at edges + in preamble).
- **Promote + wiring (`outline_test.go`):** promote creates the file, appends the manifest in order
  with the beat title, seeds the synopsis from notes, and marks `[x]`; guards for double-promote /
  non-manuscript / empty title; `ctrl+l` enters `screenOutline` and creates `outline.md` when absent;
  `esc` returns to the prior file and the manifest reflects the new chapter.

## Non-goals / scope guard
No two-way sync. No nested outliner beyond two levels. No phantom cards. No manifest **shape** change
(writes v1-shaped manifests only). No new dependency. One cohesive subsystem → one spec + one plan.

## Shared-contract note
Promote writes `manifest.json` (append) and `.okashi-synopsis.json` — both already okashi-owned
writers using the v1 manifest shape. **No schema/shape change**, so the shared-contract HARD GATE is
untriggered. Filenames stay birth-stable, matching the companion app's expectations.
