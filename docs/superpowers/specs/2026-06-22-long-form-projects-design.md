# Long-form projects: outline, manuscript, export, backups — design

**Date:** 2026-06-22
**Status:** Approved (pending spec review)
**Queue note:** This is the long-form project system. The **spell/grammar/syntax
view is a separate future spec** (lightweight assistive layer, NOT a revision
tool — heavy revision happens in Word on the exported file). The **project
rename** (okashi → TBD) is still queued and independent.

## Goal

Turn okashi into a real long-form tool: organize a novel-sized work as an
ordered folder of sections, see and restructure it in an **outline view**, read
it end-to-end in a **manuscript pager**, **export** it to a double-spaced
manuscript (RTF for Word/track-changes, PDF for print), and keep **safety
backups** before destructive edits. Simple works (an essay, a poem, a blog post)
stay as plain loose files; categories are just folders. No new lock-in: it's all
plain Markdown files on disk, readable in Finder.

---

## Section 1 — Project model & ordering

No new on-disk concepts — just folders and a naming convention:

- **Manuscript** = a folder containing **≥1 numerically-prefixed file**
  (`01-opening.md`). Auto-detected; gets the outline, manuscript, and export
  features.
- **Category** = a plain folder of unnumbered docs (`Poetry/`, `Essays/`,
  `Blog/`). Browsed normally; each doc is independent.
- **Loose docs** = unnumbered files (at the workspace root, in a category, or
  sitting inside a manuscript folder as notes). Inside a manuscript they are
  shown in a separate "loose" group and are **excluded** from the ordered
  manuscript and export.

Ordering helpers (pure, package `main`):

- `sectionOrder(name string) (n int, ok bool)` — parse the **leading run of
  digits** as an integer; `ok=false` if the name has no leading digit. So `1`,
  `01`, `001` all yield `1`, and sorting by `n` puts `2` before `10` (no lexical
  bug). Mixed digit widths still order correctly.
- `sectionTitle(name string) string` — strip the leading digits + one separator
  (`-`, `_`, `.`, or space), drop the `.md` extension, replace remaining `-`/`_`
  with spaces. `02-the-letter.md` → `the letter`. (Display only; the file keeps
  its real name.)
- `orderedSections(entries) (sections []fileEntry, loose []fileEntry)` — split a
  dir's `.md` files into the numbered (sorted by `n`, then name) and the loose
  (alphabetical) groups.
- `isManuscript(dir string) bool` — true if the dir has ≥1 file with
  `sectionOrder ok`.

When okashi **creates** sections it zero-pads to a consistent width (2 digits;
3 if the project reaches ≥100 sections) and keeps prefixes **contiguous**.

**Testing:** `sectionOrder` (`1`/`01`/`001`→1, `2`<`10`, no-digit→!ok);
`sectionTitle` strips prefix/sep/ext and de-slugs; `orderedSections` splits and
sorts; `isManuscript` true with a numbered file, false for a plain folder.

---

## Section 2 — Word-count rollups

- Reuse the existing word counter per file. `sectionWordCount(path) int` reads a
  file and counts; `projectWordCount(sections) int` sums the ordered sections.
- Cache per-file counts keyed by path+modtime so the outline/sidebar don't
  re-read every render; invalidate on save of that file.
- Surfaced in: the sidebar (per chapter), the outline (per section + project
  total), and the manuscript header (running + total).

**Testing:** `projectWordCount` sums sections (loose excluded); the cache
returns a fresh count after a file's modtime changes.

---

## Section 3 — Enhanced sidebar (inside a manuscript)

When the pane's current dir `isManuscript`:

- List the **ordered sections** (prefix stripped via `sectionTitle`) with a
  right-aligned per-section word count, then a separator, then the **loose**
  group.
- Selection/open/breadcrumb behavior is unchanged; only the rendering of rows
  (title + count, grouped) changes. Non-manuscript dirs render as today.

**Testing:** in a manuscript dir the sidebar shows titles (not raw filenames)
with counts and a loose group; a plain dir is unchanged.

---

## Section 4 — Outline view (full-screen)

