# Margin / Revision Notes — design spec

**Date:** 2026-07-04
**Status:** DRAFT — decisions pending (a user brainstorm finalizes the Open decisions before a
plan is written).

## Goal

Author annotations — revision notes, margin remarks, "fix this", "check the date" — attached to a
chapter or to a specific line/sentence, shown à la Tufte margin notes in the **right gutter of the
live editor** (primary) and, secondarily, in the `ctrl+p` Tufte preview. Notes live in an
**okashi-owned sidecar**, never in the `.md` body and never in the manifest, so the manuscript stays
clean plain text and no shared-contract gate fires.

This is the feature the Tufte preview's footnotes were **deliberately displaced for**: footnotes
were moved to bottom endnotes (`footnotesToEndnotes`) specifically to free the right margin, and a
set of dormant sidenote helpers (`sidenoteGeometry`, `sidenotePlan`, `layoutSidenotes`,
`footnotesToSidenotes`) were retained *with tests* to seed exactly this (see `main.go` §"NOTE: the
sidenote helpers below … DORMANT" and `2026-07-01-tufte-sidenotes-readme-scrub-design.md`).

## Non-goals

- **No inline markup in the `.md`.** No `<!--note-->`, no `[^rev1]`, no zero-width markers. The
  corpus stays byte-clean; the companion app must never be handed a body it would render as content.
- **No manifest change.** Notes are okashi-owned metadata → the §1 HARD GATE is **not** triggered.
- **No export by default** (see Export). Revision notes are drafting scaffolding, not prose.
- **No threaded comments / collaboration / @mentions / resolve-workflow.** One author, flat notes.
  (Lean discipline: this propagates to the companion app.)
- **No editor-core change.** The vendored `internal/textarea` gains at most one *read-only* query
  (see Rendering); margin compositing happens in `main.go`'s `View()`, outside the editor.

## Anchoring — the hard problem

A note attached to line 12 must survive an edit that inserts a paragraph at line 3. Raw line numbers
break on any edit above the anchor; that is disqualifying as the primary mechanism. Keep two axes
**separate** — they are orthogonal and phasing muddles if merged:

- **Mechanism** — how a note re-finds its spot: quote / context / line-hint.
- **Scope** — what a note attaches to: whole chapter vs a specific line/sentence.

### Strategies evaluated

| Strategy | Survives edits above? | Corpus clean? | Companion-app safe? | Verdict |
|---|---|---|---|---|
| Raw line number only | No — shifts on any insert/delete above | Yes | Yes | **Reject as primary** — keep only as a cached *hint*. |
| Text-quote + surrounding context, fuzzy re-locate on load | Yes (re-anchors to moved text) | Yes | Yes | **Recommended.** |
| Inline invisible/marker in the `.md` | Yes | **No** — pollutes plain text | **No** — companion renders it | **Reject.** Violates the ethos and the shared corpus. |
| Chapter-level notes only (no line anchor) | N/A — no line to lose | Yes | Yes | **Ship as v1 scope**; sentence/line anchoring is v2. |

### Recommendation

**Mechanism: text-quote + surrounding-context anchor, fuzzy-relocated on load, with the line number
kept only as a cached hint and graceful orphaning when the text is gone.** This is the standard
web-annotation model (a *quote selector*, robust to shifts, backed by a *position selector* used only
to disambiguate and to speed the search). Concretely, each line/sentence note stores:

- `quote` — the exact anchored text (the sentence, or the selection, or the whole source line).
- `prefix` / `suffix` — a bounded window (~32 chars) of surrounding context, to disambiguate when the
  quote recurs.
- `lineHint` — the source line index at write time. A **hint only**: used to bias the fuzzy search
  (check `lineHint` and its neighbors first) and as a last-resort fallback, never as the anchor.

**Re-location (once, on load / after an edit settles — never per frame):** scan the buffer for
`quote`, preferring the occurrence whose `prefix`/`suffix` and proximity to `lineHint` match best
(exact quote → exact; else a normalized/whitespace-tolerant compare; a small edit-distance tolerance
is an Open decision). The best match's current line index becomes the note's live `anchorLine`. If no
acceptable match exists, the note is **orphaned**: kept in the sidecar, shown in the Notes list
flagged "detached", not floated into the gutter until re-anchored (or deleted) by the author.

**Phasing:**
- **v1 — chapter-level notes** (scope = the whole file/chapter; mechanism trivially exact, no
  re-location needed). Delivers the sidecar, the storage, the Notes inspector tab, and the export
  decision immediately, at low risk.
- **v2 — line/sentence notes** with the quote+context+hint mechanism above and gutter floating in the
  editor. Reuses everything v1 built.

Rationale for chapter-first: it de-risks storage and UI without betting the release on fuzzy
re-location, and it matches okashi's lean bias — most revision notes ("this chapter drags") are
chapter-scoped anyway.

## Storage — sidecar (okashi-owned), atomic

**Location:** a per-project dot-directory `<project>/.okashi-notes/<base>.json`, one file per source
`.md` (`<base>` = the source filename without extension). The dot-dir is already excluded from the
file pane and manuscript detection (dotfile rule), and the dot-prefixed atomic temp
(`atomicWrite`) never surfaces as a document.

**Why not the manifest:** the §1 HARD GATE forbids touching the manifest *shape*; notes are transient
drafting metadata with a per-file cardinality that does not belong in the ordering file. Sidecar
keeps them okashi-private and trivially ignorable by the companion app and by `git`.

**Format** (v1 + v2 fields; v2 fields absent for chapter notes):

```json
{
  "schemaVersion": 1,
  "notes": [
    { "id": "n1", "scope": "chapter", "text": "This chapter drags — cut the flashback.",
      "createdAt": "2026-07-04T10:00:00Z" },
    { "id": "n2", "scope": "line", "text": "Check this date against ch. 3.",
      "quote": "It was the spring of 1911.",
      "prefix": "…the war had not yet come. ", "suffix": " Marguerite was thirty.",
      "lineHint": 42, "createdAt": "2026-07-04T10:01:00Z" }
  ]
}
```

- Tolerant load, mirroring `recent.json`: a missing/unreadable/`schemaVersion`-mismatched file →
  zero notes, never an error, never a crash.
- Writes go through `atomicWrite` (temp + rename, same volume); `MkdirAll` the `.okashi-notes` dir.
- **Rename coupling (must-do):** manifest chapters are filename-birth-stable, so their sidecar is
  safe. But **legacy/loose files and folders rename on disk** (the `r` handler). The rename path
  MUST move `.okashi-notes/<old>.json` → `.okashi-notes/<new>.json` (and the file **mover**,
  planned, must carry the sidecar across containers). Without this, notes silently orphan on rename.
  Delete of a source `.md` should likewise drop its sidecar.

## Rendering

### Primary surface: the live editor's right gutter

The editor pane is centered by `lipgloss.Place(editorArea, …, pane)` in `main.go`'s `View()`, and the
editor renders at the writing measure `m.colWidth`, leaving `editorArea - colWidth` of spare width on
the right. That spare band is the gutter — the exact geometry the dormant helpers already compute.

Two hard constraints, stated so the plan can't hand-wave them:

1. **The Decorator hook cannot do this.** `textarea.Decorator` only restyles existing rune ranges
   (spellcheck/syntax/grammar); it cannot add margin columns, and adding margin rendering inside
   `internal/textarea` would violate the windowed-`View()` contract and the editor-core scope
   invariant. Therefore the gutter is **composited in `main.go` around `m.editor.View()`**, at the
   same layer that already centers the pane — not inside the editor.

2. **Logical-line → in-window display-row mapping.** To place a note beside its anchor line we need
   the note's *display row within the current visible window*. The editor exposes
   `ViewportYOffset()` and its window-top math, but **no exported "display row of logical line N
   within the window"** query. This is the one genuine gap in the reuse story (see Open decisions):
   either add a small **read-only** exported helper on the editor (allowed — a query, not a
   behavior change) or derive it in `main.go` from window top + `memoizedWrap` counts.

**Reuse of the dormant helpers — precisely what:**

- `sidenoteGeometry(avail, measure)` and the gate/gutter math in `sidenotePlan` → **reuse directly**
  (min/max gutter [18,30], the "does a margin fit" gate). When the gutter doesn't fit (narrow
  terminal), notes do **not** float — they remain available in the Notes inspector tab (the
  narrow-fallback analog of Tufte's endnote fallback).
- `layoutSidenotes`' **cascade-packing + gutter composition** (notes anchored to a row, cascading
  downward so they never overlap, drawn after a `┆` divider) → **reuse, but refactored.**
  ⚠️ **Caveat that would otherwise sink the design:** `layoutSidenotes` currently derives each note's
  row by **scanning the body for its superscript marker** (`anchorOf` → `superscriptRuns`). Revision
  notes carry **no inline markers** by design, so that anchor step does not apply. Refactor
  `layoutSidenotes` to accept **explicit anchor rows** — e.g. `[]struct{ Row int; Text string }` —
  and keep only the packing + compositing. `footnotesToSidenotes` (which invents superscript markers)
  is **not** reused here; the anchor rows come from the anchoring pass instead.

