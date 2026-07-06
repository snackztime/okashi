# Corkboard / Binder Unification — the manuscript navigator

**Date:** 2026-07-06
**Status:** Design approved in brainstorm (2026-07-06, all forks resolved); pending user review of
this doc before a plan.

## Goal

Collapse okashi's overlapping manuscript-structure surfaces — the file-pane chapter list, the
pop-down **binder** (`ctrl+k`), **structure mode** (`s`), and the **corkboard** (`c`) — into **one
concept, the corkboard, with two densities of a single dataset**:

- **Corkboard in the left pane** — the persistent, everyday navigator (browse, open, glance at
  synopses, reorder, edit a synopsis).
- **Corkboard full-screen** — the same data expanded for a planning pass (roomy cards, full
  synopses, the heavier structural edits).

**The corkboard IS the synopsis view.** There is no separate "synopsis mode"; synopses live only in
the corkboard (either density), never a third place. The plain chapter-title list is simply the
pane's *other* mode (files/list ↔ corkboard). This is Scrivener's model: binder on the left,
corkboard as the roomy spread — the corkboard *is* the binder + reorder + synopsis.

## Motivation

The "chapter list" exists in three places (pane, binder, corkboard) and reorder in two (structure
mode, corkboard), all reached as transient pop-downs. Adding a synopsis or reordering means leaving
the writing surface and hopping `ctrl+k` → `c`/`s`. The redundancy is the clunk.

## The left pane — files ↔ corkboard

For a **manifest manuscript** the pane toggles between two modes with **`ctrl+k`** (repurposed from
the retired binder — it has always meant "show me my chapters," now with synopses):

- **List mode** (default, = today): ordered chapter titles + per-chapter word counts.
- **Corkboard mode**: two sections, top to bottom —
  - **Chapters** — each a compact card: title + word count + a **2-line synopsis preview**
    (wrapped, clamped, `…` overflow). No synopsis → the chapter's **first prose line, dimmed**, as a
    fallback preview (never written; display only).
  - **Resources** — the manuscript's **subfolders as navigable groups** (`Characters/`,
    `Locations/`, …) plus loose unlisted `.md` at the manuscript root. Drill into a group to list/
    open its docs (existing pane cascade). No synopsis on resources.

Actions (sidebar-focused, selection on a chapter):
- `enter` / `l` / `→` — **open** the chapter (unchanged).
- `J` / `K` (`shift+↓`/`shift+↑`) — **reorder** (staged — see §Reorder). No-op on a Resource/group/`..`.
- `e` — **edit synopsis**: a small popup `textarea` seeded with the current synopsis; `esc` commits,
  writing `.okashi-synopsis.json` **immediately** (a discrete save, not a per-move write).
- `r` — retitle (manifest chapters retitle `items[].title`; unchanged).
- `c` — **expand to the full-screen corkboard**.

Non-manuscript folders (categories, loose roots): the pane stays a plain file tree — no corkboard
mode, no `J`/`K`/`e`/`c`. **Legacy** (numbered, manifest-less) manuscripts: corkboard mode renders
previews (first-line fallback) read-only — `J`/`K`/`e` are disabled with the existing
"not reorderable — no manifest" status (no manifest to write, no okashi-owned order to change).

## The full-screen corkboard — the structural spread

Reached with **`c`** (manuscript only). Roomy cards (title + word count + full multi-line synopsis),
windowed (O(visible)). It is the home for the heavier structural edits (absorbs structure mode):

