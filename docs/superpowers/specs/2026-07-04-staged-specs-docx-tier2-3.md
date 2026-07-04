# Staged specs — DOCX export + Tier 2 + Tier 3

**Date:** 2026-07-04
**Status:** Roadmap-with-specs. From the 2026-07-03 full review. Built in stages; each stage gets a
full plan just-in-time (Stage 1 first). Stage 1 (DOCX) is specced to build level here; later stages
are design-level with **DECISIONS** flagged — confirmed when that stage is scheduled.

The export AST (`export_ast.go`) is style-agnostic: `ManuscriptDoc []Section`, `Section{Title,
Blocks}`, `Block` ∈ {`Paragraph{Runs}`, `Heading{Level,Runs}`, `Blockquote{Children}`,
`List{Ordered,Start,Items}`, `SceneBreak`, `Endnotes{Items []Endnote}`}, `Run{Text,Bold,Italic}`,
`Meta{Author,Title}`, `ExportStyle` ∈ {`StyleManuscript`, `StyleTufte`}. `writeRTF`/`writePDF`
consume it. DOCX + the title page reuse the SAME AST.

---

## Stage 1 — DOCX export (Tier-1 #3) — BUILD-LEVEL

**Why:** `.docx` is the format agents/editors actually request; okashi exports only RTF+PDF today.
Pure-Go, no cgo (OOXML is zipped XML).

