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

// homeRegion identifies a navigable group on the launch screen: the Projects
// column, the Recent column, or the centered Actions below them.
type homeRegion int

const (
	regionProjects homeRegion = iota
	regionRecent
	regionActions
)

// homeItem is one selectable entry on the launch screen.
type homeItem struct {
	kind  homeKind
	label string // basename / project name / action label
	path  string // file path, project dir, or "" for the action
}

// homeCell is a clickable item's position within the pre-Place content block
// (content-relative row + column range), so render and hit-test share one layout.
type homeCell struct {
	region homeRegion
	index  int
	row    int
	x0, x1 int
}

// buildHomeItems composes the launch list: recent files (most-recent-first),
// then project folders (immediate non-hidden subdirs of projectsDir, alpha),
// then the new-document/new-project/browse actions. The two-column launch view
// groups these by kind.
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

// homeGroups splits the flat item list into the three launch regions.
func homeGroups(items []homeItem) (projects, recents, actions []homeItem) {
	for _, it := range items {
		switch it.kind {
		case homeProject:
			projects = append(projects, it)
		case homeRecentFile:
			recents = append(recents, it)
		default:
			actions = append(actions, it)
		}
	}
	return
}

// regionItems returns the items for region r.
func (m model) regionItems(r homeRegion) []homeItem {
	projects, recents, actions := homeGroups(m.homeItems)
	switch r {
	case regionProjects:
		return projects
	case regionRecent:
		return recents
	default:
		return actions
	}
}

// resetHomeSelection places the cursor on the first non-empty region (Projects,
// then Recent, then Actions) at index 0.
func (m *model) resetHomeSelection() {
	projects, recents, _ := homeGroups(m.homeItems)
	switch {
	case len(projects) > 0:
		m.homeRegion = regionProjects
	case len(recents) > 0:
		m.homeRegion = regionRecent
	default:
		m.homeRegion = regionActions
	}
	m.homeIndex = 0
	m.homeLastCol = regionProjects
	if len(projects) == 0 && len(recents) > 0 {
		m.homeLastCol = regionRecent
	}
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
			m.homeMove(0, -1)
		case "down", "j":
			m.homeMove(0, 1)
		case "left", "h":
			m.homeMove(-1, 0)
		case "right", "l":
			m.homeMove(1, 0)
		case "tab":
			m.homeCycleRegion(1)
		case "shift+tab":
			m.homeCycleRegion(-1)
		case "enter":
			return m, m.openHomeSelection()
		}
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.homeMove(0, -1)
		case tea.MouseButtonWheelDown:
			m.homeMove(0, 1)
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			r, idx, ok := m.homeItemAt(msg.X, msg.Y)
			if !ok {
				return m, nil
			}
			doubled := r == m.homeRegion && idx == m.homeIndex &&
				time.Since(m.lastClickTime) < 400*time.Millisecond
			m.homeRegion, m.homeIndex = r, idx
			if r != regionActions {
				m.homeLastCol = r
			}
			if doubled {
				m.lastClickTime = time.Time{}
				return m, m.openHomeSelection()
			}
			m.lastClickTime = time.Now()
		}
		return m, nil
	}
	return m, nil
}

// homeMove navigates the 2-D launch grid. dx: -1 left / +1 right (Projects↔Recent
// columns); dy: -1 up / +1 down (within a region, with the columns flowing into the
// Actions block below).
func (m *model) homeMove(dx, dy int) {
	cur := m.regionItems(m.homeRegion)
	if dy < 0 {
		if m.homeRegion == regionActions && m.homeIndex == 0 {
			// up out of Actions → back to the last column used.
			col := m.homeLastCol
			if len(m.regionItems(col)) == 0 {
				col = regionProjects
				if len(m.regionItems(col)) == 0 {
					col = regionRecent
				}
			}
			if items := m.regionItems(col); len(items) > 0 {
				m.homeRegion = col
				m.homeIndex = len(items) - 1
			}
		} else if m.homeIndex > 0 {
			m.homeIndex--
		}
		return
	}
	if dy > 0 {
		if m.homeRegion != regionActions && m.homeIndex >= len(cur)-1 {
			if acts := m.regionItems(regionActions); len(acts) > 0 {
				m.homeLastCol = m.homeRegion
				m.homeRegion = regionActions
				m.homeIndex = 0
			}
		} else if m.homeIndex < len(cur)-1 {
			m.homeIndex++
		}
		return
	}
	// Horizontal: switch between the two columns (or from Actions into a column).
	target := m.homeRegion
	switch {
	case dx < 0:
		target = regionProjects
	case dx > 0:
		target = regionRecent
	}
	if target == m.homeRegion {
		return
	}
	if items := m.regionItems(target); len(items) > 0 {
		m.homeRegion = target
		m.homeLastCol = target
		if m.homeIndex > len(items)-1 {
			m.homeIndex = len(items) - 1
		}
	}
}

// homeCycleRegion moves to the next/previous non-empty region.
func (m *model) homeCycleRegion(dir int) {
	order := []homeRegion{regionProjects, regionRecent, regionActions}
	start := int(m.homeRegion)
	for i := 1; i <= len(order); i++ {
		r := order[((start+dir*i)%len(order)+len(order))%len(order)]
		if len(m.regionItems(r)) > 0 {
			m.homeRegion = r
			m.homeIndex = 0
			if r != regionActions {
				m.homeLastCol = r
			}
			return
		}
	}
}

