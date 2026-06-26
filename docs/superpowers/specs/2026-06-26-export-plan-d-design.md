# Plan D — Export (RTF + PDF, Manuscript + Tufte): design (finalized)

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Parent spec:** `docs/superpowers/specs/2026-06-22-long-form-projects-design.md` (§6, §8).
This document finalizes §6 and is grounded in a research pass (RTF verified by
round-trip through macOS `textutil`; `fpdf` API + goldmark v1.7.13 verified against
source). Where this doc and the parent differ, **this doc governs for Plan D**.

## Goal

A `ctrl+e` export that writes, into `<project>/export/`, an **editable RTF** and a
**printable PDF** of either the **current document** or the **whole manuscript**, in one
of two styles: **Manuscript** (Courier, double-spaced — for agents/editors) or **Tufte**
(elegant serif — a printable/editable copy for yourself or a reader). Pure Go throughout
(no external binary), so the Homebrew formula stays binary-only.

## Scope

**In Plan D:** the goldmark parse → `ManuscriptDoc` AST; the RTF writer; the PDF writer
(both styles, ET Book embedded); the `ctrl+e` flow (style chooser + scope-from-screen,
writing both `.rtf` and `.pdf`); bundling the open-licensed ET Book font.

**Deferred (NOT Plan D):** true Tufte sidenotes/marginnotes/margin figures (RTF can't
represent them; even in PDF they need manual layout — a later pass); footnote markdown
extension; A4 / page-size setting; per-chapter separate export files; the queued **Tufte
*preview*** (a separate brainstorm).

## Decisions (locked)

- **Trigger:** `ctrl+e` → a one-key style chooser (`m` Manuscript / `t` Tufte / `esc`
  cancel). **Scope from screen:** in the editor it exports the **current document**; on
  the outline screen it exports the **whole manuscript**. Each export writes **both**
  `<name>.rtf` and `<name>.pdf`.
- **Styles:** Manuscript and Tufte (render-time constants only — same AST).
- **Tufte serif:** embed the open-licensed **ET Book** TTF via `go:embed` (true Tufte
  type + perfect Unicode → no transcode on the Tufte PDF path).
- **Page size:** US Letter (defer A4).
- **Titles:** single-doc title = `sectionTitle(filename)`; whole-manuscript title =
  `projectTitle(folder)`. Author = `os.Getenv("OKASHI_AUTHOR")` (else empty).
- **Markdown subset:** CommonMark via `goldmark.DefaultParser()` — paragraph, heading,
  `---`/lone-`#` scene break, bold/italic, list, blockquote; everything else degrades to
  plain text.
- **Deps:** promote `github.com/yuin/goldmark` v1.7.13 to a **direct** require (pin it);
  add `codeberg.org/go-pdf/fpdf` v0.12.0 (pure Go — **not** the archived
  `github.com/go-pdf/fpdf`). Transcode uses `golang.org/x/text/encoding/charmap` (x/text
  already indirect).

## Section 1 — The AST (`export_ast.go`)

```go
type Run struct { Text string; Bold, Italic bool }

type Block interface{ isBlock() }
type Paragraph  struct{ Runs []Run }
type Heading    struct{ Level int; Runs []Run } // in-body subheads (H2+)
type Blockquote struct{ Children []Block }
type List       struct{ Ordered bool; Start int; Items []Paragraph }
type SceneBreak struct{}

type Section struct{ Title string; Blocks []Block } // Title from FILENAME via sectionTitle
type ManuscriptDoc []Section

func parseSection(src []byte) []Block        // goldmark walk
func manuscriptDoc(dir string, sections []fileEntry) ManuscriptDoc // one Section per file
```