A dedicated screen (`screenOutline`) toggled from a manuscript (key finalized in
the plan against the keymap; candidate `ctrl+l` for "outline/list").

- **Header:** project title (de-slugged folder name) · total word count ·
  section count.
- **Rows:** one per ordered section — `NN  Title  ····  1,240w`; selected row
  highlighted (reuse `selectedStyle`). A trailing "loose" group lists unnumbered
  files (not reorderable, not in the manuscript).
- **Keys:** `↑/↓` (and `j/k`) select; `Enter` opens the section in the editor
  (`screenWriting`, focus editor); **move up/down** (candidate `shift+↑/↓` or
  `J/K`) reorders; `n` new section (prompt → `<next>-<name>.md`); `m` →
  manuscript view; `esc` → back to the editor.
- **Reorder = renumber on disk:** moving a section swaps its position, then
  okashi **renames the affected files** so prefixes are contiguous and
  zero-padded (`os.Rename`). A `.backup` snapshot (Section 7) is taken
  **before** the renames. If the currently-open file is renamed, `m.currentFile`
  updates. Confined to the project folder; never touches files outside it.
- A shared `outlineRows(...)` helper owns the layout so the mouse hit-test
  matches the render (same lesson as the launch hub / breadcrumb): click selects,
  double-click opens.

**Testing:** `outlineRows` returns monotonic item rows; a reorder of `[01,02,03]`
moving #3 up yields files renamed to reflect `[01,02,03]` with the moved section
in slot 2 and the open-file path following the rename; `n` creates the next
number; a backup exists after a reorder.

---

## Section 5 — Manuscript view (full-screen read-through pager)

A dedicated screen (`screenManuscript`), reached from the outline (`m`) or a key.

- **Build:** concatenate the **ordered** sections (loose excluded) into one
  scroll buffer. Each section is preceded by a header rule `── Title ──`. Body
  defaults to **lightly-styled text** (so the line→source map below stays an
  exact 1:1 with file lines); full glamour rendering reflows/wraps and would
  break the line map, so if it's wanted the plan must build the map from the
  pre-render source, not the rendered output.
- **Running word count** in a header line; total at top.
- **Line → source map:** as it builds, the pager records, for each rendered
  line, the originating `(section file, line-in-file)`. Kept alongside the
  viewport content.
- **Navigation:** scroll (`↑/↓`, `pgup/pgdn`, wheel) the viewport; a pager
  cursor line tracks selection. **Single-click** moves the pager cursor;
  **double-click or `Enter`** on a line opens that section's file in the editor
  positioned at the mapped line (look up the line→source map, `loadFile`, set the
  editor cursor). `o` → outline; `esc` → editor.

**Testing:** the line→source map resolves a manuscript line back to the right
`(file, line)`; `Enter`/double-click on a line sets `currentFile` to that section
and moves the editor cursor to the mapped line; loose files don't appear.

---

## Section 6 — Export (standard manuscript format: RTF + PDF)

A manuscript-level **export** action (key/command from the outline or manuscript
view) writes, into `<project>/export/`:

- `<Title>.rtf` — pure-Go RTF control words. Opens natively in Word/Pages/Docs;
  Word track-changes works on it. Print from there too.
- `<Title>.pdf` — via the pure-Go `github.com/go-pdf/fpdf` library (no external
  binary).

**Standard manuscript layout (both targets):**
- Title page-ish header: **Title** (de-slugged folder name) and **by Author**
  (`OKASHI_AUTHOR` env, else empty).
- Body: **double-spaced**, 12pt (Courier-style for RTF/PDF), 1-inch margins,
  first-line paragraph indent, ragged right.
- Each **section starts a new page** with its title as a chapter heading.
- **Scene breaks** (`---` or a lone `#` in source) render centered `#`.
- Running page header `Author / TITLE / page#` (RTF/PDF page headers).

