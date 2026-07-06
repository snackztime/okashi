package main

import "strings"

// outlineBlock is the [start,end) line range of a beat and its notes.
type outlineBlock struct{ start, end int }

func isBulletMarker(b byte) bool { return b == '-' || b == '*' || b == '+' }

// isTopBeat reports whether line is a top-level list item (marker + space, at indent 0).
func isTopBeat(line string) bool {
	return len(line) >= 2 && isBulletMarker(line[0]) && line[1] == ' '
}

// beatBlocks returns each beat's block range in order; lines before the first beat (preamble) belong
// to no block.
func beatBlocks(lines []string) []outlineBlock {
	var blocks []outlineBlock
	start := -1
	for i, ln := range lines {
		if isTopBeat(ln) {
			if start >= 0 {
				blocks = append(blocks, outlineBlock{start, i})
			}
			start = i
		}
	}
	if start >= 0 {
		blocks = append(blocks, outlineBlock{start, len(lines)})
	}
	return blocks
}

// blockAt returns the beat block containing line, or ok=false when line is in the preamble.
func blockAt(lines []string, line int) (outlineBlock, bool) {
	for _, b := range beatBlocks(lines) {
		if line >= b.start && line < b.end {
			return b, true
		}
	}
	return outlineBlock{}, false
}

// stripMarker drops a leading bullet marker + space from a trimmed string.
func stripMarker(s string) string {
	if len(s) >= 2 && isBulletMarker(s[0]) && s[1] == ' ' {
		return strings.TrimSpace(s[2:])
	}
	return s
}

// beatTitle strips a beat line's marker + an optional [ ]/[x] task box + surrounding spaces.
func beatTitle(line string) string {
	s := stripMarker(strings.TrimSpace(line))
	switch {
	case strings.HasPrefix(s, "[ ] "), strings.HasPrefix(s, "[x] "), strings.HasPrefix(s, "[X] "):
		return strings.TrimSpace(s[4:])
	case s == "[ ]" || s == "[x]" || s == "[X]":
		return ""
	}
	return s
}

// beatIsPromoted reports whether a beat line carries a checked task box.
func beatIsPromoted(line string) bool {
	s := stripMarker(strings.TrimSpace(line))
	return strings.HasPrefix(s, "[x]") || strings.HasPrefix(s, "[X]")
}

// beatNotes returns a block's note lines (after the beat line), each trimmed of indent + marker,
// blanks dropped.
func beatNotes(lines []string, b outlineBlock) []string {
	var notes []string
	for i := b.start + 1; i < b.end; i++ {
		s := stripMarker(strings.TrimSpace(lines[i]))
		if s != "" {
			notes = append(notes, s)
		}
	}
	return notes
}
