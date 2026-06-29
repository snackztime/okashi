package main

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

var (
	synHeadingStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	synMarkerStyle  = lipgloss.NewStyle().Foreground(subtle)
	synBoldStyle    = lipgloss.NewStyle().Bold(true)
	synItalicStyle  = lipgloss.NewStyle().Italic(true)
	synCodeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	synLinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd"))
)

var (
	synHeadingRe = regexp.MustCompile(`^(#{1,6}\s+)(\S.*)$`)
	synBoldRe    = regexp.MustCompile(`\*\*[^*]+\*\*|__[^_]+__`)
	synItalicRe  = regexp.MustCompile(`\*[^*]+\*|_[^_]+_`)
	synCodeRe    = regexp.MustCompile("`[^`]+`")
	synLinkRe    = regexp.MustCompile(`(\[[^\]]+\])(\([^)]+\))`)
)

// syntaxDecorator styles markdown tokens on one line. Rune-range decorations,
// built in priority order; a rune already covered is not styled again.
func syntaxDecorator(line string) []textarea.Decoration {
	runes := []rune(line)
	occupied := make([]bool, len(runes))
	var decos []textarea.Decoration

	// byteToRune maps a byte offset in `line` to its rune index.
	byteToRune := func(b int) int {
		return len([]rune(line[:b]))
	}
	add := func(rs, re int, style lipgloss.Style) {
		if rs < 0 || re > len(runes) || rs >= re {
			return
		}
		for i := rs; i < re; i++ {
			if occupied[i] {
				return
			}
		}
		for i := rs; i < re; i++ {
			occupied[i] = true
		}
		decos = append(decos, textarea.Decoration{Start: rs, End: re, Style: style})
	}
	addByte := func(bs, be int, style lipgloss.Style) { add(byteToRune(bs), byteToRune(be), style) }

	// Heading: markers (group 1 → m[2]:m[3]) subtle, text (group 2 → m[4]:m[5]) accent bold.
	if m := synHeadingRe.FindStringSubmatchIndex(line); m != nil {
		addByte(m[4], m[5], synHeadingStyle)
		addByte(m[2], m[3], synMarkerStyle)
	}
	// List marker (group 2 → m[4]:m[5]).
	if m := listItemRe.FindStringSubmatchIndex(line); m != nil {
		addByte(m[4], m[5], synMarkerStyle)
	}
	for _, m := range synCodeRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synCodeStyle)
	}
	// Link: [text] (group 1) in link-cyan, (url) (group 2) subtle.
	for _, m := range synLinkRe.FindAllStringSubmatchIndex(line, -1) {
		addByte(m[2], m[3], synLinkStyle)
		addByte(m[4], m[5], synMarkerStyle)
	}
	for _, m := range synBoldRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synBoldStyle)
	}
	for _, m := range synItalicRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synItalicStyle)
	}
	return decos
}
