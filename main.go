package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"okashi/internal/textarea"
)

// version is overridden at build time via -ldflags "-X main.version=…".
// The Homebrew formula injects the release tag here.
var version = "dev"

const usage = `okashi — a minimal terminal writing app

Usage:
  okashi              open the writing app
  okashi --version    print the version
  okashi --help       show this help

Files open in $OKASHI_DIR, else iCloud Drive's okashi folder, else
~/Documents/okashi. Inside the app, ctrl+p toggles a Markdown preview.`

// defaultColumnWidth is the target writing measure (the readable "ideal
// measure" is ~66). Override with OKASHI_WIDTH.
const defaultColumnWidth = 72

// resolveColumnWidth reads OKASHI_WIDTH (a column count in [20,200]); otherwise
// defaultColumnWidth.
func resolveColumnWidth() int {
	if v := os.Getenv("OKASHI_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 200 {
			return n
		}
	}
	return defaultColumnWidth
}

// resolveSmartQuotes reads OKASHI_SMARTQUOTES; off/false/0 disable, default on.
func resolveSmartQuotes() bool {
	switch strings.ToLower(os.Getenv("OKASHI_SMARTQUOTES")) {
	case "off", "false", "0":
		return false
	}
	return true
}

// smartQuote returns the curly form of a straight quote. It's an opening quote
// at the start of a line or after whitespace / an opening bracket; otherwise
// closing (which also yields the right apostrophe in contractions).
func smartQuote(prev rune, hasPrev bool, q rune) rune {
	opening := !hasPrev || prev == ' ' || prev == '\t' || prev == '\n' ||
		prev == '(' || prev == '[' || prev == '{'
	switch q {
	case '\'':
		if opening {
			return rune(0x2018) // U+2018 left single quote
		}
		return rune(0x2019) // U+2019 right single quote
	case '"':
		if opening {
			return rune(0x201C) // U+201C left double quote
		}
		return rune(0x201D) // U+201D right double quote
	}
	return q
}

