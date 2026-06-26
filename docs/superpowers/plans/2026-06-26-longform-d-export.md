# Long-form Plan D — Export (RTF + PDF) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** a recurring generation bug emits a stray
> `court` token and/or drops the `antml:` namespace, silently no-op'ing tool calls.
> Mitigation: one tool call per message, as the FIRST element of the reply, explanation
> AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** A `ctrl+e` export that writes an editable RTF and a printable PDF of the current document (from the editor) or the whole manuscript (from the outline), in a Manuscript (Courier, double-spaced) or Tufte (elegant serif) style.

**Architecture:** goldmark parses each section into one small `ManuscriptDoc` AST; two writers consume the SAME AST — `writeRTF` (pure string emission, no library) and `writePDF` (`codeberg.org/go-pdf/fpdf`, pure Go). Scope = how many `Section`s you build; style is a render-time parameter. ET Book is embedded for the Tufte PDF.

**Tech Stack:** Go 1.24, goldmark v1.7.13 (promote to direct), `codeberg.org/go-pdf/fpdf` v0.12.0 (pure Go), `golang.org/x/text/encoding/charmap` (promote to direct), `//go:embed` for fonts.

**Design spec:** `docs/superpowers/specs/2026-06-26-export-plan-d-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt` (not on PATH). All deps (`fpdf` v0.12.0, goldmark v1.7.13, `x/text` v0.30.0) are already in the module cache.
- **Two writers over one AST** (`ManuscriptDoc`); **scope** = Section count (single doc = 1; whole = `orderedSections` sections, loose dropped); **style** (`StyleManuscript` | `StyleTufte`) = render-time only.
- **PDF lib:** `codeberg.org/go-pdf/fpdf` v0.12.0 — NOT the archived `github.com/go-pdf/fpdf`. Pure Go.
- **Smart-quotes × core fonts:** the editor inserts curly quotes/em-dashes; fpdf core fonts (Courier) are cp1252 — the manuscript PDF path MUST transcode UTF-8→cp1252 via `x/text/charmap` (ASCII-fallback un-encodable runes). The Tufte PDF (embedded ET Book, UTF-8) and both RTF paths do NOT transcode.
- **RTF escaping** (verified): `\`/`{`/`}` backslash-escaped; rune <0x80 literal; `0x80..0xFFFF` → `\u<signed int16>?`; `>0xFFFF` → UTF-16 surrogate pair (two `\u`). `\fs` is half-points (12pt=`\fs24`); dimensions are twips (1440/in). `\pard` reset every paragraph; matched `{}` groups for styled runs.
- **fpdf `MultiCell` can't change font mid-cell:** emphasis-bearing paragraphs render via `pdf.HTMLBasicNew().Write` (loses the first-line indent); plain paragraphs use indented `MultiCell`.
- **goldmark traps:** re-insert a space on `Text.SoftLineBreak()`; iterate top-level children (no `ast.Walk`); drop a leading H1 (filename-title dup); lone `#` (`Heading` with `ChildCount()==0`) → scene break.
- US Letter; single-doc title = `sectionTitle(filename)`, manuscript title = `projectTitle(folder)`; Author = `OKASHI_AUTHOR`. `ctrl+e` writes both `<slug>.rtf` + `<slug>.pdf` into `<dir>/export/`.
- Tests hermetic: `t.TempDir()` / `t.Setenv`. `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Parse → ManuscriptDoc AST (`export_ast.go`)

**Files:**
- Create: `export_ast.go`, `export_ast_test.go`
- Modify: `go.mod` (promote goldmark to a direct require)

**Interfaces:**
- Consumes: `fileEntry`, `sectionTitle`, `orderedSections` (existing); goldmark.
- Produces (package `main`): `ExportStyle` (`StyleManuscript`/`StyleTufte`), `Meta{Author,Title}`, `Run{Text,Bold,Italic}`, `Block` + `Paragraph`/`Heading`/`Blockquote`/`List`/`SceneBreak`, `Section{Title,Blocks}`, `ManuscriptDoc`, `func parseSection(src []byte) []Block`, `func manuscriptDoc(dir string, sections []fileEntry) ManuscriptDoc`.

- [ ] **Step 1: Write the failing tests**

Create `export_ast_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSectionBlocksAndRuns(t *testing.T) {
	src := []byte("# Dropped Title\n\nHello **bold** and *italic* world.\n\n---\n\nNext para.\n")
	blocks := parseSection(src)
	// Leading H1 dropped -> [Paragraph, SceneBreak, Paragraph]
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3 (H1 dropped): %#v", len(blocks), blocks)
	}
	p, ok := blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("block 0 should be a Paragraph, got %T", blocks[0])
	}
	var bold, italic bool
	for _, r := range p.Runs {
		if r.Text == "bold" && r.Bold {
			bold = true
		}
		if r.Text == "italic" && r.Italic {
			italic = true
		}
	}
	if !bold || !italic {
		t.Fatalf("paragraph runs should mark bold+italic: %#v", p.Runs)
	}
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("--- should be a SceneBreak, got %T", blocks[1])
	}
}

