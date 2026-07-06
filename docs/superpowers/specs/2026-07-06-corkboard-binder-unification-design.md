# Corkboard / Binder Unification — the manuscript navigator

**Date:** 2026-07-06
**Status:** Design approved in brainstorm (2026-07-06); pending user review of this doc before a plan.

## Goal

Collapse okashi's four overlapping manuscript-structure surfaces — the left file pane's chapter
list, the pop-down **binder** (`ctrl+k`), **structure mode** (`s`), and the **corkboard** (`c`) —
into **one coherent chapter/synopsis navigator with two views of a single dataset**:

- **Left pane = the persistent manuscript navigator** (everyday: browse, open, reorder, glance at
  synopses).
- **Full-screen corkboard = the structural spread** (planning pass: roomy synopsis cards + the full
  structural edits).

The corkboard *is* the binder + reorder + synopsis — not a separate thing you reach through the
binder. (Scrivener's model: binder on the left, corkboard as the roomy spread.)

## Motivation

Today the "chapter list" exists in three places (file pane, binder, corkboard) and reorder in two
(structure mode, corkboard), all reached as transient pop-downs. Adding a synopsis or reordering
means leaving the writing surface and hopping `ctrl+k` → `c`/`s`. The redundancy is the clunk.

## The two surfaces

### 1. Left pane — the manuscript navigator

For a **manifest manuscript**, the file pane gains a **synopsis-mode toggle** (`ctrl+k`, repurposed
from the retired binder):

- **Compact mode** (default, = today): ordered chapter titles + per-chapter word counts.
- **Synopsis mode**: each chapter renders as title + word count + a **2-line synopsis preview**
  (wrapped, clamped, `…` overflow). Below the chapters, a compact **Resources** section lists
  unlisted `.md` files (no synopsis, not reorderable).

Actions (sidebar-focused, selection on a chapter):
- `enter` / `l` / `→` — **open** the chapter in the editor (unchanged).
- `J` / `K` (`shift+↓`/`shift+↑`) — **reorder** the selected chapter down/up, written to the
  manifest **immediately + atomically** (see §Reorder model). No-op on a Resource or `..`.
- `e` — **edit synopsis**: a small popup `textarea` (reuse the Properties/notes idiom) seeded with
  the current synopsis; `esc` commits, writing the `.okashi-synopsis.json` sidecar immediately.
- `r` — retitle (already exists: manifest chapters retitle `items[].title`; unchanged).

Non-manuscript folders (categories, loose roots): the pane stays a plain file tree — no synopsis
mode, no `J`/`K`/`e`/`c`.

### 2. Full-screen corkboard — the structural spread

Reached with **`c`** from the sidebar (manuscript only). Roomy cards (title + word count + full
multi-line synopsis), windowed (O(visible)). It **absorbs structure mode** — one full-structure
surface:

- `↑`/`↓` select · `J`/`K` reorder · `e` synopsis (popup)
- `a` add a chapter (new blank **or** promote an existing Resource — structure mode's current add)
- `x` remove a chapter (demote to Resource — the file is *not* deleted)
- `r` retitle · `enter` open the chapter (and return to editing) · `esc` back

All writes are immediate + atomic (§Reorder model). This is the same dataset the left pane shows;
either surface reflects the other on next render.

## Reorder / structure model — immediate + atomic (no staged buffer, no confirm)

Retire the `structure*` staged buffer and the commit-confirm. Each structural action
(reorder / add / remove / retitle) is a **read-modify-write of `manifest.json` via the existing
atomic `writeManifest`**, applied on the spot:

- A reorder is a single-chapter move — trivially reversible (move it back); the manifest is always a
  complete, valid v1 document.
- `remove` only unlists a chapter (demote to Resource); the `.md` file is untouched — non-destructive
  and reversible via `add`/promote.
- `add` (new blank) creates the file then appends to `items` (ordering: **write the manifest entry
  after creating the file is the current `commitStructure` risk** — invert to *write manifest last*
  only where an orphan matters; a listed-but-missing entry is benign, an orphan file is not — reuse
  the resolved ordering from the Tier-3 `commitStructure` fix).

**Shared-contract note (no HARD GATE):** this changes okashi's *interaction* (immediate vs
confirm), not the manifest **shape** — okashi still writes v1-shaped manifests atomically. Update
CLAUDE.md §1: okashi reorders/edits structure **immediately** (no confirm); the companion app keeps
its own confirm sheet; both still read/write the one shared `manifest.json` (atomic writes +
`NSFileVersion`). No schema/serialization change, so the §1 HARD GATE is **not** triggered.

## Synopsis source — authored, with a first-line fallback

- The synopsis is **author-written** and stored in `.okashi-synopsis.json` (unchanged storage).
- When a chapter has **no** synopsis, the card/row shows the chapter's **first prose line**
  (skipping a leading `#` heading and blank lines) **dimmed**, as a preview — so nothing looks
  empty and you can still tell chapters apart. This is a *display fallback only*; it is never
  written to the sidecar and never becomes the "real" synopsis.
- **Authoring pathways:** `e` in either surface (primary); a **synopsis line on new-chapter
  creation** (the `a`-add / `ctrl+n`-in-a-manuscript prompt can accept an optional synopsis). No
  auto-derived synopsis — a summary is not the opening prose.

## Keybindings (the remap)

| Key | Before | After |
|---|---|---|
| `ctrl+k` | open pop-down binder | **toggle synopsis mode** in the left pane (manuscript only) |
| `c` (sidebar) | — (was binder→`c`) | open the **full-screen corkboard** |
| `s` (binder) | structure mode | **retired** (folded into the full-screen corkboard) |
| `m` (binder) | pager | **`m` from the sidebar** opens the pager (read-through) |
| `e` (sidebar) | — | **edit synopsis** of the selected chapter |
| `J`/`K` (sidebar) | — | **reorder** the selected chapter (manuscript only) |
| `r`, `d`, `M`, `del`, `b`, `g`, `n` | (sidebar file ops) | unchanged |

`ctrl+l` (outline.md planning doc) and the inspector tabs are unchanged. F1 help updated (the
MANUSCRIPT group).

## Retire / keep

- **Retire:** `screenOutline` (the pop-down binder) and `screenStructure` (the standalone reorder
  modal). Their entry points (`enterStructure`, the binder's `s`/`c`/`m` dispatch) and the
  `structure*` staged state + `commitStructure` staged commit are removed or reduced to the
  immediate writers. `enterCorkboard` now sources the dir from `m.files.dir`, not `m.outline.dir`.
- **Keep:** the pager (`screenManuscript`, now entered by `m` from the sidebar), the full inspector
  (Words/Outline/Goals/Analysis), the file-tree pane for non-manuscript folders, `ctrl+l` outline.md.

## Storage — unchanged

`manifest.json` (order + `items[].title` + `manifest.title`) and `.okashi-synopsis.json`
(filename → synopsis). Both already atomic, tolerant-load, okashi-owned. No new files, no schema
change.

## Non-goals

- No manifest schema change; no new sidecar. No PKM/tags. No sub-document sheets.
- Synopsis is one string per chapter (1–3 lines) — not a metadata record (no per-chapter status/
  label/POV fields; that stays a non-goal per the base scope).
- No cross-manuscript board. No drag-with-mouse reorder (keyboard `J`/`K`; native mouse stays for
  selection).

## Edge cases

- Empty manuscript (`items: []`): pane shows Resources only / "(no chapters)"; corkboard shows the
  empty-state; `J`/`K`/`e` no-op.
- Selection on `..`, a Resource, or a folder: `J`/`K`/`e`/`c` no-op (only chapters).
- Legacy (numbered, manifest-less) manuscript: read-only order (numeric prefix); **no reorder,
  no synopsis edit** (there's no manifest to write, and no okashi-owned ordering to change) — the
  pane shows synopsis-mode previews (first-line fallback) but `J`/`K`/`e` are disabled with a
  status note, matching today's "not reorderable — no manifest" gate.
- Unreadable/unsupported manifest: refuse structural edits (as today); show files flat.
- Synopsis-mode toggle persists per session (a model flag), defaulting to compact.

## Tests outline

- Pane synopsis-mode render: chapters show title + wc + 2-line synopsis; no-synopsis chapters show
  the dimmed first-line fallback; Resources listed without synopsis; windowed (O(visible)).
- First-line fallback: `firstProseLine` skips a leading `#` heading + blanks; returns "" for an
  empty file.
- Pane reorder: `J`/`K` writes the manifest immediately (assert on-disk order); no-op on Resource/`..`;
  disabled on a legacy manuscript.
- Pane synopsis edit: `e` → commit writes the sidecar; blank clears; prune-on-write intact.
- Full-screen corkboard: reorder/add/remove/retitle each write immediately + atomically; `enter`
  opens + returns; empty-state renders.
- Retirement: `ctrl+k` toggles synopsis mode (no longer opens a binder screen); `s` no longer enters
  structure mode; `m` from the sidebar opens the pager; `enterCorkboard` sources `m.files.dir`.
- Shared-contract: manifest still v1-shaped, atomic, byte-compatible with the companion app's
  serialization (no churn).

## Build order (for the plan)

1. **First-line fallback + synopsis-mode render in the file pane** (`filelist.go`): `firstProseLine`,
   synopsis-mode rows (title + wc + 2-line preview + Resources section), the `ctrl+k` toggle. Tests.
2. **Pane chapter actions**: `J`/`K` immediate-atomic reorder (manuscript-gated), `e` synopsis popup
   → sidecar write, open unchanged. Tests.
3. **Immediate-atomic structural writers**: refactor `commitStructure`/structure ops into
   `reorderChapter` / `addChapter` / `removeChapter` (demote) that each do one atomic `writeManifest`
   (reusing the manifest-first ordering fix); drop the staged buffer + confirm. Tests.
4. **Full-screen corkboard = structure spread**: rebuild `updateCorkboard` on the immediate writers,
   add `a`/`x`/`r`, source dir from `m.files.dir`, `enter` opens; keep the card view + windowing.
   Tests.
5. **Retire** `screenOutline` (binder) + `screenStructure`: remove the screens, repoint `m` (pager)
   to the sidebar, remove `s`, dead-code the staged `structure*` state. Update F1 help + the sidebar
   status hints.
6. **Docs**: README (manuscript navigator + corkboard), CLAUDE.md (shipped-features + §1
   immediate-reorder note), the retired-keys note.

## Open decisions (confirm at review)

1. **`ctrl+k` = synopsis-mode toggle** vs a different key (it was the binder; muscle memory may
   expect a "chapter overview"). Default: repurpose `ctrl+k` to the pane toggle; `c` opens the
   full-screen spread.
2. **Add-chapter home**: only in the full-screen corkboard (`a`), or also make `ctrl+n` in a
   manuscript append a chapter? Default: `a` in the corkboard only; `ctrl+n` keeps creating a
   Resource (birth-stable, un-surprising) — promote via `a`.
3. **Immediate reorder with no confirm** — confirm this is the desired divergence from the
   companion app's confirm model (default: yes, immediate; it's the point of a live pane).
