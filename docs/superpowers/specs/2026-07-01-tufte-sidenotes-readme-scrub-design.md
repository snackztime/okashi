# Tufte sidenote preview · README rewrite · companion-name scrub — design

**Date:** 2026-07-01
**Status:** Approved (direction)
**Context:** Three deliverables in one session. Two are near-mechanical (README, scrub); one is
a real feature (a sidenote layout for the `ctrl+p` preview). No shared foundation — each stands
alone. Build order: scrub → Tufte sidenotes → README (so the README is written against a
scrubbed, standalone-reading codebase).

---

## 1. Tufte-native preview — sidenotes in the margin

**Today:** `ctrl+p` renders the buffer through **glamour**. `previewTufte` (toggled by `t`)
switches between glamour's standard style and `tufteGlamourStyle()` (warm sepia, markerless
headings). GFM footnotes are flattened to a bottom **"Notes"** endnote section by
`footnotesToEndnotes` before rendering (glamour has no footnote extension). `renderPreview()`
(main.go) builds the string into the `m.preview` viewport, sized to the editor column
(`m.preview.Width`, `m.preview.Height`).

**Change:** in **Tufte mode**, when the terminal is wide enough and the document has footnotes,
float each footnote into a **right-margin gutter** aligned to the row where its reference
appears — the signature Tufte look. Everything else (Default mode, narrow terminals, footnote-free
docs) keeps today's behavior byte-for-byte.

### Approach — glamour body + a sidenote layout pass (chosen; Approach B)

Reuse glamour for the body (its inline/block rendering + ANSI-aware wrapping are solid); add a
pure layout pass on top. We do **not** re-implement a markdown renderer.

**New in `preview.go`:**

- `footnotesToSidenotes(orig string) (body string, notes []string)` — sibling of
  `footnotesToEndnotes`, sharing the same code-mask helper (extract the masking/restore into a
  small shared helper so footnote-like text inside code is never touched). It:
  - masks code, collects `[^id]: text` definitions, drops the definition lines;
  - rewrites each **referenced** `[^id]` in body order to the superscript marker
    (`superscript(n)`), exactly as `footnotesToEndnotes` already does;
  - returns the body (superscript refs in place, **no** appended Notes section) and `notes`,
    the note texts in reference order (`notes[0]` is marker ¹, etc.);
  - orphan refs (no matching def) stay literal and are **not** counted; unreferenced defs are
    dropped. If there are no referenced footnotes, `notes` is empty (caller falls back to the
    normal path).

- `layoutSidenotes(body string, notes []string, measure, gutter int) string` — **pure and
  fully unit-testable** (no glamour). Given the already-rendered `body` (glamour output, wrapped
  to `measure`) and the ordered `notes`:
  - Split `body` into lines. For note *i* (1-based), find the **first** body line containing a
    superscript run whose runes equal `superscript(i)` exactly — matching a whole superscript
    run, never a substring, so `¹` (note 1) does not match inside `¹²` (note 12). That line is
    the note's **anchor row**.
  - Wrap each note's text to `gutter` columns; prefix line 1 with its number and a thin space
    (e.g. `¹ `). Place the note block starting at `max(anchorRow, nextFree)`, then set
    `nextFree = placed + height + 1` so consecutive notes never overlap (they cascade down).
    A note whose anchor is not found (defensive) is appended after `nextFree`.
  - Compose output row by row: `padTo(bodyLine, measure) + " ┆ " + gutterLine`, where the gutter
    line is the note text for that row or spaces. Extend beyond the last body row with
    `padTo("", measure) + " ┆ " + gutterLine` while gutter content remains. Measure visible
    width with `charmbracelet/x/ansi` `StringWidth` (already a dep) so ANSI styling doesn't
    break padding. Style the `┆` divider muted and the note text sepia/muted (lipgloss).

**Changed in `renderPreview()` (main.go):**

- Compute `total := m.preview.Width` (fallback `m.colWidth`). Sidenotes engage **iff**
  `m.previewTufte && total >= sidenoteMinWidth` (`sidenoteMinWidth = 90`) **and**
  `footnotesToSidenotes(buffer)` returns a non-empty `notes`.
- When engaged: `gutter := clamp(total/3, 18, 30)`; `measure := total - gutter - 3` (the `" ┆ "`
  gap). Render the **body** (from `footnotesToSidenotes`) through the same Tufte glamour renderer
  at `WithWordWrap(measure)`, then `layoutSidenotes(rendered, notes, measure, gutter)` → viewport.
- Otherwise: the existing path unchanged — `footnotesToEndnotes` + glamour at `WithWordWrap(total)`,
  Tufte or standard style.
- The PREVIEW header (main.go ~1414) may append `· sidenotes` when sidenotes are engaged; purely
  cosmetic.

**No new key.** Sidenotes are simply what Tufte mode produces when width + footnotes allow.
Default mode and narrow Tufte mode are untouched.

