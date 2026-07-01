package main

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
		case "r":
			if m.structureSel < len(m.structureItems) {
				m.structureRenaming = true
				m.nameInput.SetValue(m.structureItems[m.structureSel].Title)
				m.nameInput.CursorEnd()
				m.nameInput.Focus()
				return m, textinput.Blink
			}
		case "esc":
			m.exitStructure()
			return m, nil
		}
	}
	return m, nil
}

func (m model) structureView() string {
	var b strings.Builder
	rows := make([]string, 0, len(m.structureItems))
	for i, it := range m.structureItems {
		num := lipgloss.NewStyle().Foreground(subtle).Render(fmtNum(i + 1))
		label := it.Title
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
	if m.structureRenaming {
		field := "retitle ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render(
		"J/K move · a add · x remove · r retitle · esc commit")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}

// fmtNum formats n as a zero-padded 2-digit string.
func fmtNum(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
