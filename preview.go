package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

var (
	fnDef = regexp.MustCompile(`(?m)^\[\^([^\]]+)\]:[ \t]*(.*)$`) // [^id]: text
	fnRef = regexp.MustCompile(`\[\^([^\]]+)\]`)                  // [^id]
	// Fenced code blocks + inline code spans — masked so footnote-like text inside them
	// ("arr[^1]") is never converted.
	codeMask = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~|`[^`\n]*`")
)

// maskCode replaces fenced/inline code with sentinels so the footnote regexes never see
// inside it, and returns a restore func. Shared by footnotesToEndnotes and footnotesToSidenotes.
func maskCode(orig string) (masked string, restore func(string) string) {
	var code []string
	masked = codeMask.ReplaceAllStringFunc(orig, func(c string) string {
		code = append(code, c)
		return fmt.Sprintf("\x00CODE%d\x00", len(code)-1)
	})
	restore = func(s string) string {
		for i, c := range code {
			s = strings.Replace(s, fmt.Sprintf("\x00CODE%d\x00", i), c, 1)
		}
		return s
	}
	return masked, restore
}

// footnotesToEndnotes rewrites GFM footnotes (which glamour can't render) into superscript
// markers plus a "Notes" endnote section, so the preview reads sensibly. No-op when the
// document has no footnote definitions. Code blocks/spans are left untouched.
func footnotesToEndnotes(orig string) string {
	md, restore := maskCode(orig)

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

// footnotesToSidenotes splits GFM footnotes out of the body for margin rendering: it rewrites
// referenced [^id] to superscript markers in place (no endnote section) and returns the note
// texts in first-reference order. Empty notes slice when nothing is referenced. Code is masked.
func footnotesToSidenotes(orig string) (body string, notes []string) {
	md, restore := maskCode(orig)
	defMatches := fnDef.FindAllStringSubmatch(md, -1)
	if len(defMatches) == 0 {
		return restore(md), nil
	}
	defs := map[string]string{}
	for _, m := range defMatches {
		defs[m[1]] = strings.TrimSpace(m[2])
	}
	b := fnDef.ReplaceAllString(md, "") // drop definition lines
	var order []string
	num := map[string]int{}
	b = fnRef.ReplaceAllStringFunc(b, func(ref string) string {
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
	b = strings.TrimRight(b, "\n")
	for _, id := range order {
		notes = append(notes, defs[id])
	}
	return restore(b), notes
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

// tuftePalette holds the warm, book-like colors for the Tufte preview, chosen per background so
// the body reads at full legibility on either theme. `note` is a step subdued from `ink` so
// sidenotes recede as secondary matter; on dark that means dimmer, on light greyer.
type tuftePalette struct{ ink, accent, note, muted string }

func tufteColors(dark bool) tuftePalette {
	if dark {
		return tuftePalette{ink: "#e6dcc8", accent: "#dbb06a", note: "#ab9974", muted: "#6b6153"}
	}
	return tuftePalette{ink: "#3b3228", accent: "#704214", note: "#8a7d6a", muted: "#9b8f80"}
}

// tufteGlamourStyle is a warm, book-like glamour theme: legible ink for all prose, a warm accent
// for (markerless) headings and links, muted rules. It derives from the light or dark base style
// so the body text stays legible on whichever terminal background is in use.
func tufteGlamourStyle(dark bool) ansi.StyleConfig {
	s := styles.LightStyleConfig // value copy
	if dark {
		s = styles.DarkStyleConfig
	}
	p := tufteColors(dark)

	s.Document.Color = sp(p.ink)
	s.Text.Color = sp(p.ink)
	s.Paragraph.Color = sp(p.ink)
	s.Item.Color = sp(p.ink)
	s.Emph.Color = sp(p.ink)
	s.Emph.Italic = bp(true)
	s.Strong.Color = sp(p.ink)
	s.Strong.Bold = bp(true)
	s.BlockQuote.Color = sp(p.ink) // legible body colour; the italic sets it apart
	s.BlockQuote.Italic = bp(true)

	s.Heading.Color = sp(p.accent)
	s.Heading.Bold = bp(true)
	s.H1.BackgroundColor = nil // drop the heavy colored block
	s.H1.Color = sp(p.accent)
	s.H1.Bold = bp(true)
	s.H1.Prefix = ""
	for _, h := range []*ansi.StyleBlock{&s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = "" // markerless headings (the book look)
		h.Color = sp(p.accent)
		h.Bold = bp(true)
	}
	s.HorizontalRule.Color = sp(p.muted)
	s.Link.Color = sp(p.accent)
	return s
}

// padTo pads s with spaces to visible width w (ANSI-aware). Longer strings are returned as-is.
func padTo(s string, w int) string {
	n := w - xansi.StringWidth(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}

// superscriptRuns returns the distinct maximal superscript runs on a line (as strings).
func superscriptRuns(line string) []string {
	isSup := func(r rune) bool {
		switch r {
		case '⁰', '¹', '²', '³', '⁴', '⁵', '⁶', '⁷', '⁸', '⁹':
			return true
		}
		return false
	}
	var runs []string
	var cur strings.Builder
	for _, r := range line {
		if isSup(r) {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() > 0 {
			runs = append(runs, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		runs = append(runs, cur.String())
	}
	return runs
}

// wrapPlain wraps s to width w on spaces (plain text — note bodies carry no ANSI).
func wrapPlain(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > w {
				out = append(out, line)
				line = word
			} else {
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
}

// layoutSidenotes composes body (glamour output, wrapped to `measure`) with `notes` floated into
// a right gutter of `gutter` columns, each anchored to the first row bearing its superscript
// marker and cascading downward so notes never overlap.
func layoutSidenotes(body string, notes []string, measure, gutter int, dark bool) string {
	p := tufteColors(dark)
	divider := lipgloss.NewStyle().Foreground(lipgloss.Color(p.muted)).Render("┆")
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.note))
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	// gutterRows[i] is the note text (already numbered/wrapped) for output row i.
	gutterRows := map[int]string{}
	nextFree := 0
	anchorOf := func(marker string) int {
		for i, ln := range lines {
			for _, run := range superscriptRuns(ln) {
				if run == marker {
					return i
				}
			}
		}
		return -1
	}
	for n, text := range notes {
		marker := superscript(n + 1)
		anchor := anchorOf(marker)
		if anchor < 0 {
			anchor = nextFree
		}
		start := anchor
		if start < nextFree {
			start = nextFree
		}
		wrapped := wrapPlain(marker+" "+text, gutter)
		for j, wl := range wrapped {
			gutterRows[start+j] = wl
		}
		nextFree = start + len(wrapped) + 1 // blank row between notes
	}
	// Determine how many rows we render (body rows or further, if a note overflows).
	maxRow := len(lines) - 1
	for r := range gutterRows {
		if r > maxRow {
			maxRow = r
		}
	}
	var b strings.Builder
	for i := 0; i <= maxRow; i++ {
		bodyLine := ""
		if i < len(lines) {
			bodyLine = lines[i]
		}
		b.WriteString(padTo(bodyLine, measure))
		b.WriteString(" ")
		b.WriteString(divider)
		b.WriteString(" ")
		if g, ok := gutterRows[i]; ok {
			b.WriteString(noteStyle.Render(g))
		}
		if i < maxRow {
			b.WriteString("\n")
		}
	}
	return b.String()
}
