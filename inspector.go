package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sectionHeader renders an UPPERCASE accent label followed by a subtle rule to width.
func sectionHeader(label string, width int) string {
	up := strings.ToUpper(label)
	hs := lipgloss.NewStyle().Foreground(accent).Bold(true)
	fill := width - lipgloss.Width(up) - 1
	if fill < 0 {
		fill = 0
	}
	return hs.Render(up) + " " + lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("─", fill))
}

// framedPanel wraps inner (multi-line) in a rounded box of the given total width/height,
// with title injected into the top border. Inner lines are padded/truncated ansi-aware.
// action, when non-empty, is rendered right-aligned in the top border before ╮.
func framedPanel(title, inner string, width, height int, action string) string {
	if width < 6 {
		width = 6
	}
	if height < 2 {
		height = 2
	}
	bs := lipgloss.NewStyle().Foreground(subtle)
	ts := lipgloss.NewStyle().Foreground(accent).Bold(true)
	contentW := width - 4 // │ <space> content <space> │

	rightSeg := ""
	if action != "" {
		rightSeg = " " + action
	}
	titleStr := title
	maxTitle := width - 4 - lipgloss.Width(rightSeg) // leave room for ╭╮, the title spaces, and the action
	if lipgloss.Width(titleStr) > maxTitle {
		titleStr = ansi.Truncate(titleStr, maxTitle, "")
	}
	fill := width - 2 - (lipgloss.Width(titleStr) + 2) - lipgloss.Width(rightSeg) // minus ╭╮, minus the two spaces around the title, minus action
	if fill < 0 {
		fill = 0
	}
	top := bs.Render("╭") + ts.Render(" "+titleStr+" ") + bs.Render(strings.Repeat("─", fill)) + ts.Render(rightSeg) + bs.Render("╮")

	// Pad to contentW; truncate FIRST (ansi-aware) so an over-long line never
	// wraps and breaks the frame.
	cell := lipgloss.NewStyle().Width(contentW)
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, height)
	out = append(out, top)
	for r := 0; r < height-2; r++ {
		c := ""
		if r < len(lines) {
			c = ansi.Truncate(lines[r], contentW, "")
		}
		out = append(out, bs.Render("│")+" "+cell.Render(c)+" "+bs.Render("│"))
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(out, "\n")
}

// inspectorInnerWidth returns the true inner content width of the inspector
// panel — the value main.go must pass to View() and the click handlers must
// use. The panel is framed at exactly inspectorWidth so render == reservation
// == offset; framedPanel uses 2 borders + 2 padding cols = 4, so inner = -4.
func inspectorInnerWidth() int { return inspectorWidth - 4 }

type inspectorTab int

const (
	tabWords inspectorTab = iota
	tabOutline
	tabGoals
	tabAnalysis
)

// inspectorTabLabels is the single source of the tab set — used by both the tab
// bar render and cycle() so they never diverge.
func inspectorTabLabels() []string { return []string{"Words", "Outline", "Goals", "Analysis"} }

// inspectorModel is the read-only right-side panel: a tab bar + the active tab.
type inspectorModel struct {
	visible bool
	tab     inspectorTab

	// grammarBackend is the Name() of the active grammarChecker, or "" if none.
	// grammarChecking is true while an async grammar pass is in flight.
	// grammarAutoRecheck mirrors model.autoRecheck. All set in the model's View().
	grammarBackend     string
	grammarChecking    bool
	grammarAutoRecheck bool
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

type goalStats struct {
	today, dailyGoal, project, projectGoal, sessionSecs, sessionGoalMin, todayActiveSecs int
	idle                                                                                 bool
}

type analysisState struct{ spell, grammar, adverb, adjective, passive bool }

// analysisRowY returns the inspector body row (y from the very top of the
// inspector, row 0 = tab bar) for each Analysis checkbox:
//
//	0 → Spellcheck  (tab-bar(0) + blank(1) + header(2) + blank(3) = 4)
//	1 → Grammar     (5)
//	2 → Adverb      (blank(6) + Syntax-header(7) = 8)
//	3 → Adjective   (9)
//	4 → Passive     (10)
func analysisRowY(i int) int {
	return [5]int{4, 5, 8, 9, 10}[i]
}

func inspectorAnalysisRowAtY(localY int) (int, bool) {
	for i := 0; i < 5; i++ {
		if analysisRowY(i) == localY {
			return i, true
		}
	}
	return 0, false
}

func checkbox(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}

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

// tabBar renders the tab labels as a single row: labels separated by single
// spaces, the active label highlighted via selectedStyle. Total width fits
// within inspectorInnerWidth().
func (in inspectorModel) tabBar() string {
	var b strings.Builder
	for i, t := range inspectorTabLabels() {
		if i > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(" "))
		}
		if inspectorTab(i) == in.tab {
			b.WriteString(selectedStyle.Render(t))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(t))
		}
	}
	return b.String()
}

