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
		syn = firstProseLine(filepath.Join(f.dir, ch.file))
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