**O(visible):** resolve every note's `anchorLine` **once** on file load and after an edit settles
(debounced, like grammar re-check) — never fuzzy-search the buffer inside `View()`. Per frame, take
only the notes whose `anchorLine` falls in the visible window, map those to display rows, and pack.
This keeps `View()` O(visible), honoring the performance invariant.

### Secondary surface: the `ctrl+p` Tufte preview

Nearly free — the preview already wraps to `colWidth` inside `previewAvail`, and `sidenotePlan`
already gates on spare width. Float chapter/line notes into the preview gutter using the same
refactored packer, anchoring line notes at their resolved rows in the rendered body. The preview is
read-only and a weak home for a *drafting* tool (you can't act on a note from a read view), so it is
**secondary**: nice for a wide-terminal review pass, not the primary workflow.

**Why the editor is primary despite being harder:** revision notes are consulted *while editing* —
you read "fix this date" and fix it in place. A read-only preview can't support that loop. The
preview path is cheaper (helpers almost ready) but low-value; the editor path is the point.

## Interaction

Reuse the existing **inspector** rather than a new full screen (lean bias; `ctrl+y` already cycles
inspector tabs). Add a **"Notes" inspector tab** listing the current file's notes (chapter notes
first, then line notes in anchor order, orphaned notes flagged "detached"). List/edit/delete/navigate
all live there; only *add* needs a global key.

