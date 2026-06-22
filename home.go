package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type homeKind int

const (
	homeRecentFile homeKind = iota
	homeProject
	homeOpenOther
)

// homeItem is one selectable row on the launch screen.
type homeItem struct {
	kind  homeKind
	label string // basename / project name / action label
	path  string // file path, project dir, or "" for the action
}

// buildHomeItems composes the launch list: recent files (most-recent-first),
// then project folders (immediate non-hidden subdirs of projectsDir, alpha),
// then a final "Open another folder…" action.
func buildHomeItems(recents []string, projectsDir string) []homeItem {
	var items []homeItem
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}

	if entries, err := os.ReadDir(projectsDir); err == nil {
		var dirs []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
		for _, d := range dirs {
			items = append(items, homeItem{kind: homeProject, label: d, path: filepath.Join(projectsDir, d)})
		}
	}

	items = append(items, homeItem{kind: homeOpenOther, label: "Browse all files"})
	return items
}

// updateHome handles input on the launch screen.
func (m model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.homeSelected > 0 {
				m.homeSelected--
			}
		case "down", "j":
			if m.homeSelected < len(m.homeItems)-1 {
				m.homeSelected++
			}
		case "enter":
			m.openHomeSelection()
		}
	}
	return m, nil
}

// homeView renders the centered logo and the launch list with group headers.
func (m model) homeView() string {
	header := lipgloss.NewStyle().Foreground(subtle).Bold(true)
	var b strings.Builder
	printedRecent, printedProjects := false, false
	for i, it := range m.homeItems {
		switch it.kind {
		case homeRecentFile:
			if !printedRecent {
				b.WriteString(header.Render("RECENT") + "\n")
				printedRecent = true
			}
		case homeProject:
			if !printedProjects {
				b.WriteString("\n" + header.Render("PROJECTS") + "\n")
				printedProjects = true
			}
		case homeOpenOther:
			b.WriteString("\n")
		}

		var icon string
		switch it.kind {
		case homeProject, homeOpenOther:
			icon = m.icons.folder
		default:
			icon = m.icons.icon(fileEntry{name: it.label})
		}
		line := "  " + icon + it.label
		if i == m.homeSelected {
			line = selectedStyle.Render(" " + icon + it.label + " ")
		}
		b.WriteString(line + "\n")
	}

	logo := bannerView(m.width)
	content := lipgloss.JoinVertical(lipgloss.Center, logo, "", b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// openHomeSelection acts on the highlighted launch item and enters writing mode.
func (m *model) openHomeSelection() {
	if len(m.homeItems) == 0 {
		return
	}
	it := m.homeItems[m.homeSelected]
	switch it.kind {
	case homeRecentFile:
		m.files.SetDir(filepath.Dir(it.path))
		m.loadFile(it.path)
		m.focus = focusEditor
		m.editor.Focus()
	case homeProject:
		m.files.SetDir(it.path)
		m.focus = focusSidebar
		m.editor.Blur()
	case homeOpenOther:
		m.files.SetDir(writingDir())
		m.focus = focusSidebar
		m.editor.Blur()
	}
	m.screen = screenWriting
	m.layout()
}
