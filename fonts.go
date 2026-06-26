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