**Design:** `func writeDOCX(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error)` in a new
`export_docx.go`, mirroring `writeRTF`. A `.docx` is a zip of OOXML parts; build it with
`archive/zip`:
- `[Content_Types].xml` — declares the document + rels content types.
- `_rels/.rels` — root relationship to `word/document.xml`.
- `word/document.xml` — the body: `<w:document><w:body>…<w:sectPr>` (page size Letter 12240×15840,
  1" margins 1440 twips).
- (Optional) `word/styles.xml` — a default style; v1 can inline formatting on each paragraph/run
  and skip a styles part.

**AST → OOXML mapping** (each `Section` starts on a new page via a page break on its title para):
- `Section.Title` → a centered, bold paragraph, on a new page (`<w:pPr><w:pageBreakBefore/>`) except
  the first.
- `Paragraph` → `<w:p>` with body run(s); Manuscript style: first-line indent 0.5" (`<w:ind
  w:firstLine="720"/>`), double spacing (`<w:spacing w:line="480" w:lineRule="auto"/>`).
- `Heading{Level}` → bold paragraph, larger size by level.
- `Blockquote` → left-indented italic paragraphs.
- `List` → one paragraph per item, prefixed `"1. "`/`"• "` (v1: literal prefix text, not native
  numbering — keeps it simple and matches the RTF degrade).
- `SceneBreak` → a centered `#` paragraph.
- `Endnotes` → a "Notes" bold heading then one numbered paragraph per item (per-chapter endnotes,
  matching RTF).
- `Run{Text,Bold,Italic}` → `<w:r><w:rPr>[<w:b/>][<w:i/>]<w:rFonts .../><w:sz w:val="24"/></w:rPr>
  <w:t xml:space="preserve">ESC(Text)</w:t></w:r>`; XML-escape `& < > "`.
- Font per style: Manuscript = `Times New Roman` (Shunn accepts Times or Courier; Times is the
  common modern default); Tufte = `Georgia`. Size 12pt = `w:sz w:val="24"` (half-points).

**Wire into export:** `runExport` (`export.go`) currently writes `<slug>.rtf` + `<slug>.pdf`. Add
`<slug>.docx` via `writeDOCX` in the same flow (atomic write), so every export emits all three;
update the status to `exported <slug>.rtf + .pdf + .docx to export/`. No new prompt key.

**Tests (`export_docx_test.go`):** `writeDOCX` returns bytes that `archive/zip.NewReader` opens;
`word/document.xml` exists and contains the chapter title + body text; a bold run emits `<w:b/>`; a
scene break emits a centered `#`; XML-escaping turns `&` into `&amp;`; `[Content_Types].xml` and
`_rels/.rels` are present (a minimal-validity check — Word opens it).

**Tasks (for the plan):** (1) `writeDOCX` + OOXML helpers + tests; (2) wire into `runExport` +
wiring test; (3) update README (DOCX now exported; soften the RTF "standard format" line).

---

## Stage 2 — Undo / redo — DESIGN

**Why:** the vendored `internal/textarea` has NO undo stack; a bad edit + autosave is currently
unrecoverable (also a Tier-0-adjacent safety gap).

**Design (lean checkpoint ring, not char-level):** model fields `undoRing []string`, `undoPos int`.
Push the current buffer onto the ring **before** each mutation that could lose text — on `loadFile`
(no, that's a switch), on each **autosave tick when the buffer changed**, and before each
spell/grammar bulk apply. `ctrl+z` restores the previous checkpoint (`editor.SetValue` +
`MoveToLine`); redo re-applies. Bounded ring (~20). Coarse (matches the autosave granularity the
writer already trusts).

**Files:** `main.go` (ring fields; push in the autosave-tick handler when dirty; `ctrl+z` handler).
**DECISIONS:** (a) redo key — `ctrl+shift+z` isn't reliably distinguishable in terminals; options:
skip redo v1, or use a free combo. (b) checkpoint cadence — every changed tick (≤ every 1s) vs a
longer debounce. Recommend: skip redo v1 (undo is the 90%); checkpoint on changed autosave ticks.

---

## Stage 3 — Find & replace (in-document) — DESIGN

**Why:** `ctrl+f` search is find-only; revision needs rename-character / fix-repeat.

**Design:** extend the search screen (`search.go`): a `replace` input line beneath the query; a key
toggles into replace mode; replace-current / replace-all in the **active chapter** via
`editor.ReplaceRange` (already used by spell/grammar apply). Manuscript-wide replace is a stretch
goal (out of v1).

**Files:** `search.go`, `main.go`. **DECISIONS:** (a) the replace-mode key within search (a free
key — e.g. `ctrl+r` is spell; pick an unused one or `tab`-cycle a mode). (b) v1 scope = current
chapter only (recommend).

---

## Stage 4 — Resume at last cursor position — DESIGN

**Why:** `loadFile` always opens at line 0; reopening a long chapter loses your place.

**Design:** change `recent.json` entries from `[]string` to `[]struct{Path string; Line int}` (with
back-compat: tolerate the old string form on load). Store the current editor line on save/switch;
`editor.MoveToLine(line)` after `SetValue` in `loadFile`.

**Files:** `recent.go` (struct + migration), `main.go` (record line, restore on load). **DECISION:**
JSON migration of existing `recent.json` (old `["a","b"]` → new objects) — decode both shapes.

---

## Stage 5 — Manuscript-format title page — DESIGN

**Why:** the Manuscript export has a running header but no standard agent title page.

**Design:** for `StyleManuscript`, prepend a title page in `writeRTF`/`writePDF`/`writeDOCX`: author
+ contact block (top-left), approximate word count (top-right, round to nearest 250/500, summed from
`ManuscriptDoc`), title + byline centered at the vertical midpoint. `meta.Author` from
`OKASHI_AUTHOR`.

**Files:** the three export writers + `export.go` (compute total words; pass via `Meta`).
**DECISION:** contact fields — `OKASHI_AUTHOR` is only a name; add `OKASHI_CONTACT` (free-text
address/email block) or ship name + word count only for v1 (recommend: add `OKASHI_CONTACT`, optional).

---

## Stage 6 — Readability stats in the Words tab — DESIGN

**Why:** no readability/craft stats today; cheap, high signal.

**Design:** extend the Words inspector tab (`inspector.go`, already computes words/chars/paragraphs
via `computeDocStats`): add **reading time** (`words/238`, shown `m:ss`), **sentence-length**
mean±stddev (split on `.!?`), and an **overused-word** top-5 (case-folded word frequency, minus a
small stop-word set). All pure-Go, additive to the existing per-frame doc stats.

**Files:** `inspector.go` + the doc-stats computation. Small. No blocking decisions.

---

## Stage 7 — Snapshot history + manual `b` key — DESIGN

**Why:** Tier-0 shipped a per-session `.okashi-bak/` timestamped ring; this adds a manual snapshot
trigger + a restore UI ("get back an earlier draft").

**Design:** a `b` key (sidebar/editor) writes an immediate timestamped snapshot of the current file
into `.okashi-bak/` (reusing `snapshotBackup`, unconditional). A restore surface: a small screen /
list of a file's snapshots (name = timestamp) with preview + "restore" (copies the snapshot back
over the live file, first snapshotting the current version). 

**Files:** a new `snapshots.go` screen + `main.go` key. **DECISIONS:** (a) restore UI = a dedicated
screen vs a compact list in the inspector. (b) `b` binding is free. Effort: M.

---

## Stage 8 — Tier-3 QoL bundle — DESIGN

Small, independent items; group into one stage/plan.

- **8a. Discoverable selection mode** — a key toggles `tea.DisableMouse`/`tea.EnableMouseCellMotion`
  so native drag-select works, with a `-- SELECT --` status indicator; toggles back to okashi's
  mouse handling. Fixes the "selection feels broken" first impression. **DECISION:** the toggle key.
- **8b. Per-project settings** — width + smartquotes per project. Store in `manifest.json` as a
  `settings` object (or a per-dir `.okashi.json`). Read in place of the global `OKASHI_*` when
  present. **DECISION:** manifest stanza (touches the shared-contract shape → HARD GATE, confirm
  with the companion app) vs a separate per-dir config file (no contract impact — recommend).
- **8c. Non-UTF-8 guard in `loadFile`** — `utf8.Valid` check; on invalid, warn and don't mark dirty
  until an intentional edit (today a Latin-1 file is silently mutated on first save). Small.
- **8d. Lock-in messaging** — README paragraph: work is plain `.md` + readable `manifest.json`
  (grep/git-friendly, zero lock-in). Trivial.
- **8e. Operation-ordering hardening** (review risks #5/#6) — `commitStructure`: write the manifest
  before creating new files (a listed-but-missing file is benign; an orphan isn't). `moveDocument`:
  update the source manifest before the file move so a partial failure stays consistent. Small.

---

## Build order (stages)
1 DOCX → 2 Undo → 3 Find&Replace → 4 Resume-cursor → 5 Title page → 6 Readability → 7 Snapshots →
8 Tier-3 bundle. Stages are independent enough to reorder on request. Each: full plan + SDD when
scheduled; DOCX first.

## Non-goals (unchanged, hold the lean/anti-PKM ethos)
EPUB; character/timeline metadata; multiple cursors / split view.