- **Add note on current line/selection:** **`alt+n`** — verified free against both `helpText` (all
  `ctrl+`letters b/y/f/l/k/n/e/p/t/d/x/z/g/u/r/o are taken; byte-aliased `ctrl+i/m/j/[/h` are
  unusable) and `textarea.DefaultKeyMap` (`alt+f/b/d/c/l/u/</>` taken; `alt+n` is not). `main.go`'s
  key handler runs before the editor's `Update`, so it can intercept `alt+n` before the editor would
  insert it. Opens a small note-entry input (an inline `textinput`, like rename) seeded from the
  cursor's sentence/line as the `quote`; on commit, writes the sidecar and re-renders. A chapter note
  is added the same way with no anchor (Open decision: separate key/toggle, or a "scope: chapter"
  choice in the entry).
- **List / edit / delete:** in the Notes tab — `↑/↓` select, `⏎` edit (reopens the entry input),
  `del`/`d` delete (with okashi's confirm idiom). Editing a line note's text does not re-anchor;
  re-anchoring is automatic on load.
- **Navigate:** from the Notes tab, `⏎` on a line note jumps the editor to its `anchorLine`
  (`editor.MoveToLine`, the same primitive the outline/pager jumps use) and returns focus to the
  editor. `n` / `N` to hop next/previous note is an Open decision.
- **Detached notes:** shown flagged; `⏎` offers "re-anchor here" (attach to the current cursor line)
  or the author deletes it.

`helpText` gains a line: `alt+n    add revision note (ctrl+y → Notes tab)`.

## Export

**Excluded by default.** Revision notes are drafting annotations, not prose or citations — footnotes
already own the endnote channel in export (`footnotesToEndnotes` / export AST). Shipping notes into
RTF/PDF would leak "fix this date" into a manuscript handed to a reader. `ctrl+e` output is unchanged.

An optional **"review copy"** export (notes as a trailing appendix or as PDF margin notes) is a
plausible future nicety → Open decisions, off by default if ever built.

## Tests

Storage (`notes_test.go`):
- Sidecar round-trips (chapter + line notes); tolerant load (missing/corrupt/`schemaVersion`
  mismatch → zero notes, no error); atomic write leaves no `.`-temp behind.
- Rename moves the sidecar (`<old>.json` → `<new>.json`); delete drops it.

Anchoring (`notes_anchor_test.go`) — the heart:
- Exact re-anchor: insert a paragraph *above* the anchored line → note re-locates to the shifted
  line (proves line-hint is not load-bearing).
- Disambiguation: a `quote` that recurs → `prefix`/`suffix`/`lineHint` proximity picks the right one.
- Orphaning: the anchored text is deleted → note flagged detached, retained in the sidecar, not
  floated.
- Whitespace-tolerant match (if adopted): re-wrapped/re-spaced quote still anchors.

Rendering (`notes_render_test.go` / extend `preview_test.go`):
- Refactored packer: explicit anchor rows cascade without overlap; a note at row `r` lands at row
  `r` in the gutter; two adjacent-row notes don't collide (mirror `TestLayoutSidenotesCascadeNoOverlap`).
- Gutter gate: reuse `sidenoteGeometry` cases (avail 80/measure 72 → no float; avail 96 → gutter in
  [18,30]; ≥200 clamps to 30).
- Editor compositing: notes for lines outside the visible window are not drawn (O(visible)); the
  logical-line → display-row mapping is correct under typewriter and non-typewriter scroll.

Interaction (extend model tests):
- `alt+n` on a line opens entry, commits a line note seeded with the sentence as `quote`.
- Notes tab lists in order; `⏎` jumps the editor to `anchorLine`; delete confirms and removes.

## Open decisions

1. **Editor-line → window-display-row mapping:** add a read-only exported helper on
   `internal/textarea` (e.g. `DisplayRowOf(logicalLine int) (row int, visible bool)`), or derive it
   in `main.go` from `ViewportYOffset()` + wrap counts? This is the one honest gap in the reuse story.
2. **Fuzzy tolerance:** exact-quote + normalized-whitespace only (simplest, predictable), or add a
   bounded edit-distance so a lightly-typo-fixed sentence still anchors? Trade robustness vs surprise
   re-anchoring.
3. **Sentence vs whole-line anchoring** for line notes: seed `quote` from the cursor's sentence
   (reuses the dim-mode `cursorSentenceSpan`) or the whole source line? Sentence is finer but couples
   to sentence detection.
4. **Chapter-note entry:** a separate key, a scope toggle in the entry, or "note with no selection =
   chapter note"?
5. **Next/prev note hop** (`n`/`N` or similar) from the editor — worth a global key, or Notes-tab-only?
6. **Preview floating in v1**, or defer the preview surface to after the editor gutter ships?
7. **Optional "review copy" export** with notes as an appendix — build, or drop as YAGNI?
8. **v1 scope line:** ship chapter-notes-only first (recommended), or go straight to line anchoring?

## Build order (tasks, for the plan)

1. **Sidecar store** (`notes.go`): `note` + file schema, `loadNotes`/`saveNotes` (tolerant load,
   `atomicWrite`, `.okashi-notes` dir), path helpers + tests.
2. **Rename/delete/move coupling:** sidecar follows `r` rename, `del` delete, and the file mover.
3. **Notes inspector tab** (chapter notes, v1): list/edit/delete/navigate; `alt+n` add; `helpText`
   line; jump via `MoveToLine`.
4. **Anchoring pass** (`notes_anchor.go`, v2): quote+context+hint re-location on load/edit-settle,
   orphan flagging + tests. Populates each note's live `anchorLine`.
5. **Refactor `layoutSidenotes`** to take explicit anchor rows (drop the marker scan for this path);
   keep the cascade/composite. Reuse `sidenoteGeometry`/`sidenotePlan` gate.
6. **Editor gutter compositing** in `main.go` `View()`: resolve the line→display-row mapping (Open
   decision 1), float visible notes into the right band, O(visible).
7. **Preview floating** (secondary surface) using the same refactored packer.
8. Docs: README (Revision Notes; `alt+n`, Notes tab), CLAUDE.md shipped-features + env line if any.