// homeContent builds the launch block: the centered logo, the Projects/Recent
// columns, and the centered Actions. It returns the lines, the clickable cells
// (content-relative coords), and the block width — so render and hit-test agree.
func (m model) homeContent() (lines []string, cells []homeCell, blockW int) {
	projects, recents, actions := homeGroups(m.homeItems)
	hdr := lipgloss.NewStyle().Foreground(subtle).Bold(true)

	// Item label incl. icon + the 1-col selection gutter on each side.
	itemText := func(it homeItem, sel bool) string {
		g := m.homeGlyph(it)
		if sel {
			return selectedStyle.Render(" " + renderIcon(g, true) + it.label + " ")
		}
		return " " + renderIcon(g, false) + it.label + " "
	}
	colWidth := func(header string, items []homeItem) int {
		w := lipgloss.Width(header)
		for _, it := range items {
			if x := lipgloss.Width(itemText(it, false)); x > w {
				w = x
			}
		}
		return w
	}

	projW := colWidth("PROJECTS", projects)
	recW := colWidth("RECENT", recents)
	const gap = 4
	colsW := projW + gap + recW

	logoLines := strings.Split(bannerArt, "\n")
	logoW := 0
	for _, l := range logoLines {
		if x := lipgloss.Width(l); x > logoW {
			logoW = x
		}
	}
	actW := 0
	for _, it := range actions {
		if x := lipgloss.Width(itemText(it, false)); x > actW {
			actW = x
		}
	}

	blockW = colsW
	if logoW > blockW {
		blockW = logoW
	}
	if actW > blockW {
		blockW = actW
	}

	pad := func(s string, w int) string { // left-pad to center s within w
		left := (w - lipgloss.Width(s)) / 2
		if left < 0 {
			left = 0
		}
		return strings.Repeat(" ", left) + s
	}

	// Logo.
	for _, l := range logoLines {
		lines = append(lines, pad(bannerStyle.Render(l), blockW))
	}
	lines = append(lines, "")

	// Column headers + rows.
	colsLeft := (blockW - colsW) / 2
	if colsLeft < 0 {
		colsLeft = 0
	}
	leftPad := strings.Repeat(" ", colsLeft)
	cell := func(s string, w int) string { return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s))) }

	lines = append(lines, leftPad+cell(hdr.Render("PROJECTS"), projW)+strings.Repeat(" ", gap)+hdr.Render("RECENT"))
	rows := len(projects)
	if len(recents) > rows {
		rows = len(recents)
	}
	for i := 0; i < rows; i++ {
		row := len(lines)
		left, right := "", ""
		if i < len(projects) {
			sel := m.homeRegion == regionProjects && m.homeIndex == i
			left = itemText(projects[i], sel)
			cells = append(cells, homeCell{regionProjects, i, row, colsLeft, colsLeft + lipgloss.Width(left)})
		}
		if i < len(recents) {
			sel := m.homeRegion == regionRecent && m.homeIndex == i
			right = itemText(recents[i], sel)
			rx := colsLeft + projW + gap
			cells = append(cells, homeCell{regionRecent, i, row, rx, rx + lipgloss.Width(right)})
		}
		lines = append(lines, leftPad+cell(left, projW)+strings.Repeat(" ", gap)+right)
	}

	// Actions, centered.
	lines = append(lines, "")
	for i, it := range actions {
		sel := m.homeRegion == regionActions && m.homeIndex == i
		s := itemText(it, sel)
		left := (blockW - lipgloss.Width(s)) / 2
		if left < 0 {
			left = 0
		}
		cells = append(cells, homeCell{regionActions, i, len(lines), left, left + lipgloss.Width(s)})
		lines = append(lines, strings.Repeat(" ", left)+s)
	}
	return lines, cells, blockW
}

// homeGlyph picks the icon for a launch item.
func (m model) homeGlyph(it homeItem) glyph {
	switch it.kind {
	case homeProject, homeOpenOther:
		return m.icons.folder
	case homeNewDocument, homeNewProject:
		return m.icons.action
	default:
		return m.icons.iconFor(fileEntry{name: it.label})
	}
}

func (m model) homeView() string {
	lines, _, blockW := m.homeContent()
	block := strings.Join(lines, "\n")
	// Place the fixed-width block centered; lipgloss.Place centers by the block's
	// own width so the columns stay aligned.
	block = lipgloss.NewStyle().Width(blockW).Render(block)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

// homeItemAt maps an absolute screen (x,y) to a launch (region, index).
func (m model) homeItemAt(x, y int) (homeRegion, int, bool) {
	lines, cells, blockW := m.homeContent()
	blockH := len(lines)
	xoff := (m.width - blockW) / 2
	yoff := (m.height - blockH) / 2
	if xoff < 0 {
		xoff = 0
	}
	if yoff < 0 {
		yoff = 0
	}
	cx, cy := x-xoff, y-yoff
	for _, c := range cells {
		if c.row == cy && cx >= c.x0 && cx < c.x1 {
			return c.region, c.index, true
		}
	}
	return 0, 0, false
}

// openHomeSelection acts on the highlighted launch item and enters writing mode.
func (m *model) openHomeSelection() tea.Cmd {
	items := m.regionItems(m.homeRegion)
	if m.homeIndex < 0 || m.homeIndex >= len(items) {
		return nil
	}
	it := items[m.homeIndex]
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
