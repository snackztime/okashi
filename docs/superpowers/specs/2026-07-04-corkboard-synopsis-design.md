# Corkboard / Synopsis — design spec

**Date:** 2026-07-04
**Status:** DRAFT — decisions pending (a user brainstorm finalizes the Open decisions before build).

## Goal

Give each chapter of a manuscript a short (1–3 line) **synopsis**, and a **corkboard** view that
shows every chapter's title + synopsis as a scrollable card list — so the writer can see and
reorder the manuscript by *idea*, not by filename. This is the Scrivener-signature feature, done
lean: one synopsis per existing chapter file, nothing more.

The corkboard is a card-rendered presentation of **structure mode's staged buffer** (see §UI). It
inherits structure mode's reorder + commit for free, and adds one action: edit the selected
chapter's synopsis inline.

## Non-goals

- **No manifest schema change.** Synopses live in an okashi-owned sidecar, NOT in `manifest.json`
  (the shared-contract HARD GATE stays untriggered — see §Storage).
- **Not a note system.** No tags, no free-floating cards, no cards without a backing chapter file.
  A synopsis exists iff its chapter does. This is a manuscript board, not PKM.
- **Synopsis ≠ `outline.md`.** okashi already has two "outline" surfaces: the `ctrl+l` planning-notes
  document (`outline.md`) and the inspector **Outline** tab (which renders that doc read-only). The
  per-chapter synopsis is a distinct, structured, per-file datum — do not conflate them.
- **Not a second manifest writer.** The board must not open a new reorder+commit path; it reuses
  structure mode's single `writeManifest` read-modify-write commit (see §UI, §Open decisions).
- No synopsis for loose files / Resources / categories in v1 — corkboard is manuscript-only, cards
  are the ordered chapters (`items`).

## Storage — okashi-owned sidecar

A single per-manuscript sidecar: **`<project>/.okashi-synopsis.json`**.

- **Name:** dot-prefixed, so it is already excluded from the file pane and from manuscript
  detection (`readEntries`/`SetDir` skip `.`-names) and never appears as a document. Distinct from
  `.okashi.json` (project settings, owned by the Properties spec) — no collision.
- **Shape:** a flat map keyed by bare chapter **filename** → synopsis string.

```json
{
  "schemaVersion": 1,
  "synopses": {
    "01-the-letter.md": "Marceau finds the unsent letter in his mother's coat.\nHe decides to read it.",
    "02-the-station.md": "The train is late. A stranger asks for the time."
  }
}
```

- **Keyed by filename — and it is safe because filenames are birth-stable.** A manifest chapter's
  filename never changes: retitle edits `items[].title`, not the file (manifest.go
  `renameChapterTitle`). So a key that matches a chapter today keeps matching it. Newlines in the
  value carry the 1–3 line shape.
- **Writes are atomic** via `atomicWrite` (temp + rename), same as `writeManifest` / `save` /
  export. Serialize with 2-space indent, `SetEscapeHTML(false)`, trailing newline trimmed — mirror
  `writeManifest` so the file is diff-legible if it ever churns.
- **Tolerant load** (mirrors `recent.json`): a missing file, unreadable JSON, or an unsupported
  `schemaVersion` → treat as "no synopses," never an error, never a crash. The board still renders;
  cards just show an empty synopsis line.
