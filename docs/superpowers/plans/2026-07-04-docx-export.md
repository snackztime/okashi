# DOCX export — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `.docx` (OOXML) export alongside okashi's RTF+PDF, from the existing style-agnostic export AST, in pure Go — the format agents/editors actually request.

**Architecture:** A `writeDOCX(doc, st, meta) ([]byte, error)` in `export_docx.go` mirrors `writeRTF` (`export_rtf.go`): map each `Block`/`Run` to OOXML, assemble a `.docx` zip with `archive/zip`. Wire into `runExport` so every export also emits `<slug>.docx`.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH). Stdlib only: `archive/zip`, `bytes`, `fmt`, `strings`.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`. Pure-Go, no cgo, no new deps.
- The export AST (`export_ast.go`) is the SAME one RTF/PDF use: `ManuscriptDoc []Section`; `Section{Title string; Blocks []Block}`; `Block` ∈ {`Paragraph{Runs []Run}`, `Heading{Level int; Runs []Run}`, `Blockquote{Children []Block}`, `List{Ordered bool; Start int; Items []Paragraph}`, `SceneBreak{}`, `Endnotes{Items []Endnote}`}; `Run{Text string; Bold, Italic bool}`; `Endnote{Num int; Runs []Run}`; `Meta{Author, Title string}`; `ExportStyle` ∈ {`StyleManuscript`, `StyleTufte`}. Read `export_rtf.go`'s `writeRTF`/`writeBlockRTF` and mirror the block handling exactly (same degrade decisions).
- All output writes go through `atomicWrite` (in `runExport`).
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` clean.

---

## Task 1: `writeDOCX` — the OOXML writer

**Files:** Create `export_docx.go`. Test: `export_docx_test.go`.

**Interfaces:** Produces `func writeDOCX(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error)`.

- [ ] **Step 1: Write the failing test**

Create `export_docx_test.go`:
```go
package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// docXML unzips a .docx and returns word/document.xml as a string (helper).
func docXML(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			var b bytes.Buffer
			b.ReadFrom(rc)
			rc.Close()
			return b.String()
		}
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func hasZipEntry(data []byte, name string) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

func TestWriteDOCXStructureAndContent(t *testing.T) {
	doc := ManuscriptDoc{{
		Title: "Chapter One",
		Blocks: []Block{
			Paragraph{Runs: []Run{{Text: "Plain "}, {Text: "bold", Bold: true}, {Text: " & tail."}}},
			SceneBreak{},
			Paragraph{Runs: []Run{{Text: "After the break."}}},
		},
	}}
	out, err := writeDOCX(doc, StyleManuscript, Meta{Author: "A. Writer", Title: "Book"})
	if err != nil {
		t.Fatal(err)
	}
	// Minimal package validity — Word needs these parts.
	for _, part := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if !hasZipEntry(out, part) {
			t.Fatalf("missing required part %q", part)
		}
	}
	xml := docXML(t, out)
	if !strings.Contains(xml, "Chapter One") {
		t.Fatalf("chapter title missing:\n%s", xml)
	}
	if !strings.Contains(xml, "<w:b/>") {
		t.Fatalf("bold run should emit <w:b/>")
	}
	if !strings.Contains(xml, "&amp;") {
		t.Fatalf("& must be XML-escaped to &amp;")
	}
	if !strings.Contains(xml, "After the break.") {
		t.Fatalf("post-scene-break text missing")
	}
	if !strings.Contains(xml, ">#<") { // the scene-break dinkus
		t.Fatalf("scene break # missing")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestWriteDOCXStructureAndContent -v`
Expected: FAIL (`writeDOCX` undefined).

- [ ] **Step 3: Implement `export_docx.go`**