func TestParseSectionSoftBreakKeepsSpace(t *testing.T) {
	// A soft-wrapped paragraph: goldmark strips the newline; we must re-insert a space.
	blocks := parseSection([]byte("alpha\nbeta\n"))
	p := blocks[0].(Paragraph)
	var joined string
	for _, r := range p.Runs {
		joined += r.Text
	}
	if joined != "alpha beta" {
		t.Fatalf("soft break should keep a space: %q", joined)
	}
}

func TestParseSectionLoneHashIsSceneBreak(t *testing.T) {
	blocks := parseSection([]byte("para one\n\n#\n\npara two\n"))
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("a lone # should be a SceneBreak, got %T", blocks[1])
	}
}

func TestManuscriptDocExcludesLooseAndTitlesFromFilename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("first"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("second"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644)
	sections, _ := orderedSections([]fileEntry{
		{name: "01-opening.md"}, {name: "02-the-letter.md"}, {name: "notes.md"},
	})
	doc := manuscriptDoc(dir, sections)
	if len(doc) != 2 {
		t.Fatalf("manuscriptDoc should have 2 sections (loose excluded), got %d", len(doc))
	}
	if doc[0].Title != "opening" || doc[1].Title != "the letter" {
		t.Fatalf("titles should come from the filename: %q, %q", doc[0].Title, doc[1].Title)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestParseSection|TestManuscriptDoc' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `export_ast.go`**

```go
package main

import (
	"os"
	"path/filepath"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ExportStyle selects the render-time typography; the AST is style-agnostic.
type ExportStyle int

const (
	StyleManuscript ExportStyle = iota
	StyleTufte
)

// Meta is the document-level metadata both writers stamp in.
type Meta struct {
	Author string
	Title  string
}

// Run is a span of text with emphasis flags.
type Run struct {
	Text   string
	Bold   bool
	Italic bool
}

// Block is one prose block; the goldmark walk produces these, both writers consume them.
type Block interface{ isBlock() }

type Paragraph struct{ Runs []Run }
type Heading struct {
	Level int
	Runs  []Run
}
type Blockquote struct{ Children []Block }
type List struct {
	Ordered bool
	Start   int
	Items   []Paragraph
}
type SceneBreak struct{}

func (Paragraph) isBlock()  {}
func (Heading) isBlock()    {}
func (Blockquote) isBlock() {}
func (List) isBlock()       {}
func (SceneBreak) isBlock() {}

// Section is one chapter; Title comes from the FILENAME, never the content.
type Section struct {
	Title  string
	Blocks []Block
}

// ManuscriptDoc is the whole export payload — one Section for a single doc, or one per
// ordered section for a whole manuscript.
type ManuscriptDoc []Section

// parseSection parses a section's markdown into our block subset.
func parseSection(src []byte) []Block {
	root := goldmark.DefaultParser().Parse(text.NewReader(src))
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

// blockFrom converts one top-level goldmark node into a Block. drop=true skips it
// (a leading H1 that would duplicate the filename title).
func blockFrom(n ast.Node, src []byte, isFirst bool) (Block, bool) {
	switch t := n.(type) {
	case *ast.Heading:
		if t.ChildCount() == 0 { // a lone "#"
			return SceneBreak{}, false
		}
		if isFirst && t.Level == 1 {
			return nil, true
		}
		return Heading{Level: t.Level, Runs: inlineRuns(n, src, 0)}, false
	case *ast.ThematicBreak:
		return SceneBreak{}, false
	case *ast.Paragraph, *ast.TextBlock:
		return Paragraph{Runs: inlineRuns(n, src, 0)}, false
	case *ast.Blockquote:
		var ch []Block
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			b, drop := blockFrom(c, src, false)
			if !drop && b != nil {
				ch = append(ch, b)
			}
		}
		return Blockquote{Children: ch}, false
	case *ast.List:
		lst := List{Ordered: t.IsOrdered()}
		if lst.Ordered {
			lst.Start = t.Start
		}
		for li := n.FirstChild(); li != nil; li = li.NextSibling() {
			lst.Items = append(lst.Items, Paragraph{Runs: inlineRuns(li, src, 0)})
		}
		return lst, false
	default:
		runs := inlineRuns(n, src, 0)
		if len(runs) == 0 {
			return nil, true
		}
		return Paragraph{Runs: runs}, false
	}
}

// inlineRuns flattens inline children into styled runs. emph is a bitmask: 1=italic, 2=bold.
// Link/Image/CodeSpan/unknown degrade to their child text.
func inlineRuns(n ast.Node, src []byte, emph int) []Run {
	var runs []Run
	bold, italic := emph&2 != 0, emph&1 != 0
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch t := c.(type) {
		case *ast.Text:
			runs = append(runs, Run{Text: string(t.Segment.Value(src)), Bold: bold, Italic: italic})
			if t.SoftLineBreak() {
				runs = append(runs, Run{Text: " ", Bold: bold, Italic: italic})
			}
		case *ast.String:
			runs = append(runs, Run{Text: string(t.Value), Bold: bold, Italic: italic})
		case *ast.Emphasis:
			bit := 1
			if t.Level == 2 {
				bit = 2
			}
			runs = append(runs, inlineRuns(c, src, emph|bit)...)
		default:
			runs = append(runs, inlineRuns(c, src, emph)...)
		}
	}
	return runs
}

// manuscriptDoc builds the doc from ordered section files (loose already excluded by the
// caller). Title comes from each filename via sectionTitle.
func manuscriptDoc(dir string, sections []fileEntry) ManuscriptDoc {
	var doc ManuscriptDoc
	for _, s := range sections {
		data, err := os.ReadFile(filepath.Join(dir, s.name))
		if err != nil {
			continue
		}
		doc = append(doc, Section{Title: sectionTitle(s.name), Blocks: parseSection(data)})
	}
	return doc
}
```

- [ ] **Step 4: Promote goldmark to a direct require; run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go get github.com/yuin/goldmark@v1.7.13
/opt/homebrew/bin/go test . -run 'TestParseSection|TestManuscriptDoc' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_ast.go export_ast_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add export_ast.go export_ast_test.go go.mod go.sum
git commit -m "export: goldmark parse -> ManuscriptDoc AST (prose-block subset)"
```

---

## Task 2: RTF writer (`export_rtf.go`)

**Files:**
- Create: `export_rtf.go`, `export_rtf_test.go`

**Interfaces:**
- Consumes: `ExportStyle`, `Meta`, `Run`, `Block`/`Paragraph`/`Heading`/`Blockquote`/`List`/`SceneBreak`, `ManuscriptDoc` (Task 1).
- Produces: `func writeRTF(doc ManuscriptDoc, st ExportStyle, meta Meta) []byte`, `func rtfEscape(s string) string`.

- [ ] **Step 1: Write the failing tests**

Create `export_rtf_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestRTFEscape(t *testing.T) {
	// Input uses explicit Unicode escapes so the test source can't be mojibaked:
	// U+201C/U+201D curly quotes, U+2014 em dash, plus a brace and a backslash.
	got := rtfEscape("a\\b{c} \u201cq\u201d \u2014")
	// int16(0x201C)=8220, int16(0x201D)=8221, int16(0x2014)=8212.
	for _, want := range []string{`\\`, `\{`, `\}`, `\u8220?`, `\u8221?`, `\u8212?`} {
		if !strings.Contains(got, want) {
			t.Fatalf("escape missing %q in %q", want, got)
		}
	}
}

func TestWriteRTFManuscriptControlWords(t *testing.T) {
	doc := ManuscriptDoc{{Title: "opening", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Hello "}, {Text: "world", Bold: true}}},
	}}}
	out := string(writeRTF(doc, StyleManuscript, Meta{Author: "Doe", Title: "The Garden"}))
	for _, want := range []string{`\rtf1`, `\sl480\slmult1`, `\fi720`, `\chpgn`, `\page`, `{\b world}`, "THE GARDEN"} {
		if !strings.Contains(out, want) {
			t.Fatalf("manuscript RTF missing %q", want)
		}
	}
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Fatalf("unbalanced braces in RTF")
	}
}