### Tests (`preview_test.go`)
- `footnotesToSidenotes`: a doc with two referenced footnotes → body has `¹`/`²`, no "Notes"
  section, `notes` has the two texts in order; orphan ref stays literal + uncounted; code-fenced
  `[^1]` untouched; footnote-free doc → empty `notes`.
- `layoutSidenotes`: note text lands on the ref's row (` ┆ ¹ …` on the anchor line); two notes on
  the same/adjacent rows cascade without overlap; a note longer than the body extends past it; a
  note wraps within `gutter`; `¹` does not mis-anchor onto a `¹²` run.
- Width gate (via a small helper or `renderPreview` seam): below `sidenoteMinWidth`, the endnote
  path is chosen (no `┆` gutter).

### Out of scope
- Non-footnote margin notes (no markdown syntax for them — would need proprietary syntax; barred
  by the markdown-flavor contract). Only footnotes become sidenotes.
- Changing Default mode, the `t` toggle semantics, or the export renderers.
- A full own markdown renderer (Approach A, rejected as non-lean).

---

## 2. Companion-name scrub (user-facing strings + code comments)

Make the shipped surface read as a standalone product. **Genericize, don't delete** — keep the
architectural "why".

- **User-facing status strings** (main.go, 6 sites): replace strings that name the companion
  app (e.g., `managed by <companion-app>` / `structure is managed by <companion-app>`) with a
  neutral phrasing that names no external project, e.g. `manifest.json is read-only (managed
  externally)` and `structure is read-only (external manifest)`; `chapter files are managed by
  <companion-app>` → `chapter files are read-only (external manifest)`. Keep the meaning: okashi
  won't structurally edit an externally-owned manifest.
- **Code comments** (manifest.go, manuscript.go, source.go): reword companion-app name
  references to "the companion app" / "the external owner of the manifest". Preserve the
  serialization-match rationale (sorted keys, no trailing newline, `[]`-not-null) — just drop the
  proper noun.
- **Test name** `TestWriteManifestMatchesCompanionAppSortedKeys` → `TestWriteManifestSortedKeys`
  (or `…MatchesExternalSortedKeys`); keep the assertions.
- **Left as internal (unchanged):** `CLAUDE.md` (its Shared-Contracts structure *is* the
  companion relationship; genericizing loses dev context) and `docs/superpowers/**` (clearly
  internal planning history).

**Verification:** `grep -rniE '<companion-app-name>' --include='*.go'` returns **0**; the suite
still passes (renamed test still runs).

---

## 3. README rewrite (with a shortcuts table)

A GitHub-facing README for okashi as a standalone product. No sibling-project references.

**Sections:**
1. **Title + one-line tagline** — "A lean terminal writing app for long-form manuscripts."
2. **Screenshot placeholder** (`<!-- screenshot -->` comment + a short feature blurb).
3. **Install** — Homebrew placeholder (`brew install …` — TBD tap, marked) and
   `go build ./...` / `go run .` (note Go 1.25).
4. **Quick start** — `OKASHI_DIR=~/writing okashi`; the launch hub; opening a doc.
5. **Keyboard shortcuts** — a Markdown table transcribed from `helpText` (the F1 cheatsheet),
   grouped (Navigation / Files / Writing / Export & preview / Search). Single source of truth =
   the current `helpText`; the table must match it.
6. **Project model** — the atom is a `.md` file; a **manuscript** is a folder with a
   `manifest.json` (order/titles/membership); **category** = plain folder; **resources** =
   unlisted files. Legacy numbered-prefix folders are read-only.
7. **Export** — `ctrl+e` → RTF + PDF, Manuscript or Tufte style.
8. **Preview** — `ctrl+p` glamour preview; `t` toggles Tufte (with margin **sidenotes** when the
   terminal is wide).
9. **Configuration** — table of env knobs: `OKASHI_DIR`, `OKASHI_WIDTH`, `OKASHI_SMARTQUOTES`,
   `OKASHI_THEME`, `OKASHI_ICONS`, `OKASHI_AUTHOR`.
10. **Text selection** — hold **⌥ Option** (iTerm2/Ghostty/Terminal.app) or **Shift** (most
    terminals) and drag; `⌘C` copies. (Mouse reporting suppresses native drag-select otherwise.)
11. **License** — placeholder line (match repo's existing license if present).

**Not unit-tested** (prose). Correctness bar: the shortcuts table matches `helpText`; the env-var
table matches the knobs okashi actually reads; no companion-app name strings.

---

## 4. Sequencing (for the plan)
1. **Scrub** — strings + comments + test rename; grep-clean. (main.go, manifest.go, manuscript.go,
   source.go, manifest_writers_test.go)
2. **Tufte sidenotes** — `footnotesToSidenotes` + shared mask helper (TDD) → `layoutSidenotes`
   (TDD) → wire into `renderPreview` + width gate + header. (preview.go, main.go, preview_test.go)
3. **README** — rewrite against the scrubbed, sidenote-aware app. (README.md)