**Walk** (`goldmark.DefaultParser().Parse(text.NewReader(src))`, then `ast.Walk`,
returning `WalkSkipChildren` after handling each top-level block so blockquote/list inner
paragraphs aren't re-emitted):
- `Heading` with `ChildCount()==0` → `SceneBreak` (a lone `#`); the **document's first
  block if it's an H1** → **dropped** (it would duplicate the filename title); other
  headings → `Heading{Level, runs}`.
- `ThematicBreak` (`---`) → `SceneBreak`.
- `Paragraph` → `Paragraph{runs}`; `Blockquote` → `Blockquote{blocks}`; `List` →
  `List{IsOrdered(), Start, items}` (read `Start` only when ordered).
- **Inline runs** (recursive, emphasis bitmask 1=italic@Level1, 2=bold@Level2):
  `Text` → `Run{ t.Segment.Value(src), bold, italic }`, and **on `SoftLineBreak()` append
  a `Run{" "}`** (goldmark strips the newline — without this, hard-wrapped words fuse);
  `Emphasis` → recurse with the level's bit; `CodeSpan`/`Link`/`Image`/unknown → degrade
  to their child text (drop URLs; image → alt or drop).

**Scope falls out of Section count:** single-doc = `ManuscriptDoc{{Title: sectionTitle(base),
Blocks: parseSection(data)}}`; whole = one Section per `orderedSections(...).sections` in
order (loose dropped).

**Testing:** the walk produces the right block/run sequence (paragraph runs with
bold/italic flags, scene break from `---` and lone `#`, list, blockquote); a soft-wrapped
paragraph keeps a space between lines; a leading H1 is dropped; loose files excluded.

## Section 2 — RTF writer (`export_rtf.go`, pure string emission)

`func writeRTF(doc ManuscriptDoc, st Style, meta Meta) []byte` — `strings.Builder`, 7-bit
ASCII. Verified control words (round-tripped through `textutil`):
- **Prolog/fonts/page:** `{\rtf1\ansi\ansicpg1252\deff0\uc1` +
  `{\fonttbl{\f0\fmodern\fcharset0 Courier New;}{\f1\froman\fcharset0 Georgia;}}` +
  `\paperw12240\paperh15840` + margins (manuscript `\margl1440\margr1440\margt1440\margb1440`;
  tufte `\margl2160\margr2880\margt1440\margb1440` for a ~66-char measure).
- **Running header (manuscript only):** `{\header\pard\qr\f0\fs24 <Author> / <UPPER Title> / \chpgn\par}`
  (`\chpgn` = live page number). Tufte omits it.
- **Body font:** manuscript `\f0\fs24` (Courier 12pt; `\fs` is half-points); tufte `\f1\fs24` (serif).
- **Per Section:** `\page` then `{\pard\qc\sb480\sa240\b <Title>\b0\par}` (centered bold
  chapter title), then blocks:
  - Paragraph: `\pard` + indent/spacing + runs + `\par` — manuscript `\fi720\sl480\slmult1`
    (0.5" indent, **double-spaced**, ragged-right=default, never `\qj`); tufte
    `\fi360\sl276\slmult1` (~1.15 leading). **`\pard` resets at every paragraph** (else
    `\fi`/`\sl`/`\qc` leak).
  - Heading: `{\pard\sb240\sa120\b\fs<28-2*Level> <runs>\b0\par}`; SceneBreak:
    `{\pard\qc\sb240\sa240 #\par}`; Blockquote: `\pard\li720\ri720 …`; List: `\pard\fi-360\li720` marker + runs.
  - **Runs** as matched self-closing groups so style can't leak: `{\b …}`, `{\i …}`, `{\b\i …}`.
- **Escaping** (every text rune): `\` `{` `}` → backslash-escaped; rune `<0x80` literal;
  `0x80..0xFFFF` → `fmt.Sprintf("\\u%d?", int16(r))` (**signed** 16-bit + `?` fallback);
  `>0xFFFF` → UTF-16 **surrogate pair**, two `\u` tokens. This cleanly absorbs the editor's
  smart quotes / em-dashes — no transcode for RTF.
- **Track changes:** emit **no** revision markup; a clean RTF opens fully editable, the
  user enables Track Changes in Word.

**Testing:** golden checks that manuscript emits `\sl480` + `\fi720` + the `\chpgn` header
and tufte emits the serif font + wider margins + no header; bold/italic produce
`{\b …}`/`{\i …}`; a curly quote / em-dash / astral rune escape to the verified `\u`
forms; braces balance; the document parses (optionally via `textutil` when available, else
a structural assertion).

## Section 3 — PDF writer (`export_pdf.go`, `codeberg.org/go-pdf/fpdf` v0.12.0)

`func writePDF(doc ManuscriptDoc, st Style, meta Meta) ([]byte, error)`.
- **Manuscript:** `fpdf.New("P","pt","Letter","")`, `SetMargins(72,72,72)`,
  `SetAutoPageBreak(true,72)`, `AliasNbPages("{nb}")`, `SetFont("Courier","",12)`, line
  height `h=24` (double). `SetHeaderFunc` right-aligns `Author / UPPER(TITLE) / PageNo()`.
  Body via `MultiCell(0,h, indent+text,"","L",false)` (indent = 5 spaces = exact 0.5" in
  monospace). **Core Courier is cp1252** → transcode each string UTF-8→cp1252 via
  `charmap.Windows1252.NewEncoder()` (ASCII-fallback un-encodable runes) before passing to
  fpdf — this fixes the smart-quote mojibake.
- **Tufte:** `SetMargins(108,90,108)` (~66-char measure), the **embedded ET Book** serif
  via `pdf.AddUTF8FontFromBytes("etbook","",etbookRegular)` (+ `"B"`, `"I"`, `"BI"` from
  their TTFs; fpdf needs one TTF per style, no synthetic bold), `SetFont("etbook","",12)`,
  relaxed `h≈16`. **No transcode** (UTF-8 TTF). No manuscript header.
- **Per Section:** `AddPage()`; centered bold title (`SetFont(font,"B",titleSize)` +
  `CellFormat(0,h,Title,"",1,"C",…)`); then blocks. **Emphasis caveat (confirmed in fpdf
  source):** `MultiCell` can't change font mid-cell, so a paragraph **with** bold/italic
  runs renders via `pdf.HTMLBasicNew().Write(h, "<b>…</b><i>…</i>")` (which loses the
  first-line indent); a plain paragraph uses indented `MultiCell`. SceneBreak → centered
  `#`. Output to an in-memory buffer (`pdf.Output(&buf)`).

**Testing:** `writePDF` returns a non-empty `%PDF`-headed multi-page buffer for a 2-section
doc in each style; the embedded-font path doesn't error; emphasis paragraphs route through
the HTML writer without panicking; a smart-quote string in the manuscript (Courier) path
produces no error and round-trips through the cp1252 encoder.

## Section 4 — Font assets (ET Book)

Bundle ET Book (Edward Tufte's open-licensed typeface) TTFs under `assets/etbook/`,
embedded with `//go:embed`. **Source:** the upstream repo `github.com/edwardtufte/et-book`
(dual MIT / SIL OFL licensed), which ships per-style `.ttf` files (roman, bold, italic,
bold-italic) under `et-book/`. Copy those four TTFs + the `LICENSE` into `assets/etbook/`.
If a needed weight is OTF-only upstream, convert to TTF (fpdf's `AddUTF8FontFromBytes`
requires TrueType). This is the only new non-code asset; it adds ~0.5–1 MB to the binary.
(If the font files cannot be fetched at implementation time, the plan's font task is the
one place to surface that and pause — everything else is pure code.)

## Section 5 — Wiring (`export.go`) & keys

- `ctrl+e` (free; editor screen and outline screen) sets `m.exportPrompt = true` and a
  status hint `export: m manuscript · t tufte · esc cancel`. A capture block routes `m`/`t`
  → `m.runExport(style)`, `esc`/other → cancel.
- `runExport(style)`: scope from screen — `screenOutline` → whole manuscript
  (`manuscriptDoc(m.files.dir, orderedSections(...).sections)`, title `projectTitle`);
  else current doc (one Section from `m.currentFile`, title `sectionTitle`). Build the doc,
  `MkdirAll(<dir>/export)`, write `<slug>.rtf` and `<slug>.pdf`, set a status with the
  output paths (or the error). `<slug>` = slug of the title.
- A status hint for `ctrl+e` is added to the editor + outline status strings; README documents it.

**Testing (hermetic, `t.TempDir`):** `ctrl+e` then `m` from the editor writes
`export/<slug>.rtf` + `.pdf` of the current doc; `ctrl+e` then `t` from the outline writes
the whole manuscript; both files are non-empty and the PDF starts with `%PDF`; loose files
are excluded from the manuscript export; `esc` cancels with no files written.

## Risks (from the research — all handled in the plan)

- **Smart quotes × Courier cp1252** (the sharp one): the editor inserts curly quotes/em-
  dashes by default; fpdf core fonts are cp1252, so the manuscript PDF path **must**
  transcode via `x/text/charmap` (RTF is fine via `\u`). Only bites the PDF.
- **RTF escaping:** `0x8000..0xFFFF` must be **negative** `int16`; astral runes need a
  surrogate **pair**; `\fs` is half-points and dimensions are twips (1440/in) — silent 2×
  traps. `\pard` reset + brace balance are mandatory.
- **fpdf `MultiCell` single-font limit:** emphasis routes through the HTML writer (drops
  the indent); an indented-AND-emphasized paragraph can't have both cheaply — accepted.
- **Dep pinning:** `codeberg.org/go-pdf/fpdf` (not the archived github fork); pin goldmark
  when promoting to direct so a glamour bump can't move the AST API. Consider `go mod vendor`.
- **Goldmark traps:** re-insert a space on `SoftLineBreak()`; `WalkSkipChildren` after each
  block; dropping a leading H1 loses a genuinely intended body-leading H1 (owned decision);
  lone-`#` vs titled-H1 distinguished by `ChildCount()==0`.

## Build order (plan)

1. AST: `export_ast.go` (`Block` model, goldmark walk, `parseSection`, `manuscriptDoc`) + tests; promote goldmark to direct.
2. RTF writer: `export_rtf.go` (both styles, escaping) + tests.
3. PDF deps + manuscript writer: add `codeberg.org/go-pdf/fpdf`; `export_pdf.go` manuscript style (Courier + cp1252 transcode) + tests.
4. ET Book assets + Tufte PDF: embed the TTFs; the Tufte style path + tests.
5. Wiring: `export.go`, `ctrl+e` chooser + scope-from-screen + write both files + tests.
6. Docs/status.
