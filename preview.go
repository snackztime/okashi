package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

var (
	fnDef = regexp.MustCompile(`(?m)^\[\^([^\]]+)\]:[ \t]*(.*)$`) // [^id]: text
	fnRef = regexp.MustCompile(`\[\^([^\]]+)\]`)                  // [^id]
	// Fenced code blocks + inline code spans — masked so footnote-like text inside them
	// ("arr[^1]") is never converted.
	codeMask = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\n]*`")
)

// footnotesToEndnotes rewrites GFM footnotes (which glamour can't render) into superscript
// markers plus a "Notes" endnote section, so the preview reads sensibly. No-op when the
// document has no footnote definitions. Code blocks/spans are left untouched.
func footnotesToEndnotes(orig string) string {
	// Mask code so the footnote regexes never see inside it; restore at the end.
	var code []string
	md := codeMask.ReplaceAllStringFunc(orig, func(c string) string {
		code = append(code, c)
		return fmt.Sprintf("\x00CODE%d\x00", len(code)-1)
	})
	restore := func(s string) string {
		for i, c := range code {
			s = strings.Replace(s, fmt.Sprintf("\x00CODE%d\x00", i), c, 1)
		}
		return s
	}

	defMatches := fnDef.FindAllStringSubmatch(md, -1)
	if len(defMatches) == 0 {
		return restore(md)
	}
	defs := map[string]string{}
	for _, m := range defMatches {
		defs[m[1]] = strings.TrimSpace(m[2])
	}
	body := fnDef.ReplaceAllString(md, "") // drop definition lines
	var order []string
	num := map[string]int{}
	body = fnRef.ReplaceAllStringFunc(body, func(ref string) string {
		id := fnRef.FindStringSubmatch(ref)[1]
		if _, ok := defs[id]; !ok {
			return ref // orphan reference: keep literal
		}
		if _, seen := num[id]; !seen {
			order = append(order, id)
			num[id] = len(order)
		}
		return superscript(num[id])
	})
	body = strings.TrimRight(body, "\n")
	if len(order) == 0 {
		return restore(body + "\n") // definitions existed but were never referenced → dropped
	}
	var b strings.Builder
	b.WriteString(body)
	b.WriteString("\n\n---\n\n### Notes\n\n")
	for _, id := range order {
		b.WriteString(fmt.Sprintf("%d. %s\n", num[id], defs[id]))
	}
	return restore(b.String())
}

func superscript(n int) string {
	sup := map[rune]rune{'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹'}
	var b strings.Builder
	for _, r := range strconv.Itoa(n) {
		b.WriteRune(sup[r])
	}
	return b.String()
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

// tufteGlamourStyle is a warm, book-like glamour theme derived from the light style:
// sepia ink, restrained (markerless) headings, muted quotes/rules.
func tufteGlamourStyle() ansi.StyleConfig {
	s := styles.LightStyleConfig // value copy
	ink := "#3b3228"
	muted := "#7a6f63"
	sepia := "#704214"

	s.Document.Color = sp(ink)
	s.Text.Color = sp(ink)
	s.Paragraph.Color = sp(ink)

	s.Heading.Color = sp(sepia)
	s.Heading.Bold = bp(true)
	s.H1.BackgroundColor = nil // drop the heavy colored block
	s.H1.Color = sp(sepia)
	s.H1.Bold = bp(true)
	s.H1.Prefix = ""
	for _, h := range []*ansi.StyleBlock{&s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = "" // markerless headings (the book look)
		h.Color = sp(sepia)
		h.Bold = bp(true)
	}
	s.BlockQuote.Color = sp(muted)
	s.BlockQuote.Italic = bp(true)
	s.Emph.Italic = bp(true)
	s.Emph.Color = sp(ink)
	s.Strong.Bold = bp(true)
	s.Strong.Color = sp(ink)
	s.HorizontalRule.Color = sp(muted)
	s.Link.Color = sp(sepia)
	s.Item.Color = sp(ink)
	return s
}
