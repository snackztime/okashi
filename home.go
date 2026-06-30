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
	"github.com/charmbracelet/x/ansi"
)

type homeKind int

const (
	homeRecentFile homeKind = iota
	homeProject
	homeFolder
	homeNewDocument
	homeNewProject
	homeOpenOther
)

// homeFileItem is one document in the FILES column: display name, path, word count, and a
// one-line opening snippet.
type homeFileItem struct {
	name, path, snippet string
	words               int
}

// classifyLibrary splits the workspace's top-level subdirs into manuscripts (projects) and
// plain category folders, alpha-sorted, excluding hidden dirs.
func classifyLibrary(workspace string) (projects, folders []homeItem) {
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		dir := filepath.Join(workspace, n)
		if dirIsManuscript(dir) {
			projects = append(projects, homeItem{kind: homeProject, label: n, path: dir})
		} else {
			folders = append(folders, homeItem{kind: homeFolder, label: n, path: dir})
		}
	}
	return
}

// dirIsManuscript reports whether dir resolves as an ordered manuscript (manifest or legacy
// numbered files) vs. a plain category folder.
func dirIsManuscript(dir string) bool {
	sub, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	var fes []fileEntry
	for _, e := range sub {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if allowedDocExts[strings.ToLower(filepath.Ext(e.Name()))] { // match homeFilesFor
			fes = append(fes, fileEntry{name: e.Name()})
		}
	}
	return resolveManuscript(dir, fes).ordered()
}

// homeFilesFor resolves a project/folder dir into its ordered documents (chapters in view
// order then loose, or a category's docs), each with word count + snippet.
func (m *model) homeFilesFor(dir string) []homeFileItem {
	sub, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var fes []fileEntry
	for _, e := range sub {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if m.files.allowed[strings.ToLower(filepath.Ext(e.Name()))] {
			fes = append(fes, fileEntry{name: e.Name()})
		}
	}
	view := resolveManuscript(dir, fes)
	mk := func(name, file string) homeFileItem {
		p := filepath.Join(dir, file)
		return homeFileItem{name: name, path: p, words: m.files.wc.count(p), snippet: m.snippets.get(p)}
	}
	var out []homeFileItem
	for _, ch := range view.chapters {
		out = append(out, mk(ch.title, ch.file))
	}
	for _, l := range view.loose {
		out = append(out, mk(l.name, l.name))
	}
	return out
}

// homeRegion identifies a navigable group on the launch screen: the Projects
// column, the Recent column, or the centered Actions below them.
type homeRegion int

const (
	regionRecent homeRegion = iota
	regionLibrary
	regionFiles
	regionActions
)

// Box widths (total, incl. the rounded border; framedPanel content = width-4).
const (
	homeRecentBox  = 20
	homeLibraryBox = 20
	homeFilesBox   = 36
	homeColGap     = 2
)

// homeItem is one selectable entry on the launch screen.
type homeItem struct {
	kind  homeKind
	label string
	path  string
}

// homeCell is a clickable item's position within the pre-Place content block.
type homeCell struct {
	region homeRegion
	index  int
	row    int
	x0, x1 int
}

// innerCell is a clickable item relative to a column's content (row 0 = first content
// line, x 0 = first content column); translated to a homeCell when the box is placed.
type innerCell struct {
	region homeRegion
	index  int
	row    int
	x0, x1 int
}

