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
)

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
		status:         "ctrl+b sidebar · esc switch · ctrl+n new · ctrl+p preview · ctrl+t typewriter · ctrl+d dim · ctrl+s save · ctrl+c quit",
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
			if isManuscript(m.files.entries) {
				m.enterOutline()
			} else {
				m.status = "not a manuscript folder (no numbered sections)"
			}
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

// updateOutline handles input on the outline screen: select, open, back.
// (Reorder and new-section are layered on in later tasks.)
func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.outline.moveSelection(-1)
	case "down", "j":
		m.outline.moveSelection(1)
	case "enter":
		if row, ok := m.outline.selectedRow(); ok {
			m.loadFile(filepath.Join(m.outline.dir, row.entry.name))
			m.screen = screenWriting
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "m":
		m.status = "manuscript view — Plan C"
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
	}
	return m, nil
}

// outlineView renders the outline screen with the status bar.
func (m model) outlineView() string {
	body := m.outline.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// enterOutline opens the manuscript outline for the current pane dir. Caller
// must have verified isManuscript(m.files.entries).
func (m *model) enterOutline() {
	m.outline.width = m.width
	m.outline.height = m.height - 1 // status bar
	m.outline.load(m.files.dir, m.files.wc)
	m.screen = screenOutline
	m.previewing = false
	m.status = "outline · ↑↓ select · J/K reorder · enter open · n new · esc back"
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