```go
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

const (
	docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`

	docxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`

	docxSectPr = `<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr>`
)

// xmlEsc escapes text for an OOXML <w:t> value.
func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func docxFont(st ExportStyle) string {
	if st == StyleTufte {
		return "Georgia"
	}
	return "Times New Roman"
}

// docxRun renders a Run as a <w:r>.
func docxRun(r Run, font string) string {
	var pr strings.Builder
	pr.WriteString("<w:rPr>")
	if r.Bold {
		pr.WriteString("<w:b/>")
	}
	if r.Italic {
		pr.WriteString("<w:i/>")
	}
	fmt.Fprintf(&pr, `<w:rFonts w:ascii="%s" w:hAnsi="%s"/><w:sz w:val="24"/>`, font, font)
	pr.WriteString("</w:rPr>")
	return "<w:r>" + pr.String() + `<w:t xml:space="preserve">` + xmlEsc(r.Text) + "</w:t></w:r>"
}

// docxPara builds a <w:p>. pPrExtra is inserted inside <w:pPr> (alignment/indent/spacing/break).
func docxPara(runs []Run, font, pPrExtra string) string {
	var b strings.Builder
	b.WriteString("<w:p><w:pPr>")
	b.WriteString(pPrExtra)
	b.WriteString("</w:pPr>")
	for _, r := range runs {
		b.WriteString(docxRun(r, font))
	}
	b.WriteString("</w:p>")
	return b.String()
}

func writeDOCX(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error) {
	font := docxFont(st)
	bodyIndent := ""
	if st == StyleManuscript {
		bodyIndent = `<w:spacing w:line="480" w:lineRule="auto"/><w:ind w:firstLine="720"/>`
	}
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for i, sec := range doc {
		// Chapter title: centered, bold, new page (except the first).
		pageBreak := ""
		if i > 0 {
			pageBreak = "<w:pageBreakBefore/>"
		}
		body.WriteString(docxPara([]Run{{Text: sec.Title, Bold: true}}, font,
			pageBreak+`<w:jc w:val="center"/><w:spacing w:before="480" w:after="240"/>`))
		for _, blk := range sec.Blocks {
			writeBlockDOCX(&body, blk, font, bodyIndent)
		}
	}
	body.WriteString(docxSectPr + "</w:body></w:document>")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(content))
		return err
	}
	if err := add("[Content_Types].xml", docxContentTypes); err != nil {
		return nil, err
	}
	if err := add("_rels/.rels", docxRootRels); err != nil {
		return nil, err
	}
	if err := add("word/document.xml", body.String()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeBlockDOCX mirrors writeBlockRTF's block handling. Read export_rtf.go and match the same
// degrade decisions (lists → literal-prefixed paragraphs, blockquote → indented italic, endnotes →
// a "Notes" heading + numbered paras, scene break → centered #).
func writeBlockDOCX(b *strings.Builder, blk Block, font, bodyIndent string) {
	switch v := blk.(type) {
	case Paragraph:
		b.WriteString(docxPara(v.Runs, font, bodyIndent))
	case Heading:
		b.WriteString(docxPara(boldRuns(v.Runs), font, `<w:spacing w:before="240" w:after="120"/>`))
	case SceneBreak:
		b.WriteString(docxPara([]Run{{Text: "#"}}, font, `<w:jc w:val="center"/><w:spacing w:before="240" w:after="240"/>`))
	case Blockquote:
		for _, c := range v.Children {
			if p, ok := c.(Paragraph); ok {
				b.WriteString(docxPara(italicRuns(p.Runs), font, `<w:ind w:left="720"/>`))
			} else {
				writeBlockDOCX(b, c, font, bodyIndent)
			}
		}
	case List:
		for i, it := range v.Items {
			prefix := "• "
			if v.Ordered {
				prefix = fmt.Sprintf("%d. ", v.Start+i)
			}
			runs := append([]Run{{Text: prefix}}, it.Runs...)
			b.WriteString(docxPara(runs, font, `<w:ind w:left="360"/>`))
		}
	case Endnotes:
		if len(v.Items) == 0 {
			return
		}
		b.WriteString(docxPara([]Run{{Text: "Notes", Bold: true}}, font, `<w:spacing w:before="480" w:after="120"/>`))
		for _, e := range v.Items {
			runs := append([]Run{{Text: fmt.Sprintf("%d. ", e.Num)}}, e.Runs...)
			b.WriteString(docxPara(runs, font, ""))
		}
	}
}

// boldRuns / italicRuns force a style on a run slice (copy — don't mutate the AST).
func boldRuns(rs []Run) []Run {
	out := make([]Run, len(rs))
	for i, r := range rs {
		r.Bold = true
		out[i] = r
	}
	return out
}
func italicRuns(rs []Run) []Run {
	out := make([]Run, len(rs))
	for i, r := range rs {
		r.Italic = true
		out[i] = r
	}
	return out
}
```
(Before finalizing, read `export_rtf.go` `writeBlockRTF` and confirm the block set + degrade choices match — adjust `writeBlockDOCX` to mirror it. If `Heading`/`List`/`Blockquote`/`Endnotes` field names differ from this plan, use the real ones from `export_ast.go`.)

- [ ] **Step 4: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w export_docx.go export_docx_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: `TestWriteDOCXStructureAndContent` PASS.

- [ ] **Step 5: Commit**

```
git add export_docx.go export_docx_test.go
git commit -m "feat(export): DOCX (OOXML) writer from the export AST"
```

---

## Task 2: Emit `.docx` from `runExport`

**Files:** Modify `export.go` (`runExport`). Test: `export_wiring_test.go` (append).

- [ ] **Step 1: Write the failing test**

Read `export_wiring_test.go` for the existing pattern (it drives `runExport` and checks the `export/` dir). Append a test that after an export, `<slug>.docx` exists and is a valid zip containing `word/document.xml`. Reuse `hasZipEntry` from `export_docx_test.go`.

- [ ] **Step 2: Wire it in**

In `runExport` (`export.go`), where it writes `<slug>.rtf` and `<slug>.pdf`, add a `<slug>.docx`:
```go
	docxPath := filepath.Join(outDir, slug+".docx")
	docxBytes, err := writeDOCX(doc, st, meta)
	if err != nil {
		m.status = "export failed (docx): " + err.Error()
		return
	}
	if err := atomicWrite(docxPath, docxBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
```
Update the success status to `"exported " + slug + ".rtf + .pdf + .docx to export/"`.

- [ ] **Step 3: Run + build + vet + commit**

```
/opt/homebrew/bin/gofmt -w export.go export_wiring_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
git add export.go export_wiring_test.go
git commit -m "feat(export): emit .docx alongside .rtf + .pdf"
```

---

## Task 3: README — DOCX is exported

**Files:** Modify `README.md`.

- [ ] **Step 1: Update the Export section**

Note that `ctrl+e` now produces RTF + PDF + **DOCX** in Manuscript or Tufte style, and soften any claim that RTF is "the standard submission format" (DOCX is). Build + commit.

```
git add README.md
git commit -m "docs: note DOCX export"
```

---

## Self-review notes
- **Mirror RTF:** `writeBlockDOCX` must match `writeBlockRTF`'s block set + degrade choices — the implementer reads `export_rtf.go` and reconciles.
- **Validity bar:** the three package parts + well-formed `word/document.xml` = Word opens it. `word/styles.xml` is intentionally omitted (formatting is inlined per run/para) — a lean v1.
- **Pure-Go:** stdlib `archive/zip` only; no new deps, no cgo.
- **AST reuse:** DOCX consumes the same `ManuscriptDoc` as RTF/PDF; the title-page work (Stage 5) will extend all three writers together.
