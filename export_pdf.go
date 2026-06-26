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
		registerETBook(pdf)
		cfg = pdfStyle{font: "etbook", bodySize: 12, titleSize: 16, lineHeight: 17, indent: ""}
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
			h := pdf.HTMLBasicNew()
			h.Write(cfg.lineHeight, pdfEnc(st, runsHTML(v.Runs)))
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
