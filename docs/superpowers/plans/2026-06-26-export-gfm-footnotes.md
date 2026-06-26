# Export GFM + Footnotes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Switch the export parser to goldmark + GFM + Footnote (matching `../inkmere`'s flavor), render footnotes as per-chapter endnotes, and degrade the rarer GFM constructs (tables, strikethrough, task lists) gracefully.

**Architecture:** A package-level `exportParser` with the extensions feeds the existing walk in `export_ast.go`; new walk cases turn footnote refs into `[N]` markers and the footnote list into an `Endnotes` block (per chapter, since each file parses separately), prefix task-list items with `[ ]`/`[x]`, and degrade tables to pipe-joined rows. The two writers gain one `Endnotes` case each. The glamour preview is unchanged.

**Tech Stack:** Go, `github.com/yuin/goldmark` + `/extension` + `/extension/ast` (already the direct goldmark dep — no new module).

**Design spec:** `docs/superpowers/specs/2026-06-26-export-gfm-footnotes-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt` (not on PATH).
- Parser: `goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()`, built once at package level. `goldmark/extension` and `goldmark/extension/ast` ship with the existing goldmark dependency — no `go get`.
- **Footnotes → per-chapter endnotes:** a footnote ref (`FootnoteLink`, `.Index`) renders as a `[N]` run; the footnote list (`FootnoteList` → `Footnote` children, `.Index` + content) becomes a single `Endnotes` block (the chapter's last block). Numbering is per chapter (each file parses separately).
- **GFM degrade:** strikethrough → plain text (already handled by the default recurse); task-list item → prefixed `[ ] `/`[x] `; table → a `Blockquote` of `" | "`-joined rows (one paragraph per row).
- Writers add ONE new block case (`Endnotes`): a bold "Notes" heading then `N. <body>` lines, both styles. No other writer changes.
- Preview (`ctrl+p`) unchanged (glamour exposes no extension hook). `CLAUDE.md` §2 note updated.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Parser flavor + footnotes → endnotes (`export_ast.go`)

**Files:**
- Modify: `export_ast.go`
- Test: `export_ast_test.go`

**Interfaces:**
- Consumes: `Run`, `Block`, `Paragraph` (existing); goldmark `extension`, `extension/ast` (alias `xast`).
- Produces: `var exportParser`; `type Endnote struct { Num int; Runs []Run }`; `type Endnotes struct { Items []Endnote }` (a `Block`); footnote handling in the walk.

- [ ] **Step 1: Write the failing tests**

Add to `export_ast_test.go`:

```go
func TestParseSectionFootnoteEndnotes(t *testing.T) {
	src := []byte("She paused[^a] there.\n\n[^a]: A note about pausing.\n")
	blocks := parseSection(src)
	// The body paragraph carries a [1] marker run.
	p, ok := blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("block 0 should be a Paragraph, got %T", blocks[0])
	}
	var marker bool
	for _, r := range p.Runs {
		if r.Text == "[1]" {
			marker = true
		}
	}
	if !marker {
		t.Fatalf("footnote ref should render a [1] marker run: %#v", p.Runs)
	}
	// The last block is the chapter's Endnotes.
	last := blocks[len(blocks)-1]
	en, ok := last.(Endnotes)
	if !ok {
		t.Fatalf("last block should be Endnotes, got %T", last)
	}
	if len(en.Items) != 1 || en.Items[0].Num != 1 {
		t.Fatalf("expected 1 endnote numbered 1, got %#v", en.Items)
	}
	var body string
	for _, r := range en.Items[0].Runs {
		body += r.Text
	}
	if body != "A note about pausing." {
		t.Fatalf("endnote body = %q", body)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestParseSectionFootnoteEndnotes 2>&1 | tail`
Expected: build/compile error — `Endnotes` undefined (and the footnote isn't yet parsed).

- [ ] **Step 3: Switch the parser, add the types and footnote walk cases**

In `export_ast.go`, update the imports:

```go
import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	xast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// exportParser parses with GFM + footnotes so the export matches the shared corpus flavor
// (CLAUDE.md shared-contract §2). Built once.
var exportParser = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()
```

Add the endnote types (next to the other `Block` types):

```go
// Endnote is one footnote, collected into a chapter's Endnotes.
type Endnote struct {
	Num  int
	Runs []Run
}

// Endnotes is the chapter's footnote bodies, rendered as a "Notes" section at the end.
type Endnotes struct{ Items []Endnote }

func (Endnotes) isBlock() {}
```

In `parseSection`, use `exportParser`:

```go
func parseSection(src []byte) []Block {
	root := exportParser.Parse(text.NewReader(src))
	var blocks []Block
	first := true
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		b, drop := blockFrom(n, src, first)
		first = false
		if drop || b == nil {
			continue
		}
		blocks = append(blocks, b)
	}
	return blocks
}
```

In `blockFrom`, add a `*xast.FootnoteList` case (before the `default`):

```go
	case *xast.FootnoteList:
		var en Endnotes
		for f := n.FirstChild(); f != nil; f = f.NextSibling() {
			fn, ok := f.(*xast.Footnote)
			if !ok {
				continue
			}
			en.Items = append(en.Items, Endnote{Num: fn.Index, Runs: itemRuns(f, src)})
		}
		if len(en.Items) == 0 {
			return nil, true
		}
		return en, false
```

In `inlineRuns`, add a `*xast.FootnoteLink` case (before the `default`):

```go
		case *xast.FootnoteLink:
			runs = append(runs, Run{Text: fmt.Sprintf("[%d]", t.Index), Bold: bold, Italic: italic})
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestParseSection|TestManuscriptDoc' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_ast.go export_ast_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add export_ast.go export_ast_test.go go.mod go.sum
git commit -m "export: goldmark GFM+footnotes parser; footnotes -> per-chapter endnotes"
```

---

## Task 2: GFM degrade — task lists, tables, strikethrough (`export_ast.go`)

**Files:**
- Modify: `export_ast.go`
- Test: `export_ast_test.go`

**Interfaces:**
- Consumes: `xast` (Task 1), `inlineRuns`, `plainText` (existing, `export_pdf.go`), `Blockquote`, `Paragraph`.

- [ ] **Step 1: Write the failing tests**

Add to `export_ast_test.go`:

```go
func TestParseSectionTaskListAndStrike(t *testing.T) {
	blocks := parseSection([]byte("- [ ] todo\n- [x] done\n"))
	lst, ok := blocks[0].(List)
	if !ok {
		t.Fatalf("block 0 should be a List, got %T", blocks[0])
	}
	first, second := "", ""
	for _, r := range lst.Items[0].Runs {
		first += r.Text
	}
	for _, r := range lst.Items[1].Runs {
		second += r.Text
	}
	if first != "[ ] todo" || second != "[x] done" {
		t.Fatalf("task list items = %q / %q, want '[ ] todo' / '[x] done'", first, second)
	}

	// Strikethrough degrades to plain text (handled by the default recurse).
	sb := parseSection([]byte("a ~~struck~~ b"))
	var joined string
	for _, r := range sb[0].(Paragraph).Runs {
		joined += r.Text
	}
	if joined != "a struck b" {
		t.Fatalf("strikethrough should degrade to plain text: %q", joined)
	}
}

func TestParseSectionTableDegrades(t *testing.T) {
	src := []byte("| A | B |\n|---|---|\n| 1 | 2 |\n")
	blocks := parseSection(src)
	bq, ok := blocks[0].(Blockquote)
	if !ok {
		t.Fatalf("a table should degrade to a Blockquote of rows, got %T", blocks[0])
	}
	// header row "A | B", body row "1 | 2"
	row0 := bq.Children[0].(Paragraph)
	var r0 string
	for _, r := range row0.Runs {
		r0 += r.Text
	}
	if r0 != "A | B" {
		t.Fatalf("first table row = %q, want 'A | B'", r0)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestParseSectionTaskListAndStrike|TestParseSectionTableDegrades' 2>&1 | tail`
Expected: FAIL — the checkbox marker is dropped and the table isn't a Blockquote yet (strikethrough already passes via the default recurse).

- [ ] **Step 3: Add the task-checkbox and table cases**

In `inlineRuns`, add a `*xast.TaskCheckBox` case (before the `default`):

```go
		case *xast.TaskCheckBox:
			mark := "[ ] "
			if t.IsChecked {
				mark = "[x] "
			}
			runs = append(runs, Run{Text: mark, Bold: bold, Italic: italic})
```

In `blockFrom`, add a `*xast.Table` case (before the `default`) — degrade each row to a `" | "`-joined paragraph, held in a `Blockquote`:

```go
	case *xast.Table:
		var rows []Block
		for r := n.FirstChild(); r != nil; r = r.NextSibling() {
			var cells []string
			for c := r.FirstChild(); c != nil; c = c.NextSibling() {
				cells = append(cells, plainText(inlineRuns(c, src, 0)))
			}
			rows = append(rows, Paragraph{Runs: []Run{{Text: strings.Join(cells, " | ")}}})
		}
		return Blockquote{Children: rows}, false
```

Add `"strings"` to `export_ast.go`'s imports (used by `strings.Join`).

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestParseSection' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_ast.go export_ast_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add export_ast.go export_ast_test.go
git commit -m "export: degrade GFM task lists ([ ]/[x]), tables (pipe rows), strikethrough"
```

---

## Task 3: Writers render the `Endnotes` block (`export_rtf.go`, `export_pdf.go`)

**Files:**
- Modify: `export_rtf.go` (`writeBlockRTF`), `export_pdf.go` (`writeBlockPDF`)
- Test: `export_rtf_test.go`, `export_pdf_test.go`

**Interfaces:**
- Consumes: `Endnotes`/`Endnote` (Task 1), `runsRTF` (existing), `plainText`/`pdfEnc` (existing).

- [ ] **Step 1: Write the failing tests**

Add to `export_rtf_test.go`:

```go
func TestWriteRTFEndnotes(t *testing.T) {
	doc := ManuscriptDoc{{Title: "ch", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Body [1]."}}},
		Endnotes{Items: []Endnote{{Num: 1, Runs: []Run{{Text: "the note"}}}}},
	}}}
	out := string(writeRTF(doc, StyleManuscript, Meta{Title: "T"}))
	if !strings.Contains(out, "Notes") || !strings.Contains(out, "1. the note") {
		t.Fatalf("RTF should render a Notes section with the endnote:\n%s", out)
	}
}
```

Add to `export_pdf_test.go`:

```go
func TestWritePDFEndnotesBuilds(t *testing.T) {
	doc := ManuscriptDoc{{Title: "ch", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Body [1]."}}},
		Endnotes{Items: []Endnote{{Num: 1, Runs: []Run{{Text: "the note"}}}}},
	}}}
	for _, st := range []ExportStyle{StyleManuscript, StyleTufte} {
		out, err := writePDF(doc, st, Meta{Title: "T"})
		if err != nil {
			t.Fatalf("style %d: endnotes PDF should build: %v", st, err)
		}
		if !bytes.HasPrefix(out, []byte("%PDF")) {
			t.Fatalf("style %d: not a PDF", st)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestWriteRTFEndnotes|TestWritePDFEndnotesBuilds' 2>&1 | tail`
Expected: FAIL — `Endnotes` not rendered (the `switch` has no case, so the block is silently skipped → "Notes" absent).

- [ ] **Step 3: Add the `Endnotes` case to each writer**

In `export_rtf.go` `writeBlockRTF`, add a case to the `switch` (e.g. after `case Heading:`):

```go
	case Endnotes:
		b.WriteString(`{\pard\sb360\sa120\b Notes\b0\par}` + "\n")
		for _, e := range v.Items {
			fmt.Fprintf(b, `{\pard\fi-360\li360 %d. %s\par}`+"\n", e.Num, runsRTF(e.Runs))
		}
```

In `export_pdf.go` `writeBlockPDF`, add a case to the `switch` (e.g. after `case Heading:`):

```go
	case Endnotes:
		pdf.Ln(cfg.lineHeight)
		pdf.SetFont(cfg.font, "B", cfg.bodySize)
		pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, "Notes"), "", "L", false)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
		for _, e := range v.Items {
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, fmt.Sprintf("%d. %s", e.Num, plainText(e.Runs))), "", "L", false)
		}
```

(`fmt` is already imported in both files.)

- [ ] **Step 4: Run the tests; full suite; build; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestWriteRTF|TestWritePDF' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_rtf.go export_pdf.go export_rtf_test.go export_pdf_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add export_rtf.go export_pdf.go export_rtf_test.go export_pdf_test.go
git commit -m "export: render the Endnotes block (Notes section) in RTF + PDF"
```

---

## Task 4: Update `CLAUDE.md` §2

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:** none.

- [ ] **Step 1: Clear the "export update pending" note in §2**

In `CLAUDE.md`, Shared Contracts §2 (Markdown flavor), replace the `*Status:*` paragraph (which says the export parser uses `goldmark.DefaultParser()` / CommonMark only and a follow-up is pending) with:

```markdown
- *Status:* the **export** parser (`export_ast.go`) uses `goldmark` + **GFM + Footnote**
  extensions (matching the app); footnotes export as per-chapter endnotes; the rarer GFM
  constructs degrade (tables → pipe rows, strikethrough → plain, task lists → `[ ]`/`[x]`).
  The `ctrl+p` **preview** uses glamour, which renders GFM but exposes no hook to add the
  footnote extension — so footnote syntax shows literally in the preview (known limitation).
```

Also remove `(a) atomic writes` and `(b) GFM + footnotes` from the "Adopted-but-pending follow-ups" line in the Working Agreement if both are now done — atomic writes shipped earlier and GFM+footnotes ships here; delete that bullet (or leave a note that both are complete).

- [ ] **Step 2: Verify build + full suite; commit**

```bash
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add CLAUDE.md
git commit -m "docs: CLAUDE.md — export now GFM+footnotes; preview footnote limitation noted"
```

---

## Self-Review

**Spec coverage:**
- Parser → GFM+Footnote (`exportParser`) → Task 1.
- Footnotes → per-chapter endnotes (`FootnoteLink` → `[N]`, `FootnoteList` → `Endnotes`) → Task 1; rendered in the writers → Task 3.
- GFM degrade (strikethrough plain — default recurse; task list `[ ]`/`[x]`; table → pipe-row Blockquote) → Task 2.
- Preview unchanged + `CLAUDE.md` §2 note → Task 4.

**Placeholder scan:** none — full code in every code step.

**Type consistency:** `Endnote{Num int; Runs []Run}` and `Endnotes{Items []Endnote}` defined in Task 1, consumed by Task 3's writers; `exportParser` (Task 1) used by `parseSection`; `xast` alias introduced in Task 1, reused in Task 2; `plainText`/`runsRTF`/`pdfEnc` reused from existing writers; `itemRuns` reused for endnote bodies and list items.

**Ordering note:** Task 1 switches the parser, after which GFM nodes (strikethrough/table/taskcheckbox) appear but **degrade safely via the existing `default` recurse** until Task 2 refines them — so Task 1 leaves the suite green on its own. The existing export tests use plain markdown, which parses identically under the extended parser.

**Degrade-via-default caveat (for the executor):** `*xast.Strikethrough` is intentionally NOT given its own case — the `default` branch in `inlineRuns` recurses into it and emits its child text as plain runs, which is the desired "plain text" degrade (Task 2's test asserts this). Don't add a Strikethrough case.