// buildHomeItems composes the flat launch list: recent files, then projects (manuscripts),
// then folders (categories), then the actions. The three-column launcher groups by kind.
func buildHomeItems(recents []string, workspace string) []homeItem {
	var items []homeItem
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}
	projects, folders := classifyLibrary(workspace)
	items = append(items, projects...)
	items = append(items, folders...)
	items = append(items,
		homeItem{kind: homeNewDocument, label: "New document"},
		homeItem{kind: homeNewProject, label: "New project"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
	return items
}

// homeGroups splits the flat list by kind.
func homeGroups(items []homeItem) (recents, projects, folders, actions []homeItem) {
	for _, it := range items {
		switch it.kind {
		case homeRecentFile:
			recents = append(recents, it)
		case homeProject:
			projects = append(projects, it)
		case homeFolder:
			folders = append(folders, it)
		default:
			actions = append(actions, it)
		}
	}
	return
}

func (m model) recents() []homeItem { r, _, _, _ := homeGroups(m.homeItems); return r }
func (m model) actions() []homeItem { _, _, _, a := homeGroups(m.homeItems); return a }
func (m model) library() []homeItem {
	_, projects, folders, _ := homeGroups(m.homeItems)
	return append(projects, folders...)
}

func (m model) regionCount(r homeRegion) int {
	switch r {
	case regionRecent:
		return len(m.recents())
	case regionLibrary:
		return len(m.library())
	case regionFiles:
		return len(m.homeFiles)
	default:
		return len(m.actions())
	}
}

// visibleRegions returns the regions actually rendered at the current width (per
// homeColumns) plus Actions — the single source nav and reset both consult so they can
// never focus a column the render dropped. Width 0 (unsized) treats all as visible.
func (m model) visibleRegions() []homeRegion {
	if m.width <= 0 {
		return []homeRegion{regionRecent, regionLibrary, regionFiles, regionActions}
	}
	regs, _, _ := m.homeColumns()
	return append(regs, regionActions)
}

// visibleCols is visibleRegions without Actions (the rendered columns).
func (m model) visibleCols() []homeRegion {
	var out []homeRegion
	for _, r := range m.visibleRegions() {
		if r != regionActions {
			out = append(out, r)
		}
	}
	return out
}

func (m model) regionVisible(r homeRegion) bool {
	for _, v := range m.visibleRegions() {
		if v == r {
			return true
		}
	}
	return false
}

// recomputeHomeFiles fills the FILES column from the selected library item.
func (m *model) recomputeHomeFiles() {
	lib := m.library()
	if m.librarySelected < 0 || m.librarySelected >= len(lib) {
		m.homeFiles = nil
		return
	}
	m.homeFiles = m.homeFilesFor(lib[m.librarySelected].path)
}

// resetHomeSelection focuses the first non-empty column (Recent first) and points the
// library at its first item so FILES is populated.
func (m *model) resetHomeSelection() {
	if m.librarySelected < 0 || m.librarySelected >= len(m.library()) {
		m.librarySelected = 0
	}
	m.recomputeHomeFiles()
	m.homeRegion = regionActions
	for _, r := range m.visibleRegions() {
		if m.regionCount(r) > 0 {
			m.homeRegion = r
			break
		}
	}
	m.homeLastCol = regionLibrary
	if vc := m.visibleCols(); len(vc) > 0 {
		m.homeLastCol = vc[0]
	}
	if m.homeRegion != regionActions {
		m.homeLastCol = m.homeRegion
	}
	if m.homeRegion == regionLibrary {
		m.homeIndex = m.librarySelected
	} else {
		m.homeIndex = 0
	}
}

// ensureVisibleFocus refocuses to the first visible non-empty region if the current focus
// became hidden (e.g. after a resize dropped its column).
func (m *model) ensureVisibleFocus() {
	if m.regionVisible(m.homeRegion) {
		return
	}
	for _, r := range m.visibleRegions() {
		if m.regionCount(r) > 0 {
			m.focusAt(r, m.indexIn(r))
			return
		}
	}
}

// updateHome handles input on the launch screen.
func (m model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisibleFocus() // a narrower width may have dropped the focused column
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
			doubled := r == m.homeRegion && idx == m.indexIn(r) &&
				time.Since(m.lastClickTime) < 400*time.Millisecond
			m.focusAt(r, idx)
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

// indexIn returns the active index within region r (library uses librarySelected).
func (m model) indexIn(r homeRegion) int {
	if r == regionLibrary {
		return m.librarySelected
	}
	if r == m.homeRegion {
		return m.homeIndex
	}
	return 0
}

// focusAt focuses region r at index idx (clamped; library drives FILES).
func (m *model) focusAt(r homeRegion, idx int) {
	n := m.regionCount(r)
	if n == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	m.homeRegion = r
	m.homeIndex = idx
	if r != regionActions {
		m.homeLastCol = r
	}
	if r == regionLibrary {
		m.librarySelected = idx
		m.recomputeHomeFiles()
	}
}

// homeMove navigates the launcher: up/down within a column (flowing into Actions),
// left/right across Recent ↔ Library ↔ Files.
func (m *model) homeMove(dx, dy int) {
	if dy != 0 {
		n := m.regionCount(m.homeRegion)
		if dy > 0 {
			if m.homeRegion != regionActions && m.homeIndex >= n-1 {
				if m.regionCount(regionActions) > 0 {
					m.homeLastCol = m.homeRegion
					m.focusAt(regionActions, 0)
				}
				return
			}
			if m.homeIndex < n-1 {
				m.focusAt(m.homeRegion, m.homeIndex+1)
			}
			return
		}
		if m.homeRegion == regionActions && m.homeIndex == 0 {
			target := m.homeLastCol
			if !m.regionVisible(target) {
				if vc := m.visibleCols(); len(vc) > 0 {
					target = vc[len(vc)-1]
				}
			}
			m.focusAt(target, m.regionCount(target)-1)
			return
		}
		if m.homeIndex > 0 {
			m.focusAt(m.homeRegion, m.homeIndex-1)
		}
		return
	}
	// Horizontal — only across the columns actually rendered at this width.
	cols := m.visibleCols()
	cur := indexOfRegion(cols, m.homeRegion)
	if m.homeRegion == regionActions {
		cur = indexOfRegion(cols, m.homeLastCol)
	}
	if cur < 0 {
		cur = 0
	}
	for nxt := cur + dx; nxt >= 0 && nxt < len(cols); nxt += dx {
		if m.regionCount(cols[nxt]) > 0 {
			m.focusAt(cols[nxt], m.indexIn(cols[nxt]))
			return
		}
	}
}

func indexOfRegion(cols []homeRegion, r homeRegion) int {
	for i, c := range cols {
		if c == r {
			return i
		}
	}
	return -1
}

// homeCycleRegion moves to the next/previous non-empty region.
func (m *model) homeCycleRegion(dir int) {
	order := m.visibleRegions()
	start := indexOfRegion(order, m.homeRegion)
	if start < 0 {
		start = 0
	}
	for i := 1; i <= len(order); i++ {
		r := order[((start+dir*i)%len(order)+len(order))%len(order)]
		if m.regionCount(r) > 0 {
			m.focusAt(r, m.indexIn(r))
			return
		}
	}
}

// --- rendering ---

func homeDim(s string) string { return lipgloss.NewStyle().Foreground(subtle).Render(s) }

func homeLabel(s string, sel bool) string {
	if sel {
		return selectedStyle.Render(s)
	}
	return s
}

// homeWindowOffset returns a scroll offset so the active row stays visible in h rows.
func homeWindowOffset(total, active, h int) int {
	if total <= h || h <= 0 {
		return 0
	}
	off := active - h/2
	if off < 0 {
		off = 0
	}
	if off > total-h {
		off = total - h
	}
	return off
}

// recentColumn builds the RECENT box's inner lines + cells (≤ h rows).
func (m model) recentColumn(h int) ([]string, []innerCell) {
	rec := m.recents()
	if len(rec) == 0 {
		return []string{homeDim("(no recent files)")}, nil
	}
	active := 0
	if m.homeRegion == regionRecent {
		active = m.homeIndex
	}
	off := homeWindowOffset(len(rec), active, h)
	var lines []string
	var cells []innerCell
	for i := off; i < len(rec) && len(lines) < h; i++ {
		sel := m.homeRegion == regionRecent && m.homeIndex == i
		txt := homeLabel(rec[i].label, sel)
		cells = append(cells, innerCell{regionRecent, i, len(lines), 0, lipgloss.Width(rec[i].label)})
		lines = append(lines, txt)
	}
	return lines, cells
}

// libraryColumn builds the LIBRARY box (PROJECTS + FOLDERS sections) + cells (≤ h rows).
func (m model) libraryColumn(h int) ([]string, []innerCell) {
	_, projects, folders, _ := homeGroups(m.homeItems)
	type lrow struct {
		header bool
		text   string
		libIdx int
	}
	var rows []lrow
	idx := 0
	if len(projects) > 0 {
		rows = append(rows, lrow{header: true, text: "PROJECTS"})
		for _, p := range projects {
			rows = append(rows, lrow{text: "› " + p.label, libIdx: idx})
			idx++
		}
	}
	if len(folders) > 0 {
		rows = append(rows, lrow{header: true, text: "FOLDERS"})
		for _, f := range folders {
			rows = append(rows, lrow{text: "› " + f.label, libIdx: idx})
			idx++
		}
	}
	if len(rows) == 0 {
		return []string{homeDim("(no projects)")}, nil
	}
	// Find the active row (the selected library item) and window around it.
	activeRow := 0
	for i, r := range rows {
		if !r.header && r.libIdx == m.librarySelected {
			activeRow = i
		}
	}
	off := homeWindowOffset(len(rows), activeRow, h)
	var lines []string
	var cells []innerCell
	for i := off; i < len(rows) && len(lines) < h; i++ {
		r := rows[i]
		if r.header {
			lines = append(lines, homeDim(r.text))
			continue
		}
		sel := m.homeRegion == regionLibrary && m.librarySelected == r.libIdx
		cells = append(cells, innerCell{regionLibrary, r.libIdx, len(lines), 0, lipgloss.Width(r.text)})
		lines = append(lines, homeLabel(r.text, sel))
	}
	return lines, cells
}

// filesColumn builds the FILES box: two lines per file (name+count, dim snippet) + cells.
func (m model) filesColumn(h, contentW int) ([]string, []innerCell) {
	if len(m.homeFiles) == 0 {
		return []string{homeDim("(empty)")}, nil
	}
	active := 0
	if m.homeRegion == regionFiles {
		active = m.homeIndex
	}
	perView := h / 2
	if perView < 1 {
		perView = 1
	}
	off := homeWindowOffset(len(m.homeFiles), active, perView)
	var lines []string
	var cells []innerCell
	for i := off; i < len(m.homeFiles) && len(lines)+2 <= h; i++ {
		f := m.homeFiles[i]
		sel := m.homeRegion == regionFiles && m.homeIndex == i
		count := commafy(f.words)
		name := f.name
		maxName := contentW - lipgloss.Width(count) - 1
		if maxName < 1 {
			maxName = 1
		}
		name = ansi.Truncate(name, maxName, "…")
		gap := contentW - lipgloss.Width(name) - lipgloss.Width(count)
		if gap < 1 {
			gap = 1
		}
		nameLine := homeLabel(name, sel) + strings.Repeat(" ", gap) + homeDim(count)
		snip := homeDim(ansi.Truncate(f.snippet, contentW, "…"))
		row := len(lines)
		cells = append(cells,
			innerCell{regionFiles, i, row, 0, contentW},
			innerCell{regionFiles, i, row + 1, 0, contentW})
		lines = append(lines, nameLine, snip)
	}
	return lines, cells
}

// homeColumns returns the boxes to show (responsive) with their widths + titles.
func (m model) homeColumns() (regions []homeRegion, titles []string, widths []int) {
	all := []homeRegion{regionRecent, regionLibrary, regionFiles}
	allTitles := []string{"RECENT", "LIBRARY", "FILES"}
	allW := []int{homeRecentBox, homeLibraryBox, homeFilesBox}
	total := func(idx []int) int {
		w := 0
		for _, i := range idx {
			w += allW[i] + homeColGap
		}
		return w - homeColGap
	}
	// Prefer all three; drop RECENT, then LIBRARY, to fit the width.
	for _, idx := range [][]int{{0, 1, 2}, {1, 2}, {2}} {
		if total(idx) <= m.width {
			for _, i := range idx {
				regions = append(regions, all[i])
				titles = append(titles, allTitles[i])
				widths = append(widths, allW[i])
			}
			return
		}
	}
	return []homeRegion{regionFiles}, []string{"FILES"}, []int{m.width}
}

// homeContent assembles the centered logo, the framed columns, and the actions, plus the
// clickable cells in block-relative coords (render == hit-test).
func (m model) homeContent() (lines []string, cells []homeCell, blockW int) {
	regions, titles, widths := m.homeColumns()
	availH := m.height - 8
	if availH < 4 {
		availH = 4
	}

	type built struct {
		inner []string
		cells []innerCell
		w     int
	}
	cols := make([]built, len(regions))
	maxH := 1
	for i, r := range regions {
		contentW := widths[i] - 4
		var inner []string
		var ics []innerCell
		switch r {
		case regionRecent:
			inner, ics = m.recentColumn(availH - 2)
		case regionLibrary:
			inner, ics = m.libraryColumn(availH - 2)
		default:
			inner, ics = m.filesColumn(availH-2, contentW)
		}
		cols[i] = built{inner, ics, widths[i]}
		if len(inner) > maxH {
			maxH = len(inner)
		}
	}
	colH := maxH + 2 // framed

	// blockW + each box's x origin.
	blockW = 0
	xorg := make([]int, len(cols))
	for i := range cols {
		xorg[i] = blockW
		blockW += cols[i].w
		if i < len(cols)-1 {
			blockW += homeColGap
		}
	}

	// Logo, centered over blockW.
	pad := func(s string) string {
		left := (blockW - lipgloss.Width(s)) / 2
		if left < 0 {
			left = 0
		}
		return strings.Repeat(" ", left) + s
	}
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, pad(bannerStyle.Render(l)))
	}
	lines = append(lines, "")
	boxTop := len(lines) // block row where the framed boxes begin

	// Frame each column and join horizontally.
	framed := make([]string, len(cols))
	for i := range cols {
		framed[i] = framedPanel(titles[i], strings.Join(cols[i].inner, "\n"), cols[i].w, colH, "")
	}
	gap := strings.Repeat(" ", homeColGap)
	parts := make([]string, 0, len(framed)*2)
	for i, f := range framed {
		if i > 0 {
			parts = append(parts, gap)
		}
		parts = append(parts, f)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	lines = append(lines, strings.Split(row, "\n")...)

	// Translate inner cells → block cells (box col +2 for "│ ", box row +1 for top border).
	for i := range cols {
		for _, c := range cols[i].cells {
			cells = append(cells, homeCell{
				region: c.region, index: c.index,
				row: boxTop + 1 + c.row,
				x0:  xorg[i] + 2 + c.x0,
				x1:  xorg[i] + 2 + c.x1,
			})
		}
	}

	// Actions, centered below.
	lines = append(lines, "")
	acts := m.actions()
	if len(acts) > 0 {
		labels := make([]string, len(acts))
		for i, a := range acts {
			labels[i] = a.label
		}
		// Lay the actions out on one centered row with gaps; record each cell.
		const asep = "    "
		full := strings.Join(labels, asep)
		left := (blockW - lipgloss.Width(full)) / 2
		if left < 0 {
			left = 0
		}
		arow := len(lines)
		col := left
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", left))
		for i, a := range acts {
			if i > 0 {
				b.WriteString(asep)
				col += lipgloss.Width(asep)
			}
			sel := m.homeRegion == regionActions && m.homeIndex == i
			b.WriteString(homeLabel(a.label, sel))
			cells = append(cells, homeCell{regionActions, i, arow, col, col + lipgloss.Width(a.label)})
			col += lipgloss.Width(a.label)
		}
		lines = append(lines, b.String())
	}
	return lines, cells, blockW
}

func (m model) homeView() string {
	lines, _, blockW := m.homeContent()
	block := lipgloss.NewStyle().Width(blockW).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

// homeItemAt maps an absolute screen (x,y) to a launch (region, index).
func (m model) homeItemAt(x, y int) (homeRegion, int, bool) {
	lines, cells, blockW := m.homeContent()
	xoff := (m.width - blockW) / 2
	yoff := (m.height - len(lines)) / 2
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

// openHomeSelection acts on the focused launch item and enters writing mode.
func (m *model) openHomeSelection() tea.Cmd {
	switch m.homeRegion {
	case regionRecent:
		rec := m.recents()
		if m.homeIndex < len(rec) {
			p := rec[m.homeIndex].path
			m.files.SetDir(filepath.Dir(p))
			m.loadFile(p)
			m.focus = focusEditor
			m.editor.Focus()
			m.screen = screenWriting
			m.layout()
		}
		return nil
	case regionFiles:
		if m.homeIndex < len(m.homeFiles) {
			p := m.homeFiles[m.homeIndex].path
			m.files.SetDir(filepath.Dir(p))
			m.loadFile(p)
			m.focus = focusEditor
			m.editor.Focus()
			m.screen = screenWriting
			m.layout()
		}
		return nil
	case regionLibrary:
		lib := m.library()
		if m.librarySelected < len(lib) {
			m.files.SetDir(lib[m.librarySelected].path)
			m.focus = focusSidebar
			m.editor.Blur()
			m.screen = screenWriting
			m.layout()
		}
		return nil
	default: // actions
		acts := m.actions()
		if m.homeIndex >= len(acts) {
			return nil
		}
		switch acts[m.homeIndex].kind {
		case homeNewDocument:
			m.files.SetDir(writingDir())
			m.screen = screenWriting
			return m.startCreate(false)
		case homeNewProject:
			m.files.SetDir(writingDir())
			m.screen = screenWriting
			return m.startCreate(true)
		case homeOpenOther:
			m.files.SetDir(writingDir())
			m.focus = focusSidebar
			m.editor.Blur()
			m.screen = screenWriting
			m.layout()
		}
		return nil
	}
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
