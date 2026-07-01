package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// structAdd is one row in the add pick: the "new blank chapter" sentinel (file == "") or an
// existing loose Resource file.
type structAdd struct {
	file  string // "" = new blank chapter
	label string
}

// structureAddChoices is [new blank chapter] followed by the manuscript's loose Resources (on-disk
// .md not currently listed in the buffer nor pending-new), de-slug-titled.
func (m model) structureAddChoices() []structAdd {
	out := []structAdd{{file: "", label: "＋ new blank chapter"}}
	listed := map[string]bool{}
	for _, it := range m.structureItems {
		listed[it.File] = true
	}
	for _, e := range readEntries(m.structureDir) { // non-hidden document files
		if listed[e.name] {
			continue
		}
		out = append(out, structAdd{file: e.name, label: "◦ " + sectionTitle(e.name)})
	}
	return out
}

// uniqueChapterFile returns a filename not present on disk in dir and not already taken by the
// buffer/pending set — "untitled.md", "untitled-2.md", …
func uniqueChapterFile(dir string, taken map[string]bool) string {
	for n := 1; ; n++ {
		name := "untitled.md"
		if n > 1 {
			name = "untitled-" + strconv.Itoa(n) + ".md"
		}
		if taken[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// applyAdd inserts the chosen add option after the cursor.
func (m *model) applyAdd(c structAdd) {
	at := m.structureSel + 1
	if at > len(m.structureItems) {
		at = len(m.structureItems)
	}
	var it manifestItem
	if c.file == "" { // new blank chapter
		taken := map[string]bool{}
		for _, x := range m.structureItems {
			taken[x.File] = true
		}
		for f := range m.structurePendingNew {
			taken[f] = true
		}
		f := uniqueChapterFile(m.structureDir, taken)
		m.structurePendingNew[f] = true
		it = manifestItem{File: f, Title: "Untitled"}
	} else { // promote an existing loose Resource
		it = manifestItem{File: c.file, Title: sectionTitle(c.file)}
	}
	m.structureItems = append(m.structureItems[:at], append([]manifestItem{it}, m.structureItems[at:]...)...)
	m.structureSel = at
	m.structureDirty = true
}

// enterStructure opens structure mode for the binder's current manuscript. Manifest manuscripts
// only — a legacy/absent/unreadable manifest keeps the binder with a status.
func (m *model) enterStructure() {
	dir := m.outline.dir
	sm, present, err := readManifest(dir)
	if !present || err != nil {
		m.status = "not reorderable — no manifest"
		return
	}
	m.structureDir = dir
	m.structureItems = append([]manifestItem{}, sm.Items...)
	m.structureSel = 0
	m.structurePendingNew = map[string]bool{}
	m.structureDirty = false
	m.structureAdding = false
	m.structureRenaming = false
	m.structureConfirm = false
	m.screen = screenStructure
}

// structureTitle is the manuscript's display title (from the on-disk manifest, falling back to the
// folder name).
func (m model) structureTitle() string {
	if sm, present, err := readManifest(m.structureDir); present && err == nil && sm.Title != "" {
		return sm.Title
	}
	return projectTitle(filepath.Base(m.structureDir))
}

// exitStructure returns to the binder (screenOutline), reloading it.
func (m *model) exitStructure() {
	m.outline.load(m.structureDir, m.files.wc)
	m.screen = screenOutline
}

func (m model) updateStructure(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.structureRenaming {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.structureRenaming = false
				m.nameInput.Blur()
				return m, nil
			case "enter":
				m.structureRenaming = false
				m.nameInput.Blur()
				if t := strings.TrimSpace(m.nameInput.Value()); t != "" && m.structureSel < len(m.structureItems) {
					m.structureItems[m.structureSel].Title = t
					m.structureDirty = true
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
		}
		if m.structureAdding {
			choices := m.structureAddChoices()
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.structureAdding = false
				return m, nil
			case "up", "k":
				if m.structureAddSel > 0 {
					m.structureAddSel--
				}
			case "down", "j":
				if m.structureAddSel < len(choices)-1 {
					m.structureAddSel++
				}
			case "enter":
				if m.structureAddSel < len(choices) {
					m.applyAdd(choices[m.structureAddSel])
				}
				m.structureAdding = false
			}
			return m, nil
		}
		if m.structureConfirm {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				if err := m.commitStructure(); err != nil {
					m.status = "commit failed: " + err.Error()
					m.structureConfirm = false
					return m, nil
				}
				m.structureConfirm = false
				m.exitStructure()
				m.status = "structure saved"
				return m, nil
			case "esc", "n":
				m.structureConfirm = false
				m.exitStructure()
				m.status = "structure changes discarded"
				return m, nil
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.structureSel > 0 {
				m.structureSel--
			}
		case "down", "j":
			if m.structureSel < len(m.structureItems)-1 {
				m.structureSel++
			}
		case "J", "shift+down":
			if m.structureSel < len(m.structureItems)-1 {
				i := m.structureSel
				m.structureItems[i], m.structureItems[i+1] = m.structureItems[i+1], m.structureItems[i]
				m.structureSel++
				m.structureDirty = true
			}
		case "K", "shift+up":
			if m.structureSel > 0 {
				i := m.structureSel
				m.structureItems[i], m.structureItems[i-1] = m.structureItems[i-1], m.structureItems[i]
				m.structureSel--
				m.structureDirty = true
			}
		case "x":
			if m.structureSel < len(m.structureItems) {
				f := m.structureItems[m.structureSel].File
				delete(m.structurePendingNew, f) // if it was a not-yet-created new chapter, forget it
				m.structureItems = append(m.structureItems[:m.structureSel], m.structureItems[m.structureSel+1:]...)
				if m.structureSel >= len(m.structureItems) && m.structureSel > 0 {
					m.structureSel--
				}
				m.structureDirty = true
			}
		case "a":
			m.structureAdding = true
			m.structureAddSel = 0
		case "r":
			if m.structureSel < len(m.structureItems) {
				m.structureRenaming = true
				m.nameInput.SetValue(m.structureItems[m.structureSel].Title)
				m.nameInput.CursorEnd()
				m.nameInput.Focus()
				return m, textinput.Blink
			}
		case "esc":
			if m.structureDirty {
				m.structureConfirm = true
			} else {
				m.exitStructure()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) structureView() string {
	var b strings.Builder
	// Window the chapter list so View() stays O(visible) and the selected row is always shown
	// (CLAUDE.md: View MUST stay O(visible) — a 400-page work has 40–100 chapters).
	visRows := m.height - 8
	if visRows < 1 {
		visRows = 1
	}
	off := homeWindowOffset(len(m.structureItems), m.structureSel, visRows)
	rows := make([]string, 0, visRows)
	for i := off; i < len(m.structureItems) && len(rows) < visRows; i++ {
		num := lipgloss.NewStyle().Foreground(subtle).Render(fmtNum(i + 1))
		label := m.structureItems[i].Title
		if m.structureSel == i {
			label = selectedStyle.Render(label)
		}
		rows = append(rows, num+"  "+label)
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("(no chapters)"))
	}
	body := framedPanel(m.structureTitle()+" — structure", strings.Join(rows, "\n"),
		max(40, min(m.width-8, 72)), len(rows)+2, "")
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	if m.structureAdding {
		var picks []string
		for i, c := range m.structureAddChoices() {
			label := c.label
			if i == m.structureAddSel {
				label = selectedStyle.Render(label)
			}
			picks = append(picks, label)
		}
		pick := framedPanel("add", strings.Join(picks, "\n"), max(30, min(m.width-8, 40)), len(picks)+2, "")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pick))
		return b.String()
	}
	if m.structureConfirm {
		msg := "Apply changes to " + m.structureTitle() + "?  y apply · esc cancel"
		bar := lipgloss.NewStyle().Foreground(accent).Render(msg)
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	if m.structureRenaming {
		field := "retitle ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render(
		"J/K move · a add · x remove · r retitle · esc commit")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}

// commitStructure writes the staged order/membership: it creates any pending new-blank files, then
// persists the whole buffer via the atomic writeManifest (re-reading the on-disk title first).
func (m *model) commitStructure() error {
	for f := range m.structurePendingNew {
		// only create files that survived to the final buffer
		inBuf := false
		for _, it := range m.structureItems {
			if it.File == f {
				inBuf = true
				break
			}
		}
		if !inBuf {
			continue
		}
		if err := atomicWrite(filepath.Join(m.structureDir, f), []byte(""), 0o644); err != nil {
			return err
		}
	}
	title := m.structureTitle() // re-reads the on-disk manifest title (read-modify-write)
	return writeManifest(m.structureDir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         m.structureItems,
	})
}

// fmtNum formats n as a zero-padded 2-digit string.
func fmtNum(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
