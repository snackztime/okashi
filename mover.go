package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type moverEntryKind int

const (
	moverMoveHere moverEntryKind = iota // "→ move into <current folder>"
	moverUp                             // "‹ .."
	moverFolder                         // "▸ name/"
)

type moverEntry struct {
	name, path string
	kind       moverEntryKind
}

// enterMover opens the file mover for the file pane's selected entry (contextual entry). The
// destination browser starts at the active source root.
func (m *model) enterMover() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	m.moverSource = filepath.Join(m.files.dir, e.name)
	m.moverIsDir = e.isDir
	m.moverFromDir = m.files.dir
	m.moverDestDir = m.activeSourceRoot()
	m.moverSel = 0
	m.moverConfirm = false
	m.moverAsChapter = true
	m.moverReturn = screenWriting
	m.moverReload()
	m.screen = screenMover
}

// moverReload rebuilds the destination browser rows for moverDestDir: a leading "move here" row,
// a ".." row when below the active source root, then the subfolders (alpha-sorted).
func (m *model) moverReload() {
	root := m.activeSourceRoot()
	var rows []moverEntry
	rows = append(rows, moverEntry{name: filepath.Base(m.moverDestDir), path: m.moverDestDir, kind: moverMoveHere})
	if m.moverDestDir != root && withinRoot(m.moverDestDir, root) {
		rows = append(rows, moverEntry{name: "..", path: filepath.Dir(m.moverDestDir), kind: moverUp})
	}
	if ents, err := os.ReadDir(m.moverDestDir); err == nil {
		var dirs []moverEntry
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, moverEntry{name: e.Name(), path: filepath.Join(m.moverDestDir, e.Name()), kind: moverFolder})
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
		rows = append(rows, dirs...)
	}
	m.moverEntries = rows
	if m.moverSel >= len(rows) {
		m.moverSel = len(rows) - 1
	}
	if m.moverSel < 0 {
		m.moverSel = 0
	}
}

func (m model) updateMover(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = m.moverReturn
			return m, nil
		case "up", "k":
			if m.moverSel > 0 {
				m.moverSel--
			}
		case "down", "j":
			if m.moverSel < len(m.moverEntries)-1 {
				m.moverSel++
			}
		case "enter":
			if m.moverSel < 0 || m.moverSel >= len(m.moverEntries) {
				return m, nil
			}
			e := m.moverEntries[m.moverSel]
			switch e.kind {
			case moverUp, moverFolder:
				m.moverDestDir = e.path
				m.moverSel = 0
				m.moverReload()
			case moverMoveHere:
				// commit target = moverDestDir → confirm (Task 2)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) moverView() string {
	// LEFT: the item being moved.
	srcName := filepath.Base(m.moverSource)
	kindLabel := "file"
	if m.moverIsDir {
		kindLabel = "folder"
	}
	left := "moving " + kindLabel + ":\n" + srcName + "\n\nfrom: " + filepath.Base(m.moverFromDir)
	leftPanel := framedPanel("MOVE", left, 26, 8, "")

	// RIGHT: the destination browser (windowed).
	visRows := m.height - 8
	if visRows < 1 {
		visRows = 1
	}
	off := homeWindowOffset(len(m.moverEntries), m.moverSel, visRows)
	var rows []string
	for i := off; i < len(m.moverEntries) && len(rows) < visRows; i++ {
		e := m.moverEntries[i]
		var text string
		switch e.kind {
		case moverMoveHere:
			text = "→ move into " + e.name + "/"
		case moverUp:
			text = "‹ .."
		default:
			text = "▸ " + e.name + "/"
		}
		if i == m.moverSel {
			text = selectedStyle.Render(text)
		}
		rows = append(rows, text)
	}
	rightW := max(30, min(m.width-30, 44))
	rightPanel := framedPanel("TO "+filepath.Base(m.moverDestDir), strings.Join(rows, "\n"), rightW, len(rows)+2, "")

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ browse · enter drill/select · esc cancel")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
