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