- **Orphan handling — lazy prune-on-write, never hook rename/mover/delete.** Some paths can leave a
  key whose chapter no longer exists: a chapter removed from the manifest (→ Resource), a deleted
  file, or the *planned* cross-container move. We do NOT wire the sidecar into `rename.go` /
  `mover.go` / delete. Instead:
  - On **read**, keys with no matching current chapter are simply ignored (harmless).
  - On **write** (any synopsis edit commit), re-derive the current chapter set from the manifest and
    **drop keys not in it** before serializing. This self-heals orphans opportunistically without
    coupling to every mutation site.
  - A chapter renamed *on disk* by an external tool loses its synopsis (acceptable: okashi itself
    never renames a manifest chapter's file; birth-stability is the contract).
  - **Caveat (pin this):** prune-against-on-disk-manifest is safe *only because the corkboard is
    reorder-only* — the staged buffer permutes chapters but never changes the chapter SET, so the
    live set equals the on-disk manifest set all session. If the board ever gains add/remove (or
    synopsis editing becomes reachable from structure mode proper, which has `a`/`x` +
    `structurePendingNew`), prune MUST run against **on-disk ∪ staged**, not on-disk alone —
    otherwise a just-typed synopsis for a pending-new chapter is silently dropped.

## UI — corkboard as an enhanced structure mode

**Entry key: `c`** ("corkboard"), from the **binder** (`screenOutline`), a sibling of `s`
(structure) and `m` (read). Verified free: the binder handler (`updateOutline`) uses only
up/down/j/k, enter, r, m, s, ctrl+e, esc; no global handler consumes `c` before screen dispatch
(the home/writing global switch has no `c` case). Manuscript-manifest projects only — a
legacy/absent/unreadable manifest shows the existing `not reorderable — no manifest` status
(same gate as `enterStructure`). Add `c   corkboard (from binder)` to `helpText`.

**Relationship to structure mode (the load-bearing decision):** the corkboard **is** structure
mode with a card layout + a synopsis-edit action. It reuses the exact staged-buffer machinery:
`m.structureItems` (the reorderable `[]manifestItem` buffer), `m.structureDir`,
`m.structureSel`, `m.structureDirty`, `m.structurePendingNew`, and — critically — **the same
`commitStructure`** on exit. Reorder-by-idea (the Scrivener signature) therefore comes for free
from the existing manifest reorder + commit-confirm; there is exactly **one** manifest write path.

Concretely, add a boolean `m.structureCork` (render as cards vs the plain list) rather than a new
screen, OR a thin `screenCorkboard` that shares all `structure*` state and delegates commit to
`commitStructure`. Either way the writers are unchanged. (Which of these — see Open decisions.)

**Layout** — a windowed vertical list of cards, each card 3–5 rows, one per staged chapter, in
manifest order:

```
╭ 03 · The Station ─────────────────────────────╮
│ The train is late. A stranger asks for the     │
│ time. Marceau lies about why he's traveling.   │
╰────────────────────────────────────────────────╯   412w
```

- Card header: zero-padded ordinal (`fmtNum`, as structure.go) + chapter title.
- Card body: the synopsis, wrapped to the card inner width, clamped to ~3 lines (overflow → `…`).
- Right-aligned per-chapter word count (reuse `wc.count`, as outline.go / filelist `sectionRow`).
- Selected card is highlighted (`selectedStyle`), same idiom as structure.go.
- Reuse `framedPanel` for the card frame (title in the top border), centered with `lipgloss.Place`,
  matching structure.go's chrome.

**Navigation & actions** (mirror structure.go so the two modes feel identical):
- `↑`/`↓` / `k`/`j` — move selection.
- `J`/`K` (shift+down/up) — reorder the selected chapter up/down in the staged buffer
  (`structureItems` swap + `structureDirty = true`), identical to structure.go.
- `e` (or `⏎`) — **edit the selected chapter's synopsis** inline (see below).
- `esc` — if `structureDirty` (order changed), the existing commit-confirm (`y apply · esc cancel`)
  writes the manifest via `commitStructure`; synopsis edits are committed to the sidecar
  independently at edit time (below). With no order changes, exit straight to the binder.
- Footer hint: `J/K move · e synopsis · esc back`.

**Inline synopsis editing** — reuse the vendored `internal/textarea` (small, ~3 rows), the same
component the Properties contact field uses:
- `e`/`⏎` focuses a small textarea seeded with the current synopsis, shown over/under the selected
  card. Keys flow to the textarea; `esc` commits (so `⏎` can insert the 1–3 line breaks).
- On commit: update an in-memory `map[string]string` synopsis buffer keyed by the chapter's
  filename, then **write the sidecar immediately** via the atomic writer with prune-on-write. This
  keeps synopsis persistence independent of the manifest commit — a writer can jot synopses without
  ever reordering (no confirm), and reordering never risks the synopsis data.
- Editing does not set `structureDirty` (that flag governs the *manifest* commit only).

## Rendering / perf — windowing (O(visible))

A 400-page work has 40–100 chapters; cards are 3–5 rows, so the full board is far taller than the
viewport. `View()` MUST stay O(visible):
- Compute visible card capacity from `m.height` and the fixed per-card row count.
- Use `homeWindowOffset(len(items), sel, visibleCards)` (the same helper structure.go uses) to pick
  the window start so the selected card is always shown; render only `items[off : off+visibleCards]`.
- Never build cards for off-screen chapters. Word counts come from the existing `wordCountCache`
  (already O(1) amortized). Synopses are read once from the in-memory buffer, not re-parsed per frame.
- Keep lipgloss out of the hot path: the card frame styling is per-visible-card only, never
  per-chapter-in-manuscript.

## Tests outline

`synopsis_test.go` (storage):
- Sidecar round-trip: `saveSynopses`/`loadSynopses` write and read back the map; atomic write leaves
  no `.`-temp file behind.
- Tolerant load: missing file → empty map, no error; corrupt JSON → empty map, no error;
  unsupported `schemaVersion` → empty map, no error.
- Prune-on-write: a sidecar with a key not in the current manifest chapter set is dropped on the next
  save; keys for live chapters survive.
- Filename-key stability: a manifest retitle (`renameChapterTitle`) leaves the synopsis key matching
  (proves birth-stable keying).

`corkboard_test.go` (UI/wiring):
- `c` from the binder enters the corkboard for a manifest manuscript; a legacy/no-manifest dir gets
  the `not reorderable` status and stays on the binder.
- Reorder on the board (`J`/`K`) then commit writes the manifest via the shared `commitStructure`
  path (assert order in the on-disk manifest) — and does NOT write a second/duplicate path.
- Synopsis edit commit writes the sidecar and is visible on reload; it does not set the manifest
  dirty flag and does not require the order-commit confirm.
- `View()` windows: with N cards taller than the viewport, only the visible window renders and the
  selected card is on-screen (offset via `homeWindowOffset`).

## Open decisions (finalize in brainstorm)

1. **Sidecar vs frontmatter.** *Default: sidecar* (`.okashi-synopsis.json`). Frontmatter in each
   `.md` pollutes the plain-markdown corpus and forces the companion app to parse/round-trip YAML on
   every chapter; a sidecar keeps the `.md` clean and the synopsis okashi-owned. Decide: confirm
   sidecar, or reconsider frontmatter if the companion app wants synopses in the files.
2. **okashi-only vs shared.** *Default: okashi-only for v1.* If the companion macOS app should read
   synopses, the sidecar becomes a **shared contract** (schema + location + serialization) and this
   triggers the cross-repo coordination gate (like `manifest.json`). Flag: do we want the app to
   share this eventually? If yes, the sidecar schema should be spec'd jointly before we ship, even if
   the app doesn't read it day one.
3. **Corkboard vs structure-mode relationship.** *Default: the corkboard IS structure mode* (shared
   staged buffer + `commitStructure`), a card presentation with a synopsis-edit action. Alternative:
   a separate read-mostly board that hands off to `s` for reordering (rejected as default — reorder
   by idea is the whole point). Sub-decision: a `structureCork` flag on the existing screen vs a thin
   `screenCorkboard` that shares `structure*` state.
4. **Edit-in-place vs a detail view.** *Default: inline textarea over the selected card* (matches the
   Properties contact-field idiom, keeps the board as the single surface). Alternative: press `⏎` to
   open a per-chapter detail pane (title + synopsis + stats) — heavier, deferred unless the brainstorm
   wants it.
5. **Synopsis persistence timing.** *Default: write the sidecar immediately on each synopsis-edit
   commit* (independent of the manifest commit-confirm). Alternative: stage synopsis edits and flush
   them in the same commit as the reorder. Default chosen so a writer can jot synopses without ever
   entering a reorder/confirm flow.

## Build order (tasks, for the plan)

1. `synopsis.go`: `type synopsisFile struct{ SchemaVersion int; Synopses map[string]string }`,
   `synopsisPath(dir)`, tolerant `loadSynopses(dir)`, atomic `saveSynopses(dir, m, chapterSet)` with
   prune-on-write. Tests (`synopsis_test.go`).
2. Corkboard state + entry: `structureCork` flag (or `screenCorkboard`) sharing `structure*` state;
   `enterCorkboard` (manifest gate, load staged buffer + synopsis buffer); `c` in the binder handler;
   screen dispatch in `Update`/`View`.
3. Card `View()` with windowing (`homeWindowOffset`, `framedPanel`, word counts); footer hints.
4. Navigation + reorder (reuse structure.go `J`/`K` + `commitStructure` on exit) + inline synopsis
   textarea edit → immediate atomic sidecar write. Tests (`corkboard_test.go`).
5. Docs: `helpText` (`c   corkboard`), binder status line (`… c corkboard …`), CLAUDE.md shipped-
   features + a note that synopses live in an okashi-owned sidecar (not the manifest). If Open
   decision #2 flips to shared, add the sidecar to the SHARED CONTRACTS block and coordinate the app.
