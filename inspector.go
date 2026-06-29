package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type inspectorTab int

const tabWords inspectorTab = 0 // more tabs (Goals, Analysis, Outline) added later

// inspectorModel is the read-only right-side panel: a tab bar + the active tab.
type inspectorModel struct {
	visible bool
	tab     inspectorTab
}

type docStats struct{ words, chars, paragraphs int }

type projStats struct {
	words, chapters int
	manuscript      bool
}

var blankLineRe = regexp.MustCompile(`\n[ \t]*\n`)

// computeDocStats derives word/char/paragraph counts from the open buffer.
func computeDocStats(text string) docStats {
	if strings.TrimSpace(text) == "" {
		return docStats{}
	}
	ds := docStats{
		words: wordCount(text),
		chars: utf8.RuneCountInString(text),
	}
	for _, block := range blankLineRe.Split(text, -1) {
		if strings.TrimSpace(block) != "" {
			ds.paragraphs++
		}
	}
	return ds
}

// computeProjStats sums the resolved manuscript's chapter word counts (or, for a
// plain folder, its loose docs) using the existing word-count cache.
func computeProjStats(dir string, v manuscriptView, wc *wordCountCache) projStats {
	if wc == nil {
		return projStats{}
	}
	if len(v.chapters) > 0 {
		ps := projStats{manuscript: true, chapters: len(v.chapters)}
		for _, ch := range v.chapters {
			ps.words += wc.count(filepath.Join(dir, ch.file))
		}
		return ps
	}
	ps := projStats{}
	for _, e := range v.loose {
		ps.words += wc.count(filepath.Join(dir, e.name))
	}
	return ps
}

// kvRow renders "  label" left, a subtle right-aligned number, fit to width.
func kvRow(label string, n, width int) string {
	lbl := "  " + label
	val := commafy(n)
	gap := width - lipgloss.Width(lbl) - lipgloss.Width(val)
	if gap < 1 {
		gap = 1
	}
	return lbl + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(subtle).Render(val)
}

// View renders the tab bar + the active tab's body, fit to the given inner width.
func (in inspectorModel) View(width int, doc docStats, proj projStats) string {
	tabs := []string{"Words"} // future: Goals, Analysis, Outline
	var bar strings.Builder
	for i, t := range tabs {
		chip := " " + t + " "
		if inspectorTab(i) == in.tab {
			bar.WriteString(selectedStyle.Render(chip))
		} else {
			bar.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(chip))
		}
	}

	var b strings.Builder
	b.WriteString(bar.String())
	b.WriteString("\n\n")
	b.WriteString(breadcrumbStyle.Render("Document") + "\n")
	b.WriteString(kvRow("Words", doc.words, width) + "\n")
	b.WriteString(kvRow("Characters", doc.chars, width) + "\n")
	b.WriteString(kvRow("Paragraphs", doc.paragraphs, width) + "\n\n")
	b.WriteString(breadcrumbStyle.Render("Project") + "\n")
	b.WriteString(kvRow("Words", proj.words, width))
	if proj.manuscript {
		b.WriteString("\n" + kvRow("Chapters", proj.chapters, width))
	}
	return b.String()
}