// inspectorTabAtX maps an x offset within the tab bar to a tab. Labels are
// rendered as: label0 space label1 space label2 space label3 (no chip padding).
func inspectorTabAtX(localX int) (inspectorTab, bool) {
	x := 0
	for i, t := range inspectorTabLabels() {
		w := len(t)
		if localX >= x && localX < x+w {
			return inspectorTab(i), true
		}
		x += w + 1 // +1 for the separator space
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
func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string, goals goalStats, analysis analysisState) string {
	var b strings.Builder
	b.WriteString(in.tabBar())
	b.WriteString("\n\n")
	switch in.tab {
	case tabAnalysis:
		b.WriteString(sectionHeader("Analysis", width) + "\n\n")
		b.WriteString("  " + checkbox(analysis.spell) + "Spellcheck\n")
		b.WriteString("  " + checkbox(analysis.grammar) + grammarStyle.Render("Grammar") + "\n")
		b.WriteString("\n")
		b.WriteString(sectionHeader("Syntax", width) + "\n")
		b.WriteString("  " + checkbox(analysis.adverb) + adverbStyle.Render("Adverb") + "\n")
		b.WriteString("  " + checkbox(analysis.adjective) + adjStyle.Render("Adjective") + "\n")
		b.WriteString("  " + checkbox(analysis.passive) + passiveStyle.Render("Passive/weak"))
		if analysis.grammar && in.grammarBackend != "" {
			// Stable layout so the click rows never shift: action (analysisActionRowY=12),
			// backend name (13, dim — keeps a long name off the action row), Auto-recheck (14).
			action := "▸ Check grammar"
			if in.grammarChecking {
				action = "checking grammar…"
			}
			b.WriteString("\n\n  " + action)
			b.WriteString("\n  " + lipgloss.NewStyle().Foreground(subtle).Render(in.grammarBackend))
			b.WriteString("\n  " + checkbox(in.grammarAutoRecheck) + "Auto-recheck")
		}
	case tabOutline:
		b.WriteString(sectionHeader("Outline", width) + "\n\n")
		outLines := strings.Split(renderOutline(outline, width-2), "\n")
		indented := make([]string, len(outLines))
		for i, l := range outLines {
			indented[i] = "  " + l
		}
		b.WriteString(strings.Join(indented, "\n"))
	case tabGoals:
		b.WriteString(sectionHeader("Daily", width) + "\n")
		b.WriteString("  " + progressBar(goals.today, goals.dailyGoal, max(4, width-10)) + "\n")
		b.WriteString("  " + fmt.Sprintf("%s / %s\n", commafy(goals.today), commafy(goals.dailyGoal)))
		if goals.today >= goals.dailyGoal && goals.dailyGoal > 0 {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ goal met"))
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(commafy(goals.dailyGoal-goals.today)+" to go"))
		}
		if goals.projectGoal > 0 {
			b.WriteString("\n\n" + sectionHeader("Project", width) + "\n")
			b.WriteString("  " + progressBar(goals.project, goals.projectGoal, max(4, width-10)) + "\n")
			b.WriteString("  " + fmt.Sprintf("%s / %s", commafy(goals.project), commafy(goals.projectGoal)))
		}
		b.WriteString("\n\n" + sectionHeader("Time", width) + "\n")
		sess := "  Session   " + fmtDuration(time.Duration(goals.sessionSecs)*time.Second)
		if goals.idle {
			sess += " ⏸"
		}
		b.WriteString(sess + "\n")
		if goals.sessionGoalMin > 0 {
			mins := goals.todayActiveSecs / 60
			b.WriteString("  Today\n")
			b.WriteString("  " + progressBar(mins, goals.sessionGoalMin, max(4, width-10)) + "\n")
			b.WriteString("  " + fmt.Sprintf("%d / %d min\n", mins, goals.sessionGoalMin))
			if mins >= goals.sessionGoalMin {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ time goal met"))
			} else {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(fmt.Sprintf("%d min to go", goals.sessionGoalMin-mins)))
			}
		} else {
			b.WriteString("  Today     " + fmtDuration(time.Duration(goals.todayActiveSecs)*time.Second) + "\n")
		}
	default: // tabWords
		b.WriteString(sectionHeader("Document", width) + "\n")
		b.WriteString("  " + kvRow("Words", doc.words, width-2) + "\n")
		b.WriteString("  " + kvRow("Characters", doc.chars, width-2) + "\n")
		b.WriteString("  " + kvRow("Paragraphs", doc.paragraphs, width-2) + "\n\n")
		b.WriteString(sectionHeader("Project", width) + "\n")
		b.WriteString("  " + kvRow("Words", proj.words, width-2))
		if proj.manuscript {
			b.WriteString("\n  " + kvRow("Chapters", proj.chapters, width-2))
		}
	}
	return b.String()
}
