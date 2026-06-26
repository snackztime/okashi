# Export: GFM + footnotes (parser flavor parity) — design

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Context:** Honors the `CLAUDE.md` Shared-Contract §2 markdown-flavor HARD GATE — the export
parser must be **CommonMark + GFM + footnotes** via goldmark, so the shared corpus parses
identically in okashi and `../inkmere`. Today `export_ast.go` uses `goldmark.DefaultParser()`
(CommonMark only). This is the queued "export update pending" follow-up.

## Goal

Switch the export parser to goldmark with the GFM and Footnote extensions; render **footnotes
as per-chapter endnotes**; **degrade** the rarer GFM constructs (tables, strikethrough, task
lists) gracefully. The `ctrl+p` preview stays on glamour (which can't be given the footnote
extension) — noted as a known limitation.

## Parser (`export_ast.go`)

Replace `goldmark.DefaultParser()` with a parser built once with the extensions:

```go
import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var exportParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()
```

`parseSection` calls `exportParser.Parse(text.NewReader(src))`. `goldmark/extension` ships
with the already-direct goldmark dependency — no new module. **Each chapter file is parsed
separately**, so footnote numbering restarts per chapter and the footnote list lands at the
end of that chapter's parse — per-chapter endnotes fall out naturally.

## AST additions

New block type (package `main`, `export_ast.go`):

```go
type Endnote  struct { Num int; Runs []Run }
type Endnotes struct { Items []Endnote }
func (Endnotes) isBlock() {}
```

Walk handling for the extension nodes (`github.com/yuin/goldmark/extension/ast`):
- **`*xast.FootnoteLink`** (inline; has `.Index int`) → a `Run{Text: "[N]"}` marker (e.g. `[1]`).
- **`*xast.FootnoteList`** (block; children are `*xast.Footnote`, each with `.Index` + content
  blocks) → an `Endnotes` block; each `Footnote` → `Endnote{Num: Index, Runs: <flattened
  inline runs of its blocks>}` (a multi-paragraph footnote joins its runs with a space). It is
  the chapter's last block (goldmark emits the list at the end of the parse).
- **`*xast.Strikethrough`** (inline) → degrade: recurse to its child text as plain `Run`s (no
  styling).
- **`*xast.TaskCheckBox`** (inline, first child of a task list item) → prefix the list item's
  runs with `"[x] "` (checked) or `"[ ] "` (unchecked).
- **`*xast.Table`** (block; `TableHeader`/`TableRow` → `TableCell` children) → degrade: each
  row becomes a `Paragraph` whose runs are the cells' text joined by `" | "`. Header row first,
  then body rows.

Unknown/other extension nodes continue to degrade to their child text via the existing
`default` recurse in `inlineRuns`/`blockFrom`.

## Writers (`export_rtf.go`, `export_pdf.go`)

One new block case in each writer — the `Endnotes` block, rendered at the chapter's end
(after the body, before the next chapter's `\page` / `AddPage`):
- A small bold **"Notes"** heading.
- Each item as `N. <runs>` (a hanging-indent line in RTF; a `MultiCell`/per-run line in PDF),
  in both Manuscript and Tufte styles (style only changes the surrounding font/spacing
  constants, as for every other block).

The `[N]` markers in the body are already plain `Run`s. **Table-degraded paragraphs,
task-list items, and de-struck text all flow through the existing Paragraph/List rendering —
no new writer code beyond `Endnotes`.**

## Preview (`ctrl+p`)

Unchanged. glamour v1.0.0 exposes no option to add goldmark extensions, so the preview keeps
its built-in GFM and renders footnote syntax (`[^1]`) as literal text. This is a known
limitation (the gate is about the *corpus/export* flavor). Update `CLAUDE.md` §2 to: clear the
"export update pending" note, and record that the glamour preview covers GFM but not footnotes.

## Testing (hermetic)

- `parseSection("text[^1]\n\n[^1]: the note")` → the body Paragraph contains a `Run{"[1]"}`,
  and the section's blocks end with an `Endnotes` block whose `Items[0] == {Num:1, "the note"}`.
- A GFM table parses to degraded `Paragraph`s with `" | "`-joined cell text (header + rows).
- A task list parses to list items prefixed `"[ ] "` / `"[x] "`.
- `~~struck~~` → a plain `Run{"struck"}` (no styling, no markers).
- `writeRTF`/`writePDF` of a doc with an `Endnotes` block emit a "Notes" section (RTF contains
  the literal "Notes" and the entry; the PDF builds as a valid `%PDF`).

## Out of scope

- True Tufte **sidenotes** / margin notes (manual PDF layout) — still deferred; footnotes are
  endnotes for now.
- Page-bottom footnotes (RTF `{\footnote}` / fpdf page-bottom) — endnotes instead.
- Rich table rendering (real ruled tables) — degraded to text rows.
- Replacing glamour to give the preview footnotes — its own future effort if wanted.
