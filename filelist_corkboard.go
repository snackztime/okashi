package main

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// corkView renders the pane in corkboard mode: chapter cards (title + word count + a 2-line synopsis,
// or a dimmed first-line fallback when none is authored), then a Resources section (subfolder groups
// + loose docs). Windowed so View() stays O(visible). Selection matches f.selected's entry by name.
func (f filelist) corkView() string {
	w := f.width
	selName := ""
	if f.selected >= 0 && f.selected < len(f.entries) {
		selName = f.entries[f.selected].name
	}

	var lines []string
	selLine := 0
	add := func(block []string, selected bool) {
		if selected {
			selLine = len(lines)
		}
		for _, l := range block {
			if selected {
				lines = append(lines, selectedStyle.Width(w).Render(ansi.Truncate(l, w, "…")))
			} else {
				lines = append(lines, ansi.Truncate(l, w, "…"))
			}
		}
	}

	// Chapters as cards, in manifest order.
	for _, ch := range f.view.chapters {
		add(f.chapterCard(ch, w), ch.file == selName)
	}

	// Resources: subfolder groups (dirs, incl. "..") then loose docs.
	resHeaderShown := false
	resHeader := func() {
		if !resHeaderShown {
			lines = append(lines, sectionHeader("Resources", w))
			resHeaderShown = true
		}
	}
	for _, e := range f.entries {
		if !e.isDir {
			continue
		}
		resHeader()
		add([]string{" " + renderIcon(f.icons.iconFor(e), e.name == selName) + e.name}, e.name == selName)
	}
	for _, e := range f.view.loose {
		resHeader()
		add([]string{" " + renderIcon(f.icons.iconFor(e), e.name == selName) + e.name}, e.name == selName)
	}

	if len(lines) == 0 {
		return lipgloss.NewStyle().Foreground(subtle).Render("(no chapters)")
	}

	// Window the flat lines so the selected block stays visible.
	off := homeWindowOffset(len(lines), selLine, f.height)
	end := off + f.height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[off:end], "\n")
}

// cachedFirstProse returns a chapter's first prose line, memoized per SetDir so corkView never
// re-reads every chapter file on each frame (View() must stay O(visible)).
func (f filelist) cachedFirstProse(file string) string {
	if f.proseCache != nil {
		if v, ok := f.proseCache[file]; ok {
			return v
		}
	}
	v := firstProseLine(filepath.Join(f.dir, file))
	if f.proseCache != nil {
		f.proseCache[file] = v
	}
	return v
}

// moveChapter swaps the selected chapter with its neighbor (d = -1 up / +1 down) in the staged
// view and rebuilds the entry list, keeping the selection on the moved chapter. Returns false when
// the selection isn't a movable chapter or the move is out of range (a no-op).
func (f *filelist) moveChapter(d int) bool {
	if f.selected < 0 || f.selected >= len(f.entries) {
		return false
	}
	name := f.entries[f.selected].name
	ci := -1
	for i, ch := range f.view.chapters {
		if ch.file == name {
			ci = i
			break
		}
	}
	if ci < 0 {
		return false // selection is a Resource / dir / "..", not a chapter
	}
	nj := ci + d
	if nj < 0 || nj >= len(f.view.chapters) {
		return false
	}
	f.view.chapters[ci], f.view.chapters[nj] = f.view.chapters[nj], f.view.chapters[ci]
	f.rebuildEntries()
	f.selectByName(name)
	return true
}

// rebuildEntries re-derives the entry list from the (possibly reordered) view: dirs, then chapters
// in view order, then loose.
func (f *filelist) rebuildEntries() {
	next := make([]fileEntry, 0, len(f.entries))
	for _, e := range f.entries {
		if e.isDir {
			next = append(next, e)
		}
	}
	for _, ch := range f.view.chapters {
		next = append(next, fileEntry{name: ch.file})
	}
	next = append(next, f.view.loose...)
	f.entries = next
}

// selectByName moves the selection to the entry with the given name (and scrolls it into view).
func (f *filelist) selectByName(name string) {
	for i, e := range f.entries {
		if e.name == name {
			f.selected = i
			f.scrollIntoView()
			return
		}
	}
}

// chapterCard renders one chapter as a header row (title + right-aligned word count) plus a 2-line
// synopsis (authored, or the dimmed first-line fallback).
func (f filelist) chapterCard(ch chapterRef, w int) []string {
	n := 0
	if f.wc != nil {
		n = f.wc.count(filepath.Join(f.dir, ch.file))
	}
	count := commafy(n) + "w"
	title := ansi.Truncate(" "+ch.title, max(1, w-lipgloss.Width(count)-1), "…")
	gap := w - lipgloss.Width(title) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	header := title + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(subtle).Render(count)

	syn := f.synopses[ch.file]
	dim := false
	if syn == "" {
		syn = f.cachedFirstProse(ch.file) // memoized — avoids a full-file read per frame
		dim = true
	}
	if syn == "" {
		return []string{header, lipgloss.NewStyle().Foreground(subtle).Render("   (no synopsis)")}
	}
	out := []string{header}
	for _, l := range strings.Split(wrapClamp(syn, max(1, w-3), 2), "\n") {
		row := "   " + l
		if dim {
			row = lipgloss.NewStyle().Foreground(subtle).Render(row)
		}
		out = append(out, row)
	}
	return out
}
