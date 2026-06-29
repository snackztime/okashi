package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type inspectorTab int

const (
	tabWords inspectorTab = iota
	tabOutline
	tabGoals
)

// inspectorTabLabels is the single source of the tab set — used by both the tab
// bar render and cycle() so they never diverge.
func inspectorTabLabels() []string { return []string{"Words", "Outline", "Goals"} }

// inspectorModel is the read-only right-side panel: a tab bar + the active tab.
type inspectorModel struct {
	visible bool
	tab     inspectorTab
}

// cycle advances the inspector: hidden → Words → Outline → … → hidden.
func (in *inspectorModel) cycle() {
	if !in.visible {
		in.visible = true
		in.tab = tabWords
		return
	}
	in.tab++
	if int(in.tab) >= len(inspectorTabLabels()) {
		in.visible = false
		in.tab = tabWords
	}
}

// renderOutline shows the outline read-only: top-level bullets in accent, nested
// lines plain, each truncated to width. Empty → a hint.
func renderOutline(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty — ctrl+l to edit)")
	}
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		shown := ansi.Truncate(line, width, "…")
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(shown))
		} else {
			b.WriteString(shown)
		}
	}
	return b.String()
}

type docStats struct{ words, chars, paragraphs int }

type projStats struct {
	words, chapters int
	manuscript      bool
}

type goalStats struct{ today, dailyGoal, project, projectGoal int }

// progressBar renders a width-cell bar: filled proportion in accent █, rest ░.
func progressBar(cur, goal, width int) string {
	filled := 0
	if goal > 0 {
		filled = (cur * width) / goal
		if filled > width {
			filled = width
		}
		if filled < 0 {
			filled = 0
		}
	}
	bar := lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("░", width-filled))
	return "[" + bar + "]"
}

// inspectorTabAtX maps an x offset within the tab bar to a tab (chips are
// " label " = len+2 wide). Mirrors the View tab-bar render.
func inspectorTabAtX(localX int) (inspectorTab, bool) {
	x := 0
	for i, t := range inspectorTabLabels() {
		w := len(t) + 2
		if localX >= x && localX < x+w {
			return inspectorTab(i), true
		}
		x += w
	}
	return tabWords, false
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
func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string, goals goalStats) string {
	var bar strings.Builder
	for i, t := range inspectorTabLabels() {
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
	switch in.tab {
	case tabOutline:
		b.WriteString(breadcrumbStyle.Render("Outline") + "\n\n")
		b.WriteString(renderOutline(outline, width))
	case tabGoals:
		b.WriteString(breadcrumbStyle.Render("Daily") + "\n")
		b.WriteString(progressBar(goals.today, goals.dailyGoal, max(4, width-8)) + "\n")
		b.WriteString(fmt.Sprintf("%s / %s\n", commafy(goals.today), commafy(goals.dailyGoal)))
		if goals.today >= goals.dailyGoal && goals.dailyGoal > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render("✓ goal met"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(commafy(goals.dailyGoal-goals.today) + " to go"))
		}
		if goals.projectGoal > 0 {
			b.WriteString("\n\n" + breadcrumbStyle.Render("Project") + "\n")
			b.WriteString(progressBar(goals.project, goals.projectGoal, max(4, width-8)) + "\n")
			b.WriteString(fmt.Sprintf("%s / %s", commafy(goals.project), commafy(goals.projectGoal)))
		}
	default: // tabWords
		b.WriteString(breadcrumbStyle.Render("Document") + "\n")
		b.WriteString(kvRow("Words", doc.words, width) + "\n")
		b.WriteString(kvRow("Characters", doc.chars, width) + "\n")
		b.WriteString(kvRow("Paragraphs", doc.paragraphs, width) + "\n\n")
		b.WriteString(breadcrumbStyle.Render("Project") + "\n")
		b.WriteString(kvRow("Words", proj.words, width))
		if proj.manuscript {
			b.WriteString("\n" + kvRow("Chapters", proj.chapters, width))
		}
	}
	return b.String()
}