**Markdown handling:** parse each section with **goldmark** (already an indirect
dep via glamour) and emit a prose subset — paragraphs, headings (chapter/scene),
**bold**/*italic*, lists, blockquotes. Unsupported constructs degrade to plain
text. A shared `manuscriptDoc(sections) []block` AST feeds both the RTF and PDF
writers so they stay consistent.

**Testing:** `manuscriptDoc` produces the right block sequence (chapter starts,
paragraphs, emphasis runs, scene breaks) from sample sections; the RTF writer
emits double-spacing + indent control words and a parseable document; the PDF
writer produces a non-empty multi-page PDF; export writes both files under
`export/` and excludes loose files.

---

## Section 7 — Safety backups

- `backupSection(paths ...string)` copies the named files into
  `<project>/.backup/<RFC3339-ish timestamp>/` (preserving relative names)
  **before** any destructive op: outline reorder/renumber, section delete.
- An **on-demand** snapshot (key) copies the whole project's `.md` files into a
  fresh `.backup/<timestamp>/`.
- `.backup/` is ignored by the file pane (hidden, like other dotfiles) and never
  treated as a manuscript/category.
- Timestamps use a value passed in at call time (no `Date.now()`-style calls in
  pure helpers); the model supplies `time.Now()`.

**Testing:** `backupSection` creates a timestamped dir containing copies of the
named files; a reorder leaves a pre-reorder snapshot; `.backup/` is excluded from
the pane listing and from `isManuscript`.

---

## Section 8 — Navigation & keys

okashi gains two project-level screens beside the editor: **Outline** and
**Manuscript**. Proposed keys (finalized in the plan after a keymap audit so they
don't collide with existing `ctrl+b/n/o/p/s/t/d`, `esc`, `tab`):

- Editor → Outline: candidate `ctrl+l`.
- Outline ↔ Manuscript: `m` / `o` within those screens.
- Export: candidate `ctrl+e` from outline/manuscript.
- On-demand backup: candidate within the outline (e.g. `b`).
- `esc` from either screen returns to the editor.

The screens are only available when the current project dir `isManuscript`.

---

## Out of scope (non-goals)

- **Editable continuous manuscript buffer** (one big editable doc that splits
  back to files) — explicitly rejected; the pager is read-through + jump-to-edit.
- **Tags / cross-cutting categories** — categories are folders; tags are a
  possible later layer.
- **Heading-level sub-outline** (scenes within a chapter file) — a later layer
  on the folder outline.
- **Drag-to-reorder** — keyboard reorder first; mouse drag later.
- **Spell/grammar/syntax view** — separate future spec; this system stops at
  drafting + structure + export. Track-changes/revision lives in Word.
- **PDF as a track-changes target** — PDF is print-only; editing/markup is RTF.

## Risks

- **Reorder renames real files** — the one destructive new op. Mitigations: a
  `.backup` snapshot fires first; renames are confined to the project folder and
  validated (no path escape); the open file's path is updated after rename.
- **Outline/manuscript hit-test drift** — render and mouse mapping MUST share
  one layout helper (`outlineRows`, the line→source map), per the launch-hub and
  breadcrumb lessons.
- **Markdown→manuscript fidelity** — scope to a prose subset; degrade unknown
  constructs to plain text rather than failing; test the AST.
- **PDF dependency** — `go-pdf/fpdf` is pure Go (no binary), keeping Homebrew
  installs clean; vendored like other deps.
- **Tests stay hermetic** — anything touching `writingDir()`/project dirs uses
  `t.Setenv("OKASHI_DIR", t.TempDir())`.

## Build order (plans)

Cohesive system, built as stacked plans (each independently testable):

- **Plan A — Project model:** Sections 1, 2, 3, 7 (ordering + title helpers,
  word-count rollup + cache, enhanced sidebar, backup helper). The data layer.
- **Plan B — Outline view:** Section 4 (screen, rows, select, reorder+renumber
  on disk with backup, new section, jump, mouse).
- **Plan C — Manuscript pager:** Section 5 (concatenation, line→source map,
  scroll, jump-to-edit) + Section 8 wiring.
- **Plan D — Export:** Section 6 (`manuscriptDoc` AST, RTF writer, PDF writer,
  export action + `export/` output + `OKASHI_AUTHOR`).
