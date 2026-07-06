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
// Note: strip the marker BEFORE trimming — TrimSpace("- ") == "-", which would hide an empty beat.
func beatTitle(line string) string {
	s := stripMarker(line) // beat lines are at indent 0; stripMarker trims the remainder
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
	s := stripMarker(line)
	return strings.HasPrefix(s, "[x]") || strings.HasPrefix(s, "[X]")
}

// moveBeat swaps the beat block containing cursorLine with its neighbor (dir -1 up / +1 down),
// keeping the cursor on the same line within the moved block. ok=false (no change) when the cursor is
// in the preamble or there is no neighbor that way. Adjacent beat blocks are contiguous.
func moveBeat(lines []string, cursorLine, dir int) ([]string, int, bool) {
	blocks := beatBlocks(lines)
	idx := -1
	for i, b := range blocks {
		if cursorLine >= b.start && cursorLine < b.end {
			idx = i
			break
		}
	}
	if idx < 0 {
		return lines, cursorLine, false
	}
	j := idx + dir
	if j < 0 || j >= len(blocks) {
		return lines, cursorLine, false
	}
	lo, hi := idx, j
	if lo > hi {
		lo, hi = hi, lo
	}
	A, B := blocks[lo], blocks[hi] // A.end == B.start (contiguous)
	out := make([]string, 0, len(lines))
	out = append(out, lines[:A.start]...)
	out = append(out, lines[B.start:B.end]...) // B moves before A
	out = append(out, lines[A.start:A.end]...)
	out = append(out, lines[B.end:]...)
	off := cursorLine - blocks[idx].start
	newStart := A.start // moving B up → B now starts where A did
	if idx == lo {      // moving A down → A now sits after B
		newStart = A.start + (B.end - B.start)
	}
	return out, newStart + off, true
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
