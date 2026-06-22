package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type homeKind int

const (
	homeRecentFile homeKind = iota
	homeProject
	homeNewDocument
	homeNewProject
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

	items = append(items,
		homeItem{kind: homeNewDocument, label: "New document"},
		homeItem{kind: homeNewProject, label: "New project"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
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
			return m, m.openHomeSelection()
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.homeSelected > 0 {
				m.homeSelected--
			}
		case tea.MouseButtonWheelDown:
			if m.homeSelected < len(m.homeItems)-1 {
				m.homeSelected++
			}
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			idx := homeItemAtY(m.homeItems, m.homeSelected, m.icons, m.height, msg.Y)
			if idx < 0 {
				return m, nil
			}
			m.homeSelected = idx
			now := time.Now()
			if idx == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
				m.lastClickTime = time.Time{}
				return m, m.openHomeSelection()
			}
			m.lastClickRow = idx
			m.lastClickTime = now
		}
		return m, nil
	}
	return m, nil
}

// homeRows builds the launch content: the logo, group headers, and item rows.
// It returns the lines, the content-row of each item (by index), and the total
// height — so the renderer and the mouse hit-test share one layout.
func homeRows(items []homeItem, sel int, icons iconSet) (lines []string, itemRow []int, height int) {
	header := lipgloss.NewStyle().Foreground(subtle).Bold(true)
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, l)
	}
	lines = append(lines, "") // gap under the logo

	itemRow = make([]int, len(items))
	printedRecent, printedProjects := false, false
	for i, it := range items {
		switch it.kind {
		case homeRecentFile:
			if !printedRecent {
				lines = append(lines, header.Render("RECENT"))
				printedRecent = true
			}
		case homeProject:
			if !printedProjects {
				lines = append(lines, "", header.Render("PROJECTS"))
				printedProjects = true
			}
		case homeNewDocument:
			lines = append(lines, "")
		}

		var icon string
		switch it.kind {
		case homeProject, homeOpenOther:
			icon = icons.folder
		case homeNewDocument, homeNewProject:
			icon = icons.action
		default:
			icon = icons.icon(fileEntry{name: it.label})
		}
		row := "  " + icon + it.label
		if i == sel {
			row = selectedStyle.Render(" " + icon + it.label + " ")
		}
		itemRow[i] = len(lines)
		lines = append(lines, row)
	}
	return lines, itemRow, len(lines)
}

func (m model) homeView() string {
	lines, _, _ := homeRows(m.homeItems, m.homeSelected, m.icons)
	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// homeItemAtY maps an absolute screen Y to a launch item index, or -1.
func homeItemAtY(items []homeItem, sel int, icons iconSet, screenH, y int) int {
	_, itemRow, h := homeRows(items, sel, icons)
	off := (screenH - h) / 2
	if off < 0 {
		off = 0
	}
	contentRow := y - off
	for i, r := range itemRow {
		if r == contentRow {
			return i
		}
	}
	return -1
}

// openHomeSelection acts on the highlighted launch item and enters writing mode.
func (m *model) openHomeSelection() tea.Cmd {
	if len(m.homeItems) == 0 {
		return nil
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
	case homeNewDocument:
		m.files.SetDir(writingDir())
		m.screen = screenWriting
		return m.startCreate(false)
	case homeNewProject:
		m.files.SetDir(writingDir())
		m.screen = screenWriting
		return m.startCreate(true)
	}
	m.screen = screenWriting
	m.layout()
	return nil
}

// startCreate opens the name prompt in file or folder mode.
func (m *model) startCreate(folder bool) tea.Cmd {
	m.creatingFile = true
	m.creatingFolder = folder
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m.editor.Blur()
	m.layout()
	return textinput.Blink
}