- `↑`/`↓` select · `J`/`K` reorder (staged) · `e` synopsis (immediate popup)
- `a` **add**: a picker — new **chapter** (blank, optional synopsis) or promote an existing
  **Resource** to a chapter (structure mode's current add).
- `x` **remove**: demote a chapter to a Resource — the `.md` file is *not* deleted (non-destructive).
- `r` retitle · `enter` open the chapter (and return to editing) · `esc` back / apply-confirm.

Same dataset as the pane corkboard; either surface reflects the other on next render.

## Reorder / structure model — staged, confirmed on exit (keep the existing machinery)

Reordering and the structural edits (add / remove / retitle) are **staged in memory and committed
behind one confirmation** — the model structure mode already implements (`structure*` buffer +
`commitStructure`). We **keep** that machinery and re-surface it; nothing is written per keystroke.

- First `J`/`K` (or `a`/`x`) begins a staged edit; a **`-- REORDER --`** indicator shows.
- Continue rearranging freely (all staged, rendered live).
- `esc` → **"apply changes? y apply · esc discard"** → one atomic `writeManifest` (read-modify-write,
  reusing the manifest-first ordering from the Tier-3 `commitStructure` fix so a failed new-file
  create leaves a benign listed-but-missing entry, never an orphan).
- Available from **both** the pane (lightweight, chapters only: reorder) and the full-screen
  corkboard (full: reorder + add + remove + retitle).
- **Synopsis edits are exempt** — `e` writes the sidecar immediately on commit of that one edit
  (discrete, not a move stream); it never sets the manifest-dirty flag.

**Shared contract — unchanged, no HARD GATE.** okashi still reorders/edits structure behind a commit
confirmation (CLAUDE.md §1 stays accurate) and writes v1-shaped manifests atomically. No schema/
serialization/interaction-model change to reconcile with the companion app.

## Adding chapters vs resources — the `ctrl+n` prompt

In a **manuscript**, `ctrl+n` opens a small picker: **chapter** or **resource**.
- **Chapter** → create the `.md` and append to `items` **immediately** (a lone add doesn't need the
  reorder ceremony; atomic `writeManifest`), with an optional synopsis line.
- **Resource** → create an unlisted `.md`, either **loose** (manuscript root) or into a **resource
  folder**: pick an existing subfolder or type a new folder name (okashi creates it). No templates,
  no per-resource metadata.

Outside a manuscript, `ctrl+n` is unchanged (creates a file/folder in the current dir).

## Resources in the navigator (folded in)

Resources are surfaced as **user-made subfolders shown as groups** + loose docs — no auto-created
`Characters/`/`Locations/`, no metadata, no status fields (lean; those would drift toward the
Scrivener/PKM bloat okashi avoids). Mechanically this reuses okashi's existing folder support
(subfolders already navigable; a manuscript subfolder's `.md` are resources, not chapters, since
chapters are the manifest `items` at the manuscript root). The navigator just *presents* them under
a Resources heading and lets `ctrl+n → resource` file into one.

## Keybindings (the remap)

| Key | Before | After |
|---|---|---|
| `ctrl+k` | open pop-down binder | **toggle the pane: list ↔ corkboard** (manuscript only) |
| `c` (sidebar) | — | **expand to the full-screen corkboard** |
| `s` (binder) | structure mode | **retired** (folded into the full-screen corkboard) |
| `m` (binder) | pager | **`m` from the sidebar** opens the pager (read-through) |
| `e` (sidebar) | — | **edit synopsis** of the selected chapter |
| `J`/`K` (sidebar) | — | **reorder** the selected chapter (staged; manuscript only) |
| `ctrl+n` (manuscript) | new loose file | **prompt: chapter \| resource** (resource → loose or a folder) |
| `r`, `d`, `M`, `del`, `b`, `g`, `n` | sidebar file ops | unchanged |

`ctrl+l` (outline.md planning doc) and the inspector tabs are unchanged. F1 help updated (MANUSCRIPT
group + the `ctrl+n` prompt).

## Retire / keep

- **Retire:** `screenOutline` (the pop-down binder). `screenStructure` as a *standalone* modal —
  its logic merges into the full-screen corkboard (which becomes the structural surface). The
  staged `structure*` buffer + `commitStructure` are **kept** and reused (now enterable from the
  pane too); `enterCorkboard` sources the dir from `m.files.dir`, not `m.outline.dir`.
- **Keep:** the pager (`screenManuscript`, entered by `m` from the sidebar), the full inspector
  (Words/Outline/Goals/Analysis), the file-tree pane for non-manuscript folders, `ctrl+l` outline.md.

## Storage — unchanged

`manifest.json` (order + `items[].title` + `manifest.title`) and `.okashi-synopsis.json` (filename →
synopsis). Resources use existing folders/files — **no new file, no schema change**.

## Non-goals

No manifest schema change; no new sidecar. **No auto-created resource folders / templates; no
per-resource metadata or status fields** (lean). No PKM/tags. No sub-document sheets. Synopsis is one
string per chapter (1–3 lines), not a metadata record. No cross-manuscript board. No mouse-drag
reorder (keyboard `J`/`K`; native mouse stays for text selection).

## Edge cases

- Empty manuscript (`items: []`): corkboard shows Resources only / "(no chapters)"; `J`/`K`/`e` no-op.
- Selection on `..`, a Resource, a resource group, or a folder: `J`/`K`/`e` no-op (chapters only).
- Legacy (numbered, manifest-less): corkboard previews render read-only; `J`/`K`/`e`/`a`/`x` disabled.
- Unreadable/unsupported manifest: refuse structural edits (as today); files shown flat.
- Corkboard-vs-list pane mode persists per session (a model flag), defaulting to list.
- A staged reorder/add/remove with `esc`-discard leaves the manifest untouched (existing
  commitStructure discard path; already hardened to clear the staged buffer on exit).

## Tests outline

- Pane corkboard render: chapters show title + wc + 2-line synopsis; no-synopsis chapters show the
  dimmed first-line fallback; Resources section lists subfolders-as-groups + loose docs; windowed.
- `firstProseLine`: skips a leading `#` heading + blanks; "" for an empty file.
- Pane reorder: `J`/`K` **stages** (no disk write); `esc` → confirm → one manifest write with the new
  order; discard leaves the manifest unchanged; no-op on Resource/group/`..`; disabled on legacy.
- Pane synopsis edit: `e` → commit writes the sidecar immediately; blank clears; does not set
  manifest-dirty; prune-on-write intact.
- Full-screen corkboard: reorder/add/remove/retitle stage then commit atomically on confirm; `enter`
  opens + returns; empty-state renders.
- `ctrl+n` prompt: chapter → appended to `items` (+ optional synopsis); resource loose → unlisted at
  root; resource → folder → created/filed in the subfolder; outside a manuscript unchanged.
- Retirement: `ctrl+k` toggles the pane mode (no binder screen); `s` no longer enters structure mode;
  `m` from the sidebar opens the pager; `enterCorkboard` sources `m.files.dir`.
- Shared-contract: manifest still v1-shaped, atomic, byte-compatible with the companion app (no churn).

## Build order (for the plan)

1. **First-line fallback + corkboard-mode render in the pane** (`filelist.go`): `firstProseLine`,
   the Chapters section (title + wc + 2-line preview) + Resources section (subfolders-as-groups +
   loose), the `ctrl+k` list↔corkboard toggle. Tests.
2. **Pane chapter actions**: `J`/`K` staged reorder (reuse `structure*` buffer + `commitStructure`,
   with the `-- REORDER --` indicator + esc-confirm) surfaced from the pane; `e` synopsis popup →
   immediate sidecar write; open unchanged. Tests.
3. **Full-screen corkboard = structural spread**: rebuild `updateCorkboard` on the shared staged
   buffer, add `a`/`x`/`r`, source dir from `m.files.dir`, `enter` opens; keep the card view +
   windowing + the esc-confirm. Tests.
4. **`ctrl+n` chapter|resource prompt** + resource-folder targeting (pick/create subfolder). Tests.
5. **Retire** `screenOutline` (binder) + the standalone `screenStructure`: remove the binder screen,
   repoint `m` (pager) + `c` (corkboard) + reorder to the sidebar/pane, remove `s`. Update F1 help +
   sidebar status hints.
6. **Docs**: README (the manuscript navigator + corkboard + resources), CLAUDE.md (shipped-features;
   §1 stays as-is), the retired-keys note.

## Open decisions — all resolved (2026-07-06)

1. Corkboard = the synopsis view (one concept, pane + full-screen). ✓
2. Resources as folder-groups folded in; `ctrl+n` → chapter | resource (loose or folder). ✓
3. Staged reorder with confirm-on-exit (keep the existing staged machinery). ✓

A lone `ctrl+n → chapter` append writes the manifest immediately (only multi-move reorders stage +
confirm).