func TestWriteRTFTufteDiffers(t *testing.T) {
	doc := ManuscriptDoc{{Title: "opening", Blocks: []Block{Paragraph{Runs: []Run{{Text: "x"}}}}}}
	out := string(writeRTF(doc, StyleTufte, Meta{Title: "T"}))
	if !strings.Contains(out, `\f1`) { // serif body
		t.Fatalf("tufte RTF should select the serif font \\f1")
	}
	if strings.Contains(out, `\chpgn`) {
		t.Fatalf("tufte RTF should omit the manuscript running header")
	}
	if !strings.Contains(out, `\margl2160`) {
		t.Fatalf("tufte RTF should use the wider Tufte margins")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestRTFEscape|TestWriteRTF' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `export_rtf.go`**

```go
package main

import (
	"fmt"
	"strings"
)

// rtfEscape escapes one text string for RTF: \ { } are backslash-escaped; runs >=0x80
// become \u<signed-16-bit>? (astral runes as a UTF-16 surrogate pair). Verified by
// round-trip through macOS textutil.
func rtfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '{':
			b.WriteString(`\{`)
		case r == '}':
			b.WriteString(`\}`)
		case r < 0x80:
			b.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&b, `\u%d?`, int16(r))
		default:
			c := r - 0x10000
			hi := 0xD800 + (c >> 10)
			lo := 0xDC00 + (c & 0x3FF)
			fmt.Fprintf(&b, `\u%d?\u%d?`, int16(hi), int16(lo))
		}
	}
	return b.String()
}

// runsRTF emits styled runs as matched {...} groups so style can't leak across paragraphs.
func runsRTF(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		txt := rtfEscape(r.Text)
		switch {
		case r.Bold && r.Italic:
			fmt.Fprintf(&b, `{\b\i %s}`, txt)
		case r.Bold:
			fmt.Fprintf(&b, `{\b %s}`, txt)
		case r.Italic:
			fmt.Fprintf(&b, `{\i %s}`, txt)
		default:
			b.WriteString(txt)
		}
	}
	return b.String()
}

// writeRTF renders the doc to RTF bytes in the given style.
func writeRTF(doc ManuscriptDoc, st ExportStyle, meta Meta) []byte {
	var b strings.Builder
	b.WriteString(`{\rtf1\ansi\ansicpg1252\deff0\uc1` + "\n")
	b.WriteString(`{\fonttbl{\f0\fmodern\fcharset0 Courier New;}{\f1\froman\fcharset0 Georgia;}}` + "\n")
	b.WriteString(`\paperw12240\paperh15840`)
	if st == StyleTufte {
		b.WriteString(`\margl2160\margr2880\margt1440\margb1440` + "\n")
	} else {
		b.WriteString(`\margl1440\margr1440\margt1440\margb1440` + "\n")
		fmt.Fprintf(&b, `{\header\pard\qr\f0\fs24 %s / %s / \chpgn\par}`+"\n",
			rtfEscape(meta.Author), rtfEscape(strings.ToUpper(meta.Title)))
	}
	if st == StyleTufte {
		b.WriteString(`\f1\fs24` + "\n")
	} else {
		b.WriteString(`\f0\fs24` + "\n")
	}
	for _, sec := range doc {
		b.WriteString(`\page` + "\n")
		fmt.Fprintf(&b, `{\pard\qc\sb480\sa240\b %s\b0\par}`+"\n", rtfEscape(sec.Title))
		for _, blk := range sec.Blocks {
			writeBlockRTF(&b, blk, st)
		}
	}
	b.WriteString("}")
	return []byte(b.String())
}

func writeBlockRTF(b *strings.Builder, blk Block, st ExportStyle) {
	para := `\pard\fi720\sl480\slmult1 ` // manuscript: 0.5" indent, double-spaced
	if st == StyleTufte {
		para = `\pard\fi360\sl276\slmult1 ` // tufte: 0.25" indent, ~1.15 leading
	}
	switch v := blk.(type) {
	case Paragraph:
		fmt.Fprintf(b, "%s%s\\par\n", para, runsRTF(v.Runs))
	case Heading:
		fs := 28 - 2*v.Level
		if fs < 20 {
			fs = 20
		}
		fmt.Fprintf(b, `{\pard\sb240\sa120\b\fs%d %s\b0\par}`+"\n", fs, runsRTF(v.Runs))
	case SceneBreak:
		b.WriteString(`{\pard\qc\sb240\sa240 #\par}` + "\n")
	case Blockquote:
		for _, c := range v.Children {
			if p, ok := c.(Paragraph); ok {
				fmt.Fprintf(b, `\pard\li720\ri720\sl480\slmult1 %s\par`+"\n", runsRTF(p.Runs))
			} else {
				writeBlockRTF(b, c, st)
			}
		}
	case List:
		for i, it := range v.Items {
			marker := `\bullet  `
			if v.Ordered {
				marker = fmt.Sprintf("%d.  ", v.Start+i)
			}
			fmt.Fprintf(b, `{\pard\fi-360\li720 %s%s\par}`+"\n", marker, runsRTF(it.Runs))
		}
	}
}
```

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestRTFEscape|TestWriteRTF' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_rtf.go export_rtf_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add export_rtf.go export_rtf_test.go
git commit -m "export: RTF writer (manuscript + tufte styles, verified escaping)"
```

---

## Task 3: PDF writer — Manuscript style (`export_pdf.go`)

**Files:**
- Create: `export_pdf.go`, `export_pdf_test.go`
- Modify: `go.mod` (add `codeberg.org/go-pdf/fpdf`, promote `golang.org/x/text`)

**Interfaces:**
- Consumes: the AST types (Task 1); `fpdf`, `charmap`.
- Produces: `func writePDF(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error)` (Tufte branch added in Task 4), plus helpers `cp1252`, `pdfEnc`, `plainText`, `hasEmphasis`, `runsHTML`. This task implements `StyleManuscript` fully; `StyleTufte` temporarily uses the Courier path (Task 4 replaces it with ET Book).

- [ ] **Step 1: Write the failing tests**

Create `export_pdf_test.go`:

```go
package main

import (
	"bytes"
	"testing"
)

func TestWritePDFManuscriptValid(t *testing.T) {
	doc := ManuscriptDoc{
		{Title: "one", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Plain line."}}}}},
		{Title: "two", Blocks: []Block{Paragraph{Runs: []Run{{Text: "Has "}, {Text: "bold", Bold: true}}}}},
	}
	out, err := writePDF(doc, StyleManuscript, Meta{Author: "Doe", Title: "The Garden"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (no %%PDF header)")
	}
	if len(out) < 800 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(out))
	}
}

func TestWritePDFSmartQuotesNoError(t *testing.T) {
	// The editor inserts curly quotes/em dashes; the Courier (cp1252) path must transcode.
	doc := ManuscriptDoc{{Title: "q", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "“Curly” — dashes — and an é."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("smart-quote text should not error on the Courier path: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestCp1252FallsBackOnUnencodable(t *testing.T) {
	// An astral emoji has no cp1252 encoding -> must fall back, not panic.
	got := cp1252("hi \U0001F600")
	if got == "" {
		t.Fatal("cp1252 returned empty")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestWritePDF|TestCp1252' 2>&1 | tail`
Expected: build error — undefined / missing import.

- [ ] **Step 3: Implement `export_pdf.go`**

```go
package main

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"codeberg.org/go-pdf/fpdf"
	"golang.org/x/text/encoding/charmap"
)

// cp1252 transcodes UTF-8 to Windows-1252 for fpdf's core fonts (Courier/Times), which
// can't render raw UTF-8. Un-encodable runes (e.g. emoji) fall back to '?'.
func cp1252(s string) string {
	enc := charmap.Windows1252.NewEncoder()
	if out, err := enc.String(s); err == nil {
		return out
	}
	var b strings.Builder
	for _, r := range s {
		if e, err := charmap.Windows1252.NewEncoder().String(string(r)); err == nil {
			b.WriteString(e)
		} else {
			b.WriteByte('?')
		}
	}
	return b.String()
}

// pdfEnc transcodes for the core-font (manuscript Courier) path; the Tufte path (embedded
// UTF-8 ET Book) passes through unchanged.
func pdfEnc(st ExportStyle, s string) string {
	if st == StyleTufte {
		return s
	}
	return cp1252(s)
}

func plainText(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

func hasEmphasis(runs []Run) bool {
	for _, r := range runs {
		if r.Bold || r.Italic {
			return true
		}
	}
	return false
}

// runsHTML renders runs as the minimal HTML fpdf's HTMLBasic writer understands.
func runsHTML(runs []Run) string {
	var b strings.Builder
	for _, r := range runs {
		txt := html.EscapeString(r.Text)
		if r.Bold {
			txt = "<b>" + txt + "</b>"
		}
		if r.Italic {
			txt = "<i>" + txt + "</i>"
		}
		b.WriteString(txt)
	}
	return b.String()
}

// pdfStyle holds the per-style typography knobs.
type pdfStyle struct {
	font       string
	bodySize   float64
	titleSize  float64
	lineHeight float64
	indent     string
}

func writePDF(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error) {
	pdf := fpdf.New("P", "pt", "Letter", "")
	cfg := pdfStyle{font: "Courier", bodySize: 12, titleSize: 14, lineHeight: 24, indent: "     "}
	if st == StyleTufte {
		// Task 4 replaces this with the embedded ET Book serif + Tufte metrics.
		cfg = pdfStyle{font: "Times", bodySize: 12, titleSize: 14, lineHeight: 17, indent: ""}
		pdf.SetMargins(108, 90, 108)
	} else {
		pdf.SetMargins(72, 72, 72)
		pdf.AliasNbPages("{nb}")
		pdf.SetHeaderFunc(func() {
			pdf.SetFont("Courier", "", 12)
			hdr := fmt.Sprintf("%s / %s / %d", meta.Author, strings.ToUpper(meta.Title), pdf.PageNo())
			pdf.CellFormat(0, 14, pdfEnc(st, hdr), "", 0, "R", false, 0, "")
			pdf.Ln(24)
		})
	}
	pdf.SetAutoPageBreak(true, 72)

	for _, sec := range doc {
		pdf.AddPage()
		pdf.SetFont(cfg.font, "B", cfg.titleSize)
		pdf.CellFormat(0, cfg.lineHeight, pdfEnc(st, sec.Title), "", 1, "C", false, 0, "")
		pdf.Ln(cfg.lineHeight)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
		for _, blk := range sec.Blocks {
			writeBlockPDF(pdf, blk, st, cfg)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeBlockPDF(pdf *fpdf.Fpdf, blk Block, st ExportStyle, cfg pdfStyle) {
	switch v := blk.(type) {
	case Paragraph:
		if hasEmphasis(v.Runs) {
			// MultiCell can't switch font mid-cell, so emphasis routes through the HTML
			// writer (which can't carry the first-line indent).
			pdf.HTMLBasicNew().Write(cfg.lineHeight, pdfEnc(st, runsHTML(v.Runs)))
			pdf.Ln(cfg.lineHeight)
		} else {
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, cfg.indent+plainText(v.Runs)), "", "L", false)
		}
	case Heading:
		pdf.SetFont(cfg.font, "B", 13)
		pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, plainText(v.Runs)), "", "L", false)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
	case SceneBreak:
		pdf.CellFormat(0, cfg.lineHeight, "#", "", 1, "C", false, 0, "")
	case Blockquote:
		for _, c := range v.Children {
			writeBlockPDF(pdf, c, st, cfg)
		}
	case List:
		for i, it := range v.Items {
			marker := "- "
			if v.Ordered {
				marker = fmt.Sprintf("%d. ", v.Start+i)
			}
			pdf.MultiCell(0, cfg.lineHeight, pdfEnc(st, marker+plainText(it.Runs)), "", "L", false)
		}
	}
}
```

- [ ] **Step 4: Add deps; run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go get codeberg.org/go-pdf/fpdf@v0.12.0
/opt/homebrew/bin/go get golang.org/x/text@v0.30.0
/opt/homebrew/bin/go test . -run 'TestWritePDF|TestCp1252' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export_pdf.go export_pdf_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add export_pdf.go export_pdf_test.go go.mod go.sum
git commit -m "export: PDF writer — manuscript style (Courier, cp1252 transcode, fpdf)"
```

---

## Task 4: ET Book fonts + Tufte PDF (`fonts.go`, `export_pdf.go`)

**Files:**
- Create: `assets/etbook/etbook-roman.ttf`, `-bold.ttf`, `-italic.ttf`, `-bolditalic.ttf`, `LICENSE`; `fonts.go`
- Modify: `export_pdf.go` (Tufte branch)
- Test: `export_pdf_test.go`

**Interfaces:**
- Consumes: `fpdf` (`AddUTF8FontFromBytes`).
- Produces: `func registerETBook(pdf *fpdf.Fpdf)`; `writePDF`'s `StyleTufte` branch uses the `etbook` family.

- [ ] **Step 1: Fetch the ET Book TTFs (network)**

Download the four TTFs + license from the upstream repo (`github.com/edwardtufte/et-book`, under `et-book/`). If the fetch isn't possible in this environment, STOP and report — this is the one network-dependent step; everything else is pure code.

```bash
mkdir -p /Users/michael/dev/okashi/assets/etbook
cd /Users/michael/dev/okashi/assets/etbook
base="https://raw.githubusercontent.com/edwardtufte/et-book/gh-pages/et-book"
curl -fsSL "$base/et-book-roman-line-figures/et-book-roman-line-figures.ttf"             -o etbook-roman.ttf
curl -fsSL "$base/et-book-bold-line-figures/et-book-bold-line-figures.ttf"               -o etbook-bold.ttf
curl -fsSL "$base/et-book-display-italic-old-style-figures/et-book-display-italic-old-style-figures.ttf" -o etbook-italic.ttf
curl -fsSL "$base/et-book-roman-old-style-figures/et-book-roman-old-style-figures.ttf"   -o etbook-bolditalic.ttf
curl -fsSL "https://raw.githubusercontent.com/edwardtufte/et-book/gh-pages/LICENSE"       -o LICENSE
ls -l
```
(If a weight path 404s, list the repo's `et-book/` dir and pick the nearest TTF; the four files just need to be valid TrueType. The bold-italic may be substituted by the roman if no true BI exists — note it.)

- [ ] **Step 2: Write the failing test**

Add to `export_pdf_test.go`:

```go
func TestWritePDFTufteEmbedsFontValid(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Elegant "}, {Text: "serif", Italic: true}, {Text: " prose — with an em dash."}}},
	}}}
	out, err := writePDF(doc, StyleTufte, Meta{Title: "Tufte"})
	if err != nil {
		t.Fatalf("tufte PDF should build with the embedded font: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	if len(out) < 5000 {
		t.Fatalf("an embedded-font PDF should be larger than %d bytes", len(out))
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestWritePDFTufteEmbeds' 2>&1 | tail`
Expected: FAIL — Tufte path still uses core "Times" (no `registerETBook`).

- [ ] **Step 4: Implement `fonts.go` and the Tufte branch**

Create `fonts.go`:

```go
package main

import (
	_ "embed"

	"codeberg.org/go-pdf/fpdf"
)

//go:embed assets/etbook/etbook-roman.ttf
var etbookRoman []byte

//go:embed assets/etbook/etbook-bold.ttf
var etbookBold []byte

//go:embed assets/etbook/etbook-italic.ttf
var etbookItalic []byte

//go:embed assets/etbook/etbook-bolditalic.ttf
var etbookBoldItalic []byte

// registerETBook registers the embedded ET Book TTFs as the "etbook" family (one TTF per
// style — fpdf has no synthetic bold).
func registerETBook(pdf *fpdf.Fpdf) {
	pdf.AddUTF8FontFromBytes("etbook", "", etbookRoman)
	pdf.AddUTF8FontFromBytes("etbook", "B", etbookBold)
	pdf.AddUTF8FontFromBytes("etbook", "I", etbookItalic)
	pdf.AddUTF8FontFromBytes("etbook", "BI", etbookBoldItalic)
}
```

In `export_pdf.go`, replace the Tufte branch in `writePDF`:

```go
	if st == StyleTufte {
		// Task 4 replaces this with the embedded ET Book serif + Tufte metrics.
		cfg = pdfStyle{font: "Times", bodySize: 12, titleSize: 14, lineHeight: 17, indent: ""}
		pdf.SetMargins(108, 90, 108)
	} else {
```

with:

```go
	if st == StyleTufte {
		registerETBook(pdf)
		cfg = pdfStyle{font: "etbook", bodySize: 12, titleSize: 16, lineHeight: 17, indent: ""}
		pdf.SetMargins(108, 90, 108)
	} else {
```

- [ ] **Step 5: Run tests; full suite; build; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestWritePDF' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w fonts.go export_pdf.go export_pdf_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add assets/etbook fonts.go export_pdf.go export_pdf_test.go
git commit -m "export: embed ET Book for the Tufte PDF style"
```

---

## Task 5: Wiring — ctrl+e chooser, scope, write both files (`export.go`, `main.go`)

**Files:**
- Create: `export.go`, `export_wiring_test.go`
- Modify: `main.go` (`model` struct, `Update` capture, editor + outline key cases, `statusBar`)

**Interfaces:**
- Consumes: `manuscriptDoc`, `parseSection`, `writeRTF`, `writePDF`, `ExportStyle`, `Meta`, `orderedSections`, `sectionTitle`, `projectTitle`, `slugify` (existing, rename.go), `m.files`, `m.currentFile`.
- Produces: `model.exportPrompt bool`; `func (m *model) runExport(st ExportStyle)`; `ctrl+e` raises the chooser in the editor and the outline.

- [ ] **Step 1: Write the failing tests**

Create `export_wiring_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExportSingleDocFromEditor(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("She wrote **back**."), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	m.currentFile = filepath.Join(proj, "02-the-letter.md")

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	if !m.exportPrompt {
		t.Fatal("ctrl+e should raise the export chooser")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// single-doc title = sectionTitle -> "the letter" -> slug "the-letter"
	rtf := filepath.Join(proj, "export", "the-letter.rtf")
	pdf := filepath.Join(proj, "export", "the-letter.pdf")
	if b, err := os.ReadFile(rtf); err != nil || !bytes.Contains(b, []byte(`\rtf1`)) {
		t.Fatalf("expected an RTF at %s: %v", rtf, err)
	}
	if b, err := os.ReadFile(pdf); err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("expected a PDF at %s: %v", pdf, err)
	}
}

func TestExportWholeManuscriptFromOutline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "my-novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("beta"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE}) // export chooser on the outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}) // tufte
	m = nm.(model)
	// whole-manuscript title = projectTitle("my-novel") = "my novel" -> slug "my-novel"
	if _, err := os.Stat(filepath.Join(proj, "export", "my-novel.pdf")); err != nil {
		t.Fatalf("expected the whole-manuscript PDF: %v", err)
	}
}

func TestExportCancel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "x.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(root)
	m.currentFile = filepath.Join(root, "x.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.exportPrompt {
		t.Fatal("esc should dismiss the export chooser")
	}
	if _, err := os.Stat(filepath.Join(root, "export")); !os.IsNotExist(err) {
		t.Fatal("cancel should write nothing")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestExport' 2>&1 | tail`
Expected: build error — `exportPrompt` / `runExport` undefined.

- [ ] **Step 3: Implement `export.go`**

```go
package main

import (
	"os"
	"path/filepath"
)

// runExport builds the export doc for the current scope (whole manuscript on the outline
// screen, else the current document) and writes <slug>.rtf + <slug>.pdf under <dir>/export/.
func (m *model) runExport(st ExportStyle) {
	dir := m.files.dir
	var doc ManuscriptDoc
	var title string
	if m.screen == screenOutline {
		sections, _ := orderedSections(m.files.entries)
		doc = manuscriptDoc(dir, sections)
		title = projectTitle(filepath.Base(dir))
	} else {
		if m.currentFile == "" {
			m.status = "nothing to export"
			return
		}
		dir = filepath.Dir(m.currentFile)
		base := filepath.Base(m.currentFile)
		data, err := os.ReadFile(m.currentFile)
		if err != nil {
			m.status = "export failed: " + err.Error()
			return
		}
		title = sectionTitle(base)
		doc = ManuscriptDoc{{Title: title, Blocks: parseSection(data)}}
	}
	if len(doc) == 0 {
		m.status = "nothing to export"
		return
	}

	meta := Meta{Author: os.Getenv("OKASHI_AUTHOR"), Title: title}
	outDir := filepath.Join(dir, "export")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	slug := slugify(title)
	rtfPath := filepath.Join(outDir, slug+".rtf")
	pdfPath := filepath.Join(outDir, slug+".pdf")
	if err := os.WriteFile(rtfPath, writeRTF(doc, st, meta), 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	pdfBytes, err := writePDF(doc, st, meta)
	if err != nil {
		m.status = "export failed (pdf): " + err.Error()
		return
	}
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	m.status = "exported " + slug + ".rtf + .pdf to export/"
}
```

- [ ] **Step 4: Wire the chooser into `main.go`**

Add to the `model` struct (after `convertPrompt bool`):

```go
	exportPrompt bool
```

Add the capture block in `Update`, right after the `if m.convertPrompt { … }` block:

```go
	if m.exportPrompt {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "m":
				m.exportPrompt = false
				m.runExport(StyleManuscript)
				return m, nil
			case "t":
				m.exportPrompt = false
				m.runExport(StyleTufte)
				return m, nil
			case "esc":
				m.exportPrompt = false
				m.status = "export cancelled"
				return m, nil
			}
		}
		return m, nil
	}
```

In the editor `case tea.KeyMsg` switch (alongside `ctrl+l`), add:

```go
		case "ctrl+e":
			m.exportPrompt = true
			m.status = "export: m manuscript · t tufte · esc cancel"
			return m, nil
```

In `updateOutline`'s key switch (alongside `m`, `n`), add a parallel case. But the outline's capture must run too: add the SAME `if m.exportPrompt { … }` block at the top of `updateOutline` (right after the `if m.renaming { … }` block), then add:

```go
	case "ctrl+e":
		m.exportPrompt = true
		m.status = "export: m manuscript · t tufte · esc cancel"
```

In `statusBar`, add right after the `if m.convertPrompt { … }` block:

```go
	if m.exportPrompt {
		return "export: m manuscript · t tufte · esc cancel"
	}
```

- [ ] **Step 5: Run tests; full suite; build; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestExport' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w export.go main.go export_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add export.go main.go export_wiring_test.go
git commit -m "export: ctrl+e chooser (manuscript/tufte), scope from screen, writes rtf+pdf"
```

---

## Task 6: Docs & status line

**Files:**
- Modify: `main.go` (editor + outline status strings), `README.md`, `.gitignore` (ignore exported output)

**Interfaces:** none.

- [ ] **Step 1: Add the `ctrl+e` hint to the status strings**

In `initialModel`, insert `· ctrl+e export` into the editor status string (after `ctrl+l outline`). In `enterOutline`, append ` · ctrl+e export` to the outline status string.

- [ ] **Step 2: Ignore exported output**

Append to `.gitignore` (create if absent):

```
# exported manuscripts
**/export/
```

- [ ] **Step 3: Document export in `README.md`**

Read `README.md` first to match its style. Add near the outline/pager sections:

```markdown
### Export (RTF + PDF)

Press **ctrl+e**, then choose a style — **m** Manuscript (Courier, double-spaced, the
agent/editor submission format) or **t** Tufte (elegant serif, for a printable/readable
copy). From the editor it exports the current document; from the outline it exports the
whole manuscript. Both an editable `.rtf` and a printable `.pdf` are written to
`<project>/export/`. Set `OKASHI_AUTHOR` for the manuscript running header.
```

- [ ] **Step 4: Verify build + full suite; commit**

```bash
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go README.md .gitignore
git commit -m "docs: export (ctrl+e) keymap + ignore exported output"
```

---

## Self-Review

**Spec coverage:**
- §1 AST (goldmark walk → block subset, soft-break space, drop leading H1, lone-# scene break, loose excluded, filename titles) → Task 1.
- §2 RTF writer (both styles, verified escaping/control words, `\page` per chapter, `\chpgn` header, styled-run groups) → Task 2.
- §3 PDF writer (fpdf, manuscript Courier + cp1252 transcode, emphasis via HTML writer, header/page numbers; Tufte ET Book) → Task 3 (manuscript) + Task 4 (Tufte/fonts).
- §4 ET Book assets (embed 4 TTFs + license) → Task 4.
- §5 Wiring (`ctrl+e` chooser, scope from screen, both files into `export/`, titles/author) → Task 5.
- Deps (goldmark direct, codeberg fpdf, x/text) → Tasks 1, 3.
- Docs/status/gitignore → Task 6.

**Placeholder scan:** none — full code in every step. Task 4 Step 1 is a real `curl` recipe with a documented fallback (the one network-dependent step, flagged to pause if it fails).

**Type consistency:** `ExportStyle`/`Meta`/`Run`/`Block`+variants/`Section`/`ManuscriptDoc` defined in Task 1, consumed by Tasks 2–5; `writeRTF(doc,st,meta)[]byte` (Task 2) and `writePDF(doc,st,meta)([]byte,error)` (Tasks 3–4) consumed by Task 5; `pdfStyle`/`pdfEnc`/`cp1252`/`plainText`/`hasEmphasis`/`runsHTML` consistent within Tasks 3–4; `registerETBook` (Task 4) called by `writePDF`'s Tufte branch; `model.exportPrompt`, `runExport` (Task 5) used by the capture blocks; `slugify` reused from `rename.go`.

**Cross-cutting checks baked into tests:** soft-break space + dropped H1 + lone-# (Task 1); RTF escaping of curly quotes/em-dash/braces + brace balance + style divergence (Task 2); manuscript PDF is a valid multi-page `%PDF` and smart-quote text doesn't error on the cp1252 path (Task 3); the Tufte PDF embeds the font and is larger (Task 4); ctrl+e writes both files at the right scope/title, esc cancels writing nothing (Task 5).

**Notes for the executor:**
- **Task 4 Step 1 (fonts) needs network.** It's the only non-pure-code step; if the ET Book TTFs can't be fetched, pause and surface it (the rest of the plan is unaffected). A bold-italic weight may be substituted if upstream lacks a true BI — note it.
- Task 5 adds the SAME `if m.exportPrompt {…}` capture in BOTH the writing-path `Update` and `updateOutline` (the chooser must work on either screen), exactly as the rename prompt did. `runExport` infers scope from `m.screen` (stable while the modal is up).
- All deps are already in the module cache at the pinned versions, so the `go get` steps resolve offline.
