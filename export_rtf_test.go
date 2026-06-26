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
