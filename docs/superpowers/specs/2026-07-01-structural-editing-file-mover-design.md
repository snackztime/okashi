# Structural manifest editing + file mover — design

**Date:** 2026-07-01
**Status:** Approved (direction)
**Context:** okashi is now a manifest *writer* (create + chapter-title retitle, no-confirm) with a
multi-source library. The remaining §0 promise is **structural** editing — reorder / insert /
remove / move — "planned behind a confirmation." This spec designs that, plus the **file mover**,
which performs the *same* structural edits whenever a move crosses a manuscript boundary. **One
shared foundation, two UIs.**

---

## 0. Authority — structural edits, confirm-gated (SHARED-CONTRACT update)

okashi now performs **structural** manifest edits (reorder / insert / remove / move), each gated
by a **confirmation**. This ships what the library spec's §0 called "planned behind a confirmation."

- **Safety** (unchanged model): atomic temp-then-rename + iCloud `NSFileVersion` + read-modify-write
  + byte-shape parity with wicklight (`JSONEncoder(.prettyPrinted,.sortedKeys)`: sorted keys, no
  HTML escaping, no trailing newline) — so the shared corpus doesn't churn. All via the existing
  `writeManifest`.
- **Bidirectional with wicklight** — both apps read/write the same `manifest.json`.
- **Scope:** **manifest manuscripts only.** Legacy numbered-prefix manuscripts stay read-only here
  (reordering them would mean renumbering files — a different mechanism, and they are a read-only
  transitional courtesy).
- **HARD-GATE (authority, not schema):** schema stays v1. Reflect the "planned → shipped
  (confirm-gated)" change in **both** repos (`../okashi/CLAUDE.md` §1 + inkmere `SPEC.md` /
  multi-source §4 / project-model §4). No wicklight code change expected; **verify** wicklight
  reloads an okashi structural write (file-presenter + per-source index rebuild).

---

## 1. Foundation — structural manifest ops (`manifest.go`)

Pure, testable helpers on top of the existing `readManifest` / `writeManifest`. The reorder /
insert / remove transforms are pure `items`-slice functions (no I/O); the move operations compose
them with the file system.

```go
// Pure item-slice transforms (no I/O) — return a new manifest, never mutate the argument.
func manifestInsert(m manifest, file, title string, at int) manifest // clamp at to [0,len]
func manifestRemove(m manifest, file string) manifest                // no-op if file absent
func manifestReorder(m manifest, file string, to int) manifest       // move item→to, clamp

// File-system moves that compose the transforms + writeManifest (atomic, read-modify-write).
func moveDocument(srcDir, file, dstDir string, asChapter bool) error
func moveFolder(srcDir, dstDir string) error
```

- **`moveDocument`**: relocate one document `file` from `srcDir` to `dstDir`.
  1. Refuse a no-op (`srcDir == dstDir`) and a name collision in `dstDir` (return an error; do NOT
     auto-rename — a silent rename would break manifest `items[].file` references).
  2. `safeMove(srcDir/file, dstDir/file)` — an `os.Rename` on the common same-volume path, with a
     copy-then-remove fallback for a cross-*source* move onto a different volume (§5).
  3. If `srcDir` is a manifest manuscript **and** `file` was a listed chapter → re-read src
     manifest, `manifestRemove`, `writeManifest(srcDir, …)`.
  4. If `dstDir` is a manifest manuscript **and** `asChapter` → re-read dst manifest,
     `manifestInsert(…, file, sectionTitle(file), len(items))` (append with a de-slugged title from
     the filename — the existing `sectionTitle` helper), `writeManifest(dstDir, …)`. Otherwise the
     file simply lands as a loose Resource in `dstDir` (no manifest write).
- **`moveFolder`**: `os.Rename(srcDir, dstDir/base(srcDir))`. Refuse a collision and moving a dir
  into itself or a descendant. A manuscript's `manifest.json` rides along inside the folder — no
  manifest edit.
- The pure transforms are exported so **both** structure mode and the mover reuse them; the file
  moves are the mover's engine.

## 2. Structure mode — `screenStructure` (full-page, entered from the binder)

- **Entry:** from the binder (`ctrl+k` chapter navigator), press **`s`** → structure mode for that
  manuscript. Manifest manuscripts only (else a status: "not reorderable — no manifest").
- **State (staged in memory):** `structureDir string`, `structureItems []manifestItem` (the working
  order/membership, copied from the manifest on entry), `structureSel int`, `structureAdds` (files a
  "new blank chapter" will create on commit), `structureDirty bool`.
- **Ops:**
  - `↑`/`↓` (`j`/`k`) — move the cursor.
  - `J`/`K` (or `shift+↑`/`shift+↓`) — move the selected chapter up/down in `structureItems`.
  - `a` — **add**: a small in-view pick — **new blank chapter** (record a create; a real
    `NN-untitled.md` is written on commit and inserted at the cursor) **or** **promote a loose
    Resource** (list the manuscript folder's unlisted `.md` files; the chosen one is inserted at the
    cursor).
  - `x` — **remove**: drop the selected chapter from `structureItems` (demote to Resource — the file
    on disk is untouched; it just leaves `items` on commit).
  - `r` — **retitle**: edit `structureItems[sel].Title` in the buffer (prefill the current title).
  - `esc` — if dirty → confirm; else exit.