var listItemRe = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+(.*)$`)

// listContinuation inspects a list line for Enter handling. ok=false means it's
// not a list item (normal Enter). clear=true means the item is empty → end the
// list. Otherwise prefix is inserted after a newline to continue the list.
func listContinuation(line string) (prefix string, clear bool, ok bool) {
	mtch := listItemRe.FindStringSubmatch(line)
	if mtch == nil {
		return "", false, false
	}
	indent, marker, content := mtch[1], mtch[2], mtch[3]
	if strings.TrimSpace(content) == "" {
		return "", true, true
	}
	next := marker
	if n, err := strconv.Atoi(strings.TrimSuffix(marker, ".")); err == nil {
		next = strconv.Itoa(n+1) + "."
	}
	return indent + next + " ", false, true
}

type focus int

const (
	focusSidebar focus = iota
	focusEditor
)

type screen int

const (
	screenHome screen = iota
	screenWriting
	screenOutline
	screenManuscript
)

// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir     string // directory containing the item
	name    string // current base name
	isDir   bool
	section bool // a numbered section -> title-only rename
}

type model struct {
	width, height int

	files     filelist
	editor    textarea.Model
	nameInput textinput.Model
	preview   viewport.Model

	screen       screen
	homeItems    []homeItem
	homeSelected int

	sidebarVisible bool
	focus          focus
	creatingFile   bool
	creatingFolder bool
	previewing     bool
	typewriter     bool
	dimEnabled     bool

	mdStyle         string // glamour theme, detected once at startup
	colWidth        int
	smartQuotes     bool
	sessionBaseline int // word count when the current file was opened/created
	currentFile     string
	status          string
	icons           iconSet
	outline         outlineModel
	outlineCreating bool

	pager pagerModel

	renaming     bool
	renameTarget renameTarget

	convertPrompt bool
	exportPrompt  bool

	lastClickRow  int
	lastClickTime time.Time

	dirty      bool
	lastEditAt time.Time
}

func initialModel() model {
	fl := newFilelist()
	fl.root = writingDir()
	fl.SetDir(writingDir())

	ta := textarea.New()
	ta.Placeholder = "Start writing…"
	ta.Prompt = "" // no gutter pipe — read like paper, not code
	ta.ShowLineNumbers = false
	ta.CharLimit = 0 // unlimited
	ta.Focus()

	// Strip the textarea's built-in chrome so the prose stands alone.
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Typewriter = true // typewriter scrolling on by default; ctrl+t toggles
	ta.Dim = true
	ta.DimStyle = lipgloss.NewStyle().Foreground(subtle)

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "chapter-01.md"
	ti.CharLimit = 255

	vp := viewport.New(defaultColumnWidth, 1) // real size set in layout()

	return model{
		files:          fl,
		editor:         ta,
		nameInput:      ti,
		preview:        vp,
		mdStyle:        previewStyle(),
		colWidth:       resolveColumnWidth(),
		smartQuotes:    resolveSmartQuotes(),
		screen:         screenHome,
		homeItems:      buildHomeItems(loadRecents(recentPath()), writingDir()),
		sidebarVisible: true,
		focus:          focusSidebar,
		typewriter:     true,
		dimEnabled:     true,
		status:         "ctrl+b sidebar · esc switch · ctrl+n new · r rename · ctrl+l outline · ctrl+p preview · ctrl+t typewriter · ctrl+d dim · ctrl+s save · ctrl+c quit",
		icons:          resolveIcons(),
	}
}

// previewStyle picks the glamour theme for the markdown preview. It's resolved
// once, here, because initialModel() runs *before* Bubble Tea takes over the
// terminal — so the one terminal-background query happens on the normal screen
// and can't race Bubble Tea for stdin. $OKASHI_THEME forces a choice.
func previewStyle() string {
	switch strings.ToLower(os.Getenv("OKASHI_THEME")) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	if termenv.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

// writingDir returns the folder okashi opens in on launch. Priority:
//  1. $OKASHI_DIR, if set — lets a user point okashi anywhere.
//  2. iCloud Drive's okashi folder, when iCloud Drive exists on this Mac.
//  3. ~/Documents/okashi as a cross-platform fallback (e.g. iCloud off, Linux).
//
// The chosen folder is created lazily on first run — nothing is written at
// install time, so a Homebrew formula only needs to drop the binary.
func writingDir() string {
	if dir := os.Getenv("OKASHI_DIR"); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}

	// "com~apple~CloudDocs" is Apple's fixed iCloud Drive container id — it's
	// identical for every macOS user, so this path is safe to hardcode.
	icloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	dir := filepath.Join(home, "Documents", "okashi")
	if fi, err := os.Stat(icloud); err == nil && fi.IsDir() {
		dir = filepath.Join(icloud, "okashi")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return dir
}

type autosaveTickMsg time.Time

// autosaveTick schedules the next autosave check. One loop runs for the app's
// lifetime, started in Init.
func autosaveTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return autosaveTickMsg(t) })
}

// autosaveDue reports whether the buffer should be flushed: there are unsaved
// edits to a real file and the writer has paused for at least 2s.
func (m model) autosaveDue(now time.Time) bool {
	return m.dirty && m.currentFile != "" && now.Sub(m.lastEditAt) >= 2*time.Second
}

// sidebarRow maps an absolute mouse Y to a row index within the file list, or
// -1 if the click is outside the list. The list starts just below the banner.
func sidebarRow(mouseY, bannerH, listHeight int) int {
	row := mouseY - bannerH
	if row < 0 || row >= listHeight {
		return -1
	}
	return row
}

// syncDim keeps the editor's dim state in step with focus mode: dim only when
// typewriter AND dimEnabled.
func (m *model) syncDim() {
	m.editor.Dim = m.typewriter && m.dimEnabled
}

func (m model) Init() tea.Cmd {
	return autosaveTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if t, ok := msg.(autosaveTickMsg); ok {
		if m.autosaveDue(time.Time(t)) {
			m.save()
		}
		return m, autosaveTick()
	}

	if m.screen == screenHome {
		return m.updateHome(msg)
	}

	if m.screen == screenOutline {
		return m.updateOutline(msg)
	}

	if m.screen == screenManuscript {
		return m.updateManuscript(msg)
	}

	// While naming a new file, the prompt captures all input.
	if m.creatingFile {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.creatingFile = false
				m.creatingFolder = false
				m.nameInput.Blur()
				m.status = "create cancelled"
				return m, nil
			case "enter":
				m.confirmCreate()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.renaming = false
				m.nameInput.Blur()
				m.status = "rename cancelled"
				m.refreshAfterRename()
				return m, nil
			case "enter":
				m.confirmRename()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.convertPrompt {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				m.convertPrompt = false
				m.convertToManuscript()
				return m, nil
			case "n", "esc":
				m.convertPrompt = false
				m.status = "convert cancelled"
				return m, nil
			}
		}
		return m, nil
	}

	if m.exportPrompt {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "m":
				m.exportPrompt = false
				m.runExport(StyleManuscript)
				return m, nil
			case "t":
				m.exportPrompt = false
				m.runExport(StyleTufte)
				return m, nil
			case "esc":
				m.exportPrompt = false
				m.status = "export cancelled"
				return m, nil
			}
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.MouseMsg:
		inSidebar := m.sidebarVisible && msg.X < sidebarWidth

		// Wheel scrolls whichever pane is under the pointer.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			up := msg.Button == tea.MouseButtonWheelUp
			switch {
			case inSidebar:
				m.focus = focusSidebar
				m.editor.Blur()
				if up {
					m.files.moveBy(-1)
				} else {
					m.files.moveBy(1)
				}
				return m, nil
			case m.previewing:
				// Read-only preview: let the viewport scroll itself.
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			default:
				// Editor: with typewriter the view is caret-locked, so scrolling
				// means moving the caret. Step by the viewport's wheel delta.
				m.focus = focusEditor
				m.editor.Focus()
				const wheelStep = 3
				for i := 0; i < wheelStep; i++ {
					if up {
						m.editor.CursorUp()
					} else {
						m.editor.CursorDown()
					}
				}
				return m, nil
			}
		}

		// Click selection / open is file-pane only.
		if !inSidebar || msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if msg.Y == 0 {
			_, hits := m.files.breadcrumbBar(sidebarWidth - 3)
			col := msg.X - 1 // sidebar left padding
			for _, h := range hits {
				if col >= h.start && col < h.end {
					m.files.SetDir(h.path)
					m.focus = focusSidebar
					m.editor.Blur()
					break
				}
			}
			return m, nil
		}
		// Breadcrumb header occupies row 0; the file list starts at row 1.
		row := sidebarRow(msg.Y, 1, m.files.height)
		if row < 0 {
			return m, nil
		}
		m.focus = focusSidebar
		m.editor.Blur()
		m.files.selectRow(row)
		now := time.Now()
		if row == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if path, ok := m.files.activate(); ok {
				m.loadFile(path)
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{} // consume the double-click
		} else {
			m.lastClickRow = row
			m.lastClickTime = now
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			m.previewing = false
			m.screen = screenHome
			m.homeItems = buildHomeItems(loadRecents(recentPath()), writingDir())
			m.homeSelected = 0
			return m, nil
		case "ctrl+n":
			m.previewing = false
			m.creatingFile = true
			m.creatingFolder = false
			m.nameInput.SetValue("")
			m.nameInput.Focus()
			m.editor.Blur()
			m.status = ""
			return m, textinput.Blink
		case "ctrl+p":
			m.togglePreview()
			return m, nil
		case "ctrl+t":
			m.typewriter = !m.typewriter
			m.editor.Typewriter = m.typewriter
			m.syncDim()
			if m.typewriter {
				m.status = "typewriter on"
			} else {
				m.status = "typewriter off"
			}
			return m, nil
		case "ctrl+l":
			switch {
			case isManuscript(m.files.entries):
				m.enterOutline()
			case m.hasConvertibleFiles():
				m.convertPrompt = true
				m.status = "make this a manuscript? (y / n)"
			default:
				m.status = "nothing to convert (no documents here)"
			}
			return m, nil
		case "ctrl+e":
			m.exportPrompt = true
			m.status = "export: m manuscript · t tufte · esc cancel"
			return m, nil
		case "ctrl+d":
			m.dimEnabled = !m.dimEnabled
			m.syncDim()
			if m.editor.Dim {
				m.status = "dim on"
			} else {
				m.status = "dim off"
			}
			return m, nil
		case "ctrl+b":
			m.sidebarVisible = !m.sidebarVisible
			if !m.sidebarVisible {
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.layout()
			return m, nil
		case "esc":
			if m.previewing {
				m.togglePreview() // exit preview
				return m, nil
			}
			if m.sidebarVisible {
				if m.focus == focusSidebar {
					m.focus = focusEditor
					m.editor.Focus()
				} else {
					m.focus = focusSidebar
					m.editor.Blur()
				}
			}
			return m, nil
		case "tab":
			if m.focus == focusEditor && !m.previewing {
				m.editor.Indent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
		case "shift+tab":
			if m.focus == focusEditor && !m.previewing {
				m.editor.Outdent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
		case "ctrl+s":
			m.save()
			return m, nil
		}
	}

	// Route everything else to whichever pane has focus. Preview is a modal
	// read-only state, so it wins over pane focus — keys/wheel scroll it and
	// nothing reaches the editor or file list underneath.
	var cmd tea.Cmd
	if m.previewing {
		m.preview, cmd = m.preview.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.focus == focusSidebar && m.sidebarVisible {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "up", "k":
				m.files.moveBy(-1)
			case "down", "j":
				m.files.moveBy(1)
			case "enter", "right", "l":
				if path, ok := m.files.activate(); ok {
					m.loadFile(path)
					m.focus = focusEditor
					m.editor.Focus()
				}
			case "left", "h", "backspace":
				m.files.SetDir(filepath.Dir(m.files.dir))
			case "r":
				m.startRename()
			}
		}
	} else {
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && m.editor.AtLineEnd() {
			if prefix, clear, ok := listContinuation(m.editor.CurrentLine()); ok {
				if clear {
					m.editor.ClearLine()
				} else {
					m.editor.InsertString("\n" + prefix)
				}
				m.dirty = true
				m.lastEditAt = time.Now()
				return m, nil
			}
		}
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '\'' || km.Runes[0] == '"') {
			prev, hasPrev := m.editor.CharBeforeCursor()
			m.editor.InsertString(string(smartQuote(prev, hasPrev, km.Runes[0])))
			m.dirty = true
			m.lastEditAt = time.Now()
			return m, nil
		}
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	if m.screen == screenHome {
		return m.homeView()
	}

	if m.screen == screenOutline {
		return m.outlineView()
	}

	if m.screen == screenManuscript {
		return m.pagerView()
	}

	bodyH := m.height - 1 // status only; no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}

	// The writing pane shows either the live editor or the rendered preview.
	pane := m.editor.View()
	if m.previewing {
		name := filepath.Base(m.currentFile)
		if m.currentFile == "" {
			name = "untitled"
		}
		header := breadcrumbStyle.Render("▌ PREVIEW · " + name)
		pane = lipgloss.JoinVertical(lipgloss.Left, header, m.preview.View())
	}

	var body string
	if m.sidebarVisible {
		sideInner := lipgloss.JoinVertical(
			lipgloss.Left,
			func() string { row, _ := m.files.breadcrumbBar(sidebarWidth - 3); return breadcrumbStyle.Render(row) }(),
			m.files.View(),
		)
		side := sidebarStyle.
			Width(sidebarWidth - 1).
			Height(bodyH - 2).
			Render(sideInner)

		// Center the 80-col column inside the space left of the sidebar.
		editorArea := m.width - sidebarWidth
		ed := lipgloss.Place(editorArea, bodyH, lipgloss.Center, lipgloss.Top, pane)

		body = lipgloss.JoinHorizontal(lipgloss.Top, side, ed)
	} else {
		// Full-screen: center the same column in the whole window.
		body = lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Top, pane)
	}

	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// layout recomputes pane sizes whenever the window resizes or the sidebar
// toggles. The editor is always clamped to colWidth; the centering happens
// in View via lipgloss.Place.
func (m *model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyH := m.height - 1 // no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}

	cw := min(m.colWidth, m.width-2)
	if m.sidebarVisible {
		m.files.height = bodyH - 3 // sidebar content height (bodyH-2) minus the breadcrumb row
		m.files.width = sidebarWidth - 3
		cw = min(m.colWidth, m.width-sidebarWidth-2)
	}
	m.editor.SetWidth(cw)
	m.editor.SetHeight(bodyH)
	m.preview.Width = cw
	m.preview.Height = bodyH - 1 // reserve one row for the PREVIEW header
}

// updateOutline handles input on the outline screen: select, open, back, reorder.
func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		m.height = sz.Height
		m.outline.width = sz.Width
		m.outline.height = sz.Height - 1 // reserve the status bar row
		m.layout()
		return m, nil
	}
	if m.outlineCreating {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc":
				m.outlineCreating = false
				m.nameInput.Blur()
				m.status = "new section cancelled"
				return m, nil
			case "enter":
				m.confirmNewSection()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			// esc cancels with no rename, so there is nothing to refresh.
			case "esc":
				m.renaming = false
				m.nameInput.Blur()
				m.status = "rename cancelled"
				return m, nil
			case "enter":
				m.confirmRename()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.exportPrompt {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "m":
				m.exportPrompt = false
				m.runExport(StyleManuscript)
				return m, nil
			case "t":
				m.exportPrompt = false
				m.runExport(StyleTufte)
				return m, nil
			case "esc":
				m.exportPrompt = false
				m.status = "export cancelled"
				return m, nil
			}
		}
		return m, nil
	}

	if mouse, ok := msg.(tea.MouseMsg); ok {
		if m.outlineCreating || m.outline.confirm {
			return m, nil
		}
		if mouse.Button != tea.MouseButtonLeft || mouse.Action != tea.MouseActionPress {
			return m, nil
		}
		row := sidebarRow(mouse.Y, outlineHeaderHeight, len(m.outline.rows()))
		if row < 0 {
			return m, nil
		}
		m.outline.selected = row
		now := time.Now()
		if row == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if r, ok := m.outline.selectedRow(); ok {
				m.loadFile(filepath.Join(m.outline.dir, r.entry.name))
				m.screen = screenWriting
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{}
		} else {
			m.lastClickRow = row
			m.lastClickTime = now
		}
		return m, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// The apply/discard gate captures input while it is up.
	if m.outline.confirm {
		// Capture the pending-open target BEFORE apply/discard mutate (and reload)
		// the outline: load() resets selected/pendingOpen, so they can't be read after.
		wantOpen := m.outline.pendingOpen
		openName := ""
		if wantOpen {
			if row, ok := m.outline.selectedRow(); ok {
				openName = row.entry.name
			}
		}
		switch key.String() {
		case "y":
			m.outline.confirm = false
			moved, errStr := m.commitOutlineOrder()
			if errStr != "" {
				m.status = errStr
				return m, nil // commit failed: stay on the outline with the error visible
			}
			m.finishOutlineOpen(wantOpen, openName, moved)
			return m, nil
		case "n":
			m.outline.confirm = false
			m.outline.working = append([]fileEntry(nil), m.outline.disk...) // discard moves
			m.finishOutlineOpen(wantOpen, openName, nil)
			return m, nil
		case "esc":
			m.outline.confirm = false // keep editing the outline
			return m, nil
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.outline.moveSelection(-1)
	case "down", "j":
		m.outline.moveSelection(1)
	case "J", "shift+down":
		m.outline.moveSection(1)
	case "K", "shift+up":
		m.outline.moveSection(-1)
	case "enter":
		m.outline.pendingOpen = true
		return m.outlineLeave()
	case "n":
		m.outlineCreating = true
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.status = "new section title — enter to create, esc to cancel"
		return m, textinput.Blink
	case "r":
		m.startRenameOutline()
	case "m":
		m.enterManuscript()
	case "ctrl+e":
		m.exportPrompt = true
		m.status = "export: m manuscript · t tufte · esc cancel"
	case "esc":
		m.outline.pendingOpen = false
		return m.outlineLeave()
	}
	return m, nil
}

// outlineLeave handles an exit/open request: if a reorder is pending, raise the
// confirm gate; otherwise complete the action immediately.
func (m model) outlineLeave() (tea.Model, tea.Cmd) {
	if m.outline.dirty() {
		m.outline.confirm = true
		m.status = "apply reordering?  y apply · n discard · esc keep editing"
		return m, nil
	}
	m.leaveOutlinePending()
	return m, nil
}

// leaveOutlinePending completes a pending exit or open (set via pendingOpen).
func (m *model) leaveOutlinePending() {
	if m.outline.pendingOpen {
		if row, ok := m.outline.selectedRow(); ok {
			m.loadFile(filepath.Join(m.outline.dir, row.entry.name))
		}
	}
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}

// finishOutlineOpen returns to the editor, opening the captured section (mapped
// through any rename in moved) when the pending action was an Enter-open. Used by
// the confirm gate, where load() has already cleared the outline's pendingOpen.
func (m *model) finishOutlineOpen(wantOpen bool, openName string, moved map[string]string) {
	if wantOpen && openName != "" {
		path := filepath.Join(m.outline.dir, openName)
		if np, ok := moved[path]; ok {
			path = np
		}
		m.loadFile(path)
	}
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}

// outlineView renders the outline screen with the status bar.
func (m model) outlineView() string {
	body := m.outline.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// updateManuscript handles input on the pager: scroll, jump-to-edit, and exits.
func (m model) updateManuscript(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		m.height = sz.Height
		w := pagerWidth(m.colWidth, sz.Width)
		m.pager.width = w
		m.pager.height = sz.Height - 1 - pagerHeaderHeight
		if m.pager.height < 1 {
			m.pager.height = 1
		}
		m.pager.load(m.pager.dir, w) // re-wrap the line→source map to the new width
		m.layout()
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Button != tea.MouseButtonLeft || mouse.Action != tea.MouseActionPress {
			return m, nil
		}
		row := sidebarRow(mouse.Y, pagerHeaderHeight, m.pager.height)
		if row < 0 {
			return m, nil
		}
		line := m.pager.offset + row
		if line >= len(m.pager.lines) {
			return m, nil
		}
		m.pager.cursor = line
		now := time.Now()
		if line == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if file, src, ok := m.pager.jumpTarget(); ok {
				m.loadFile(filepath.Join(m.pager.dir, file))
				m.editor.MoveToLine(src)
				m.screen = screenWriting
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{}
		} else {
			m.lastClickRow = line
			m.lastClickTime = now
		}
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.pager.moveCursor(-1)
	case "down", "j":
		m.pager.moveCursor(1)
	case "pgup":
		m.pager.page(-1)
	case "pgdown":
		m.pager.page(1)
	case "enter":
		if file, src, ok := m.pager.jumpTarget(); ok {
			m.loadFile(filepath.Join(m.pager.dir, file))
			m.editor.MoveToLine(src)
			m.screen = screenWriting
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "o":
		m.enterOutline()
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
	}
	return m, nil
}

// pagerView renders the pager screen with the status bar.
func (m model) pagerView() string {
	body := m.pager.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// commitOutlineOrder applies a pending reorder: backup + renumber on disk, then
// follow the open file's rename and refresh the sidebar + outline. Returns an
// error string for the status line (empty on success/no-op).
func (m *model) commitOutlineOrder() (map[string]string, string) {
	moved, err := commitReorder(m.outline.dir, m.outline.working, backupStamp(time.Now()))
	if err != nil {
		return nil, "reorder failed: " + err.Error()
	}
	if newPath, ok := moved[m.currentFile]; ok {
		m.currentFile = newPath
	}
	m.files.SetDir(m.files.dir) // re-sort the sidebar to the new names
	m.outline.load(m.outline.dir, m.files.wc)
	return moved, ""
}

// enterManuscript builds the read-through pager for the current outline's
// manuscript and shows it. Reached from the outline's `m`.
func (m *model) enterManuscript() {
	w := pagerWidth(m.colWidth, m.width)
	m.pager.width = w
	m.pager.height = m.height - 1 - pagerHeaderHeight // status row + header
	if m.pager.height < 1 {
		m.pager.height = 1
	}
	m.pager.load(m.outline.dir, w)
	m.lastClickTime = time.Time{} // don't carry a stale double-click in from another screen
	m.screen = screenManuscript
	m.status = "manuscript · ↑↓ scroll · enter edit here · o outline · esc editor"
}

// pagerWidth is the pager's measure: the configured column width, never wider than
// the terminal (mirrors the editor's clamp), floored at 1.
func pagerWidth(colWidth, termWidth int) int {
	w := min(colWidth, termWidth-2)
	if w < 1 {
		w = 1
	}
	return w
}

// enterOutline opens the manuscript outline for the current pane dir. Caller
// must have verified isManuscript(m.files.entries).
func (m *model) enterOutline() {
	m.outline.width = m.width
	m.outline.height = m.height - 1 // status bar
	m.outline.load(m.files.dir, m.files.wc)
	m.screen = screenOutline
	m.previewing = false
	m.status = "outline · ↑↓ select · J/K reorder · enter open · n new · m read · esc back"
}

// hasConvertibleFiles reports whether the current pane dir has at least one
// document file (non-dir entry) that a convert could number.
func (m model) hasConvertibleFiles() bool {
	for _, e := range m.files.entries {
		if !e.isDir {
			return true
		}
	}
	return false
}

// convertToManuscript numbers the current folder's document files contiguously
// (backup first), follows the open file, and opens the outline.
func (m *model) convertToManuscript() {
	dir := m.files.dir
	var files []fileEntry
	for _, e := range m.files.entries {
		if !e.isDir {
			files = append(files, e)
		}
	}
	if len(files) == 0 {
		m.status = "nothing to convert"
		return
	}
	ops := planConvert(files, padWidth(len(files), 0))
	var paths []string
	for _, f := range files {
		paths = append(paths, filepath.Join(dir, f.name))
	}
	if err := backupFiles(dir, backupStamp(time.Now()), paths); err != nil {
		m.status = "convert failed: " + err.Error()
		return
	}
	if err := applyRenames(dir, ops); err != nil {
		m.status = "convert failed: " + err.Error()
		return
	}
	for _, op := range ops {
		if m.currentFile == filepath.Join(dir, op.from) {
			m.currentFile = filepath.Join(dir, op.to)
		}
	}
	m.files.SetDir(dir)
	m.enterOutline()
}

func (m *model) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		m.status = "couldn't open: " + filepath.Base(path)
		return
	}
	m.editor.SetValue(string(data))
	m.currentFile = path
	m.sessionBaseline = wordCount(string(data))
	m.previewing = false
	m.status = "opened " + filepath.Base(path)
	addRecent(recentPath(), path)
	m.dirty = false
}

// confirmCreate turns the typed name into a new file or folder in the current
// pane dir. A trailing "/" (or an explicit New-project) makes a folder; an
// explicit New-project then enters it, while the sidebar "name/" convention
// creates-and-stays. Files default to .md and open a blank buffer.
func (m *model) confirmCreate() {
	name := strings.TrimSpace(m.nameInput.Value())
	explicitFolder := m.creatingFolder
	m.creatingFile = false
	m.creatingFolder = false
	m.nameInput.Blur()
	if name == "" {
		m.status = "create cancelled (no name)"
		return
	}

	folder := explicitFolder || strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")

	if strings.Contains(name, "/") || name == "." || name == ".." {
		m.status = "name can't contain a path separator"
		return
	}

	if folder {
		dir := filepath.Join(m.files.dir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "couldn't create folder: " + err.Error()
			return
		}
		if explicitFolder {
			m.files.SetDir(dir) // New project → enter the folder
			m.status = "new project " + name
		} else {
			m.files.SetDir(m.files.dir) // name/ → refresh, stay
			m.files.selectName(name)
			m.status = "created folder " + name
		}
		m.focus = focusSidebar
		m.editor.Blur()
		return
	}

	if filepath.Ext(name) == "" {
		name += ".md"
	}
	m.currentFile = filepath.Join(m.files.dir, name)
	m.editor.SetValue("")
	m.sessionBaseline = 0
	m.dirty = false
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "new file: " + name + " — ctrl+s to save"
}

// beginRename opens the rename prompt for t, pre-filled with prefill.
func (m *model) beginRename(t renameTarget, prefill string) {
	m.renameTarget = t
	m.renaming = true
	m.creatingFile = false
	m.nameInput.SetValue(prefill)
	m.nameInput.CursorEnd()
	m.nameInput.Focus()
	m.editor.Blur()
	m.status = ""
}

// startRename begins renaming the selected sidebar entry (skips the ".." row).
func (m *model) startRename() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	_, numbered := sectionOrder(e.name)
	section := numbered && !e.isDir && isManuscript(m.files.entries)
	prefill := e.name
	if section {
		prefill = sectionTitle(e.name)
	}
	m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: section}, prefill)
}

// startRenameOutline begins renaming the selected outline row (section title or
// loose file).
func (m *model) startRenameOutline() {
	row, ok := m.outline.selectedRow()
	if !ok {
		return
	}
	prefill := row.entry.name
	if row.isSection {
		prefill = sectionTitle(row.entry.name)
	}
	m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, isDir: false, section: row.isSection}, prefill)
}

// confirmRename applies the pending rename: builds the new name by target kind,
// refuses a collision, renames on disk, follows the open file, and refreshes.
func (m *model) confirmRename() {
	m.renaming = false
	m.nameInput.Blur()
	typed := strings.TrimSpace(m.nameInput.Value())
	t := m.renameTarget
	if typed == "" {
		m.status = "rename cancelled (empty)"
		m.refreshAfterRename()
		return
	}

	var newName string
	if t.section {
		newName = sectionRetitle(t.name, typed)
	} else {
		if strings.Contains(typed, "/") || typed == "." || typed == ".." {
			m.status = "name can't contain a path separator"
			m.refreshAfterRename()
			return
		}
		if t.isDir {
			newName = typed
		} else {
			newName = looseRename(t.name, typed)
		}
	}
	if newName == t.name {
		m.status = "unchanged"
		m.refreshAfterRename()
		return
	}

	oldPath := filepath.Join(t.dir, t.name)
	newPath := filepath.Join(t.dir, newName)
	if _, err := os.Stat(newPath); err == nil {
		m.status = "a file named " + newName + " already exists"
		m.refreshAfterRename()
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		m.status = "rename failed: " + err.Error()
		m.refreshAfterRename()
		return
	}
	if m.currentFile == oldPath {
		m.currentFile = newPath
	}
	m.refreshAfterRename()
	m.status = "renamed to " + newName
}

// refreshAfterRename re-reads the sidebar (and the outline, if active) and
// restores focus to the pane the rename came from.
func (m *model) refreshAfterRename() {
	m.files.SetDir(m.files.dir)
	if m.screen == screenOutline {
		m.outline.load(m.outline.dir, m.files.wc)
		return
	}
	m.focus = focusSidebar
	m.editor.Blur()
}

// confirmNewSection creates a new section after the selected one and renumbers
// the rest. The new file opens in the editor.
func (m *model) confirmNewSection() {
	m.outlineCreating = false
	m.nameInput.Blur()
	title := strings.TrimSpace(m.nameInput.Value())
	if title == "" {
		m.status = "new section cancelled (no title)"
		return
	}
	insertIndex := m.outline.selected + 1
	if m.outline.selected >= len(m.outline.working) {
		insertIndex = len(m.outline.working) // a loose row is selected: append
	}
	padW := padWidth(len(m.outline.working)+1, existingPrefixWidth(m.outline.working))
	newName, moved, err := commitInsert(m.outline.dir, slugify(title), m.outline.working, insertIndex, padW, backupStamp(time.Now()))
	if err != nil {
		m.status = "new section failed: " + err.Error()
		return
	}
	if newPath, ok := moved[m.currentFile]; ok {
		m.currentFile = newPath
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(filepath.Join(m.outline.dir, newName))
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "new section: " + newName + " — ctrl+s to save"
}

// togglePreview flips between editing and a read-only glamour render of the
// current buffer. The render is a snapshot taken on entry — you can't edit a
// rendered document, so it can't drift while preview is up.
func (m *model) togglePreview() {
	if m.previewing {
		m.previewing = false
		if m.focus == focusEditor || !m.sidebarVisible {
			m.editor.Focus()
		}
		m.status = "editing"
		return
	}

	wrap := m.preview.Width
	if wrap <= 0 {
		wrap = m.colWidth
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.mdStyle), // theme detected once at startup
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		m.status = "preview unavailable: " + err.Error()
		return
	}
	out, err := r.Render(m.editor.Value())
	if err != nil {
		m.status = "preview failed: " + err.Error()
		return
	}

	m.preview.SetContent(out)
	m.preview.GotoTop()
	m.previewing = true
	m.editor.Blur()
	m.status = "preview (read-only) · ctrl+p to edit · ↑/↓ scroll"
}

// wordCount counts whitespace-separated words.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// commafy formats an integer with thousands separators: 1240 -> "1,240".
func commafy(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// signedComma formats a signed delta with an explicit sign: 142 -> "+142".
func signedComma(n int) string {
	if n < 0 {
		return "-" + commafy(-n)
	}
	return "+" + commafy(n)
}

// statsText is the live readout shown on the right of the status bar:
// total words in the buffer, plus net words added since this file was opened.
func (m model) statsText() string {
	words := wordCount(m.editor.Value())
	delta := words - m.sessionBaseline
	return fmt.Sprintf("%s words · %s session", commafy(words), signedComma(delta))
}

// statusBar composes the bottom line: the status message on the left, the live
// word-count readout right-aligned. While naming a new file the prompt owns the
// whole bar; on a terminal too narrow for both, the stats drop out rather than
// truncate. Width is the bar minus statusStyle's 1-col padding each side.
func (m model) statusBar() string {
	if m.creatingFile {
		folderMode := m.creatingFolder || strings.HasSuffix(m.nameInput.Value(), "/")
		label := "new file ▸ "
		if folderMode {
			label = "new folder ▸ "
		}
		bar := label + m.nameInput.View()
		if folderMode {
			return bar
		}
		hint := lipgloss.NewStyle().Foreground(subtle).Render("end with / for a folder")
		gap := (m.width - 2) - lipgloss.Width(bar) - lipgloss.Width(hint)
		if gap < 1 {
			return bar
		}
		return bar + strings.Repeat(" ", gap) + hint
	}
	if m.renaming {
		return "rename ▸ " + m.nameInput.View()
	}
	if m.convertPrompt {
		return "make this a manuscript? (y / n)"
	}
	if m.exportPrompt {
		return "export: m manuscript · t tufte · esc cancel"
	}
	mark := "✓"
	if m.dirty {
		mark = "●"
	}
	stats := mark + " " + m.statsText()
	return m.composeStatus(m.status, stats)
}

// composeStatus lays out the bottom bar: the status message at the far left and
// the stats centered over the editor pane (the area right of the sidebar).
func (m model) composeStatus(status, stats string) string {
	w := m.width - 2 // statusStyle adds one column of padding on each side
	sw := lipgloss.Width(stats)
	if w < 1 || sw >= w {
		return status
	}
	editorStart := 0
	if m.sidebarVisible && sidebarWidth < m.width {
		editorStart = sidebarWidth
	}
	// Center of the editor pane, in status content columns (content starts one
	// column in because of the style's left padding, hence the -1).
	center := editorStart + (m.width-editorStart)/2 - 1
	left := center - sw/2
	if left+sw > w {
		left = w - sw
	}
	statusW := lipgloss.Width(status)
	if left < statusW+1 {
		left = statusW + 1
	}
	if left+sw > w {
		return status // no room for both
	}
	return status + strings.Repeat(" ", left-statusW) + stats
}

func (m *model) save() {
	if m.currentFile == "" {
		m.status = "no file open — pick one from the sidebar first"
		return
	}
	if err := os.WriteFile(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
		m.status = "save failed: " + err.Error()
		return // dirty stays true → retried next tick
	}
	m.dirty = false
	addRecent(recentPath(), m.currentFile)
	m.status = "saved " + filepath.Base(m.currentFile)

	// Surface a newly-created file in the sidebar if we're browsing its folder.
	// Re-saving an already-listed file leaves the selection undisturbed.
	if base := filepath.Base(m.currentFile); filepath.Dir(m.currentFile) == m.files.dir && !m.files.has(base) {
		m.files.SetDir(m.files.dir)
		m.files.selectName(base)
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("okashi " + version)
			return
		case "--help", "-h", "help":
			fmt.Println(usage)
			return
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