- **Commit (on `esc`, if dirty):** confirm — *"Apply N changes to <title>? Rewrites manifest.json"*.
  On confirm: for each "new blank chapter" add, `atomicWrite` an empty file first; then
  `writeManifest(structureDir, manifest{schemaVersion:1, title, items: structureItems})` — a single
  atomic write. Read-modify-write: re-read the on-disk manifest's `title` immediately before writing
  (in case it changed externally) but `items` is authoritative from the buffer. Cancel → discard the
  buffer, exit with no write.
- **Render:** full-page, centered — `"<title> — structure"` header, one row per chapter
  (`NN  Title`) with a move indicator on the held/selected row (live preview of the buffer order);
  footer `J/K move · a add · x remove · r retitle · esc commit`.
- **Undo:** cancel (discard) is the coarse session undo. Per-op `u` is deferred (§5).

## 3. File mover — `screenMover` (two-pane full-page + confirm)

- **Entry:** (a) **contextual** — in the editor file pane, select a file or folder, press **`M`** →
  the mover opens with that item pre-filled as the source; (b) **standalone** — a key/home
  affordance opens the mover and you pick the source in the left pane.
- **Panes:** LEFT = the item being moved (name + its current container). RIGHT = a **destination
  browser**: the active source's containers (projects / folders / `◦ Loose`), **drillable** — reuse
  the home FILES cascade (`▸ name/`, enter drills, `‹ ..` up, bounded). A source picker (`▾`) lets
  the destination live in another library source.
- **Apply (confirm dialog),** operation chosen from source-type × dest-type:
  - **folder** (category or manuscript) → any folder / root → `moveFolder` (`os.Rename`).
  - **file** → a plain folder / root (not a manuscript) → `moveDocument(asChapter:false)` (plain
    move; if it was a chapter in its source manuscript, it is removed from that manifest).
  - **file** → a **manuscript** → the confirm offers **add as: ◉ chapter (end) · ○ resource**;
    chapter → `moveDocument(asChapter:true)` (append to `items`); resource →
    `moveDocument(asChapter:false)` (lands loose in the manuscript folder).
  - **chapter out** (source is a manuscript, item is a chapter) → covered by `moveDocument` removing
    it from the source manifest as part of any of the above.
  - **Cross-source** moves allowed (the confirm shows `source → source`); same atomic + manifest
    rules. Cross-volume falls back to copy-then-remove (§5).
  - **Refuse** a name collision in the destination (status, no move).
- **Render:** two framed panes side by side + a bottom confirm bar (with the chapter/resource radio
  when the dest is a manuscript); footer `↑↓ pick destination · enter move · esc cancel`. Standard
  render==hit-test cells on both panes.
- **After a successful move:** return to where invoked (file pane refreshed, or home).

## 4. Cross-cutting — safety, contract, testing

- **Safety:** all manifest writes atomic + read-modify-write + sorted-keys / no-HTML /
  no-trailing-newline parity (existing `writeManifest`); `NSFileVersion` is the concurrent-edit net.
  Every structural / boundary-crossing edit is confirm-gated (§0).
- **Contract (both repos):** flip `CLAUDE.md` §1 + inkmere docs from "structural edits planned
  behind a confirmation" → "shipped, confirm-gated (reorder / insert / remove / move)." Schema
  unchanged. Verify wicklight reloads an okashi structural write.
- **Tests:**
  - `manifestInsert/Remove/Reorder`: order/membership correct, no-op on absent file, index clamping,
    the argument manifest is not mutated.
  - `moveDocument`: loose→category (plain), loose→manuscript-as-chapter (insert), loose→manuscript-as-
    resource (no insert), chapter→category (remove + move), chapter→manuscript (remove src + insert
    dst), collision refused, round-trips through `readManifest`, sorted-key parity preserved.
  - `moveFolder`: category move, manuscript move (manifest rides along, still reads back), collision /
    move-into-descendant refused.
  - **structure mode:** enter loads `items`; `J/K` reorders the buffer; `a` new-blank creates+inserts
    on commit; `a` promote inserts an existing loose file; `x` demotes; commit writes once atomically;
    cancel discards (no write); a wicklight manuscript commit round-trips.
  - **mover:** each confirm branch; contextual + standalone entry; cross-source; render==hit-test on
    both panes.

## 5. Out of scope / deferred

- **Renaming a file during a move** (collision → refuse, not auto-rename). A rename-in-confirm could
  come later.
- **Cross-volume moves:** documents within one source share a volume (plain `os.Rename`). A
  cross-*source* move onto a different volume needs copy-then-remove; include a small `safeMove`
  helper in the foundation but keep the common same-volume path a rename.
- **Reordering legacy numbered manuscripts** (would renumber files).
- **Nested chapters / sub-documents** (manuscripts stay flat).
- **Per-op undo** in structure mode (`cancel` = coarse undo; `u` deferred).
- The wicklight **project overlay**.

## 6. Sequencing (for the plans)

1. **Foundation** — `manifestInsert/Remove/Reorder` + `moveDocument` + `moveFolder` (+ `safeMove`),
   plus the `CLAUDE.md` §1 + inkmere contract note. Pure/FS, fully unit-tested.
2. **Structure mode** (`screenStructure`) on the foundation.
3. **File mover** (`screenMover`) on the foundation.

Each is a plan-sized, independently shippable chunk on the shared writers.
