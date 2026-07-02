package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

const helpText = `ctrl+b   toggle sidebar
ctrl+y   inspector tabs
ctrl+n   new file (+  new, right-click / F2  rename)
r        rename file
M        move file/folder
del      delete file
d        duplicate file
ctrl+l   outline
ctrl+k   binder
ctrl+e   export
ctrl+p   preview
ctrl+t   typewriter
ctrl+d   focus dim
ctrl+s   save
ctrl+g   set goals
ctrl+r   spelling suggestions
ctrl+f   search (tab scope · ctrl+a all sources)
ctrl+o   home
⌥/⇧+drag select text (native) · ⌘C copy
esc      switch focus / back
ctrl+c   quit`

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
	screenSearch
	screenStructure
	screenMover
)

const (
	scopeProject = iota
	scopeDocument
	scopeAll
)

const searchLimit = 200

// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir             string // directory containing the item
	name            string // current base name
	isDir           bool
	section         bool // a numbered section -> title-only rename (legacy file rename)
	manifestChapter bool // a manifest chapter -> edit items[].title, filename birth-stable
}

const inspectorWidth = 34
const minEditorMeasure = 50

type model struct {
	width, height int

	files     filelist
	editor    textarea.Model
	nameInput textinput.Model
	preview   viewport.Model

	screen       screen
	homeItems    []homeItem
	homeRegion   homeRegion     // launch screen: which column/group
	homeIndex    int            // index within the region
	homeLastCol  homeRegion     // last column visited (for up-out-of-Actions)
	homeFiles    []homeFileItem // FILES column: the current dir's folders + documents
	homeFilesDir string         // the dir FILES currently shows (drill-down within the selection)

	structureDir        string          // the manuscript being restructured
	structureItems      []manifestItem  // staged chapter order/membership (committed on exit)
	structureSel        int             // cursor row
	structurePendingNew map[string]bool // new-blank files to create on commit
	structureDirty      bool            // any staged edit?
	structureAdding     bool            // the add-pick sub-mode is open
	structureAddSel     int             // cursor in the add-pick
	structureRenaming   bool            // the retitle field is open (reuses nameInput)
	structureConfirm    bool            // the commit confirm bar is open
	librarySelected     int             // index into projects+folders driving FILES
	sources             []source        // library sources; [0] is always the primary (writingDir())
	activeSource        int             // index into sources driving the home library
	pinned              []string        // pinned project/folder paths (persisted via pins.go)
	snippets            *snippetCache

	searchInput     textinput.Model
	searchScope     int // scopeProject | scopeDocument
	searchHits      []searchHit
	searchSel       int
	searchOffset    int
	searchHighlight string // transient: highlight this query on the editor's visible lines
	searchReturn    screen // where ctrl+f was invoked from (esc returns here)

	sidebarVisible bool
	inspector      inspectorModel
	focus          focus
	creatingFile   bool
	creatingFolder bool
	addingSource   bool // home screen: typing a folder path into nameInput to add a source
	previewing     bool
	previewTufte   bool
	previewAvail   int
	typewriter     bool
	dimEnabled     bool

	mdStyle           string // glamour theme, detected once at startup
	colWidth          int
	smartQuotes       bool
	sessionBaseline   int // word count when the current file was opened/created
	now               time.Time
	sessionStart      time.Time
	currentFile       string
	outlineReturnFile string // chapter to return to after editing outline.md (ctrl+l)
	status            string
	icons             iconSet
	outline           outlineModel

	pager pagerModel

	renaming       bool
	renamingInPane bool
	creatingInPane bool
	renameTarget   renameTarget

	deleting     bool
	deleteTarget string

	moverSource    string
	moverIsDir     bool
	moverFromDir   string
	moverDestDir   string
	moverEntries   []moverEntry
	moverSel       int
	moverConfirm   bool
	moverAsChapter bool
	moverReturn    screen
	moverError     string

	moverPhase      int          // moverPickSource | moverPickDest
	moverSrcDir     string       // left-pane browse dir (pick-source phase)
	moverSrcEntries []moverEntry // left-pane rows
	moverSrcSel     int

	exportPrompt bool

	goalsAll        map[string]projectGoals
	goalPromptField int // 0 off, 1 daily, 2 project, 3 session

	analysis analysisState

	grammarChecker   grammarChecker
	appleFindings    map[string][]grammarFinding
	checkingGrammar  bool
	autoRecheck      bool      // re-run the Apple pass after edits settle (opt-in)
	lastGrammarCheck time.Time // when the last Apple pass was dispatched

	lastClickRow  int
	lastClickTime time.Time

	dirty      bool
	lastEditAt time.Time

	suggesting               bool
	suggestions              []string
	suggestIndex             int
	suggestStart, suggestEnd int
	suggestWord              string

	showHelp bool
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
	ta.MaxHeight = 0 // unlimited — chapters routinely exceed the fork's default 99-line cap
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

	m := model{
		files:          fl,
		editor:         ta,
		nameInput:      ti,
		preview:        vp,
		mdStyle:        previewStyle(),
		colWidth:       resolveColumnWidth(),
		smartQuotes:    resolveSmartQuotes(),
		screen:         screenHome,
		homeItems:      buildHomeItems(loadRecents(recentPath()), writingDir(), loadPins(pinsPath())), // writingDir() == activeSourceRoot() at init (activeSource==0 is the primary)
		sources:        loadSources(sourcesPath()),
		pinned:         loadPins(pinsPath()),
		activeSource:   0,
		sidebarVisible: true,
		focus:          focusSidebar,
		typewriter:     true,
		dimEnabled:     true,
		status:         "",
		icons:          resolveIcons(),
		goalsAll:       loadGoals(goalsPath()),
		grammarChecker: newGrammarChecker(),
		appleFindings:  map[string][]grammarFinding{},
		snippets:       newSnippetCache(),
		searchInput:    newSearchInput(),
		now:            time.Now(),
		sessionStart:   time.Now(),
	}
	m.resetHomeSelection()
	return m
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

// readOutlineDoc returns the project's outline.md content ("" if none) for the
// inspector's read-only Outline tab.
func readOutlineDoc(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "outline.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

type autosaveTickMsg time.Time

// autosaveTick schedules the next autosave check. One loop runs for the app's
// lifetime, started in Init.
func autosaveTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return autosaveTickMsg(t) })
}

// analysisActionRowY is the inspector body row (content-relative, 0 = tab bar)
// for the "Check grammar" action button — rendered below the Passive row (10),
// after a blank row (11), at row 12. Does NOT shift the 5 checkbox rows.
// The backend name renders at 13 and the Auto-recheck toggle at 14.
const (
	analysisActionRowY = 12
	analysisAutoRowY   = 14
)

// grammarResultMsg is returned by checkGrammarCmd when the backend finishes.
type grammarResultMsg struct {
	file     string
	findings []grammarFinding
	err      error
}

// checkGrammarCmd runs c.Check asynchronously and returns the result as a
// grammarResultMsg. Called by the action row click handler.
func checkGrammarCmd(c grammarChecker, file, text string) tea.Cmd {
	return func() tea.Msg {
		f, err := c.Check(text)
		return grammarResultMsg{file, f, err}
	}
}

// autosaveDue reports whether the buffer should be flushed: there are unsaved
// edits to a real file and the writer has paused for at least 2s.
func (m model) autosaveDue(now time.Time) bool {
	return m.dirty && m.currentFile != "" && now.Sub(m.lastEditAt) >= 2*time.Second
}

// autoRecheckDue reports whether an automatic Apple grammar pass should fire: opt-in,
// grammar on, a backend present, not already checking, real content, the buffer has been
// idle ≥1.5s, and there's been an edit since the last check (so it fires once per edit
// burst, never on clean/unchanged text).
func (m model) autoRecheckDue(now time.Time) bool {
	return m.autoRecheck && m.analysis.grammar && m.grammarChecker != nil &&
		!m.checkingGrammar && m.currentFile != "" && m.editor.Value() != "" &&
		now.Sub(m.lastEditAt) >= 1500*time.Millisecond &&
		m.lastEditAt.After(m.lastGrammarCheck)
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

// applyDecorator sets the editor's Decorator from the current analysis toggles.
// Compose order: spell first, then grammar, then POS — so spell underline wins.
func (m *model) applyDecorator() {
	a := m.analysis
	posOn := a.adverb || a.adjective || a.passive
	grammarOn := a.grammar
	cursorLine := m.editor.CurrentLine()
	apple := m.appleFindings[m.currentFile] // Tier 2 (Apple) findings, gated by grammar
	build := func(line string, idx int) []textarea.Decoration {
		var d []textarea.Decoration
		if a.spell {
			d = append(d, spellDecorator(line)...)
		}
		if grammarOn {
			d = append(d, grammarDecorator(line, line == cursorLine)...)
			// Apple findings share the green grammar underline; keyed by line index,
			// clamped against stale offsets (cleared on edit, but guard anyway).
			nr := len([]rune(line))
			for _, f := range apple {
				if f.Line == idx && f.Start >= 0 && f.Start < f.End && f.End <= nr {
					d = append(d, textarea.Decoration{Start: f.Start, End: f.End, Style: grammarStyle})
				}
			}
		}
		if posOn {
			d = append(d, posDecorator(line, a.adverb, a.adjective, a.passive)...)
		}
		if m.searchHighlight != "" {
			d = append(d, searchDecorator(line, m.searchHighlight)...)
		}
		return d
	}
	if a.spell || grammarOn || posOn || m.searchHighlight != "" {
		m.editor.Decorator = build
	} else {
		m.editor.Decorator = nil
	}
}

// invalidateAppleFindings drops the cached Apple (Tier 2) findings for the current file;
// their rune offsets no longer match once the text changes. Re-run "Check grammar" to refresh.
func (m *model) invalidateAppleFindings() {
	// The on-edit hook: drop stale Apple findings AND the transient search highlight, then
	// refresh the decorator if either was active.
	had := len(m.appleFindings[m.currentFile]) > 0 || m.searchHighlight != ""
	delete(m.appleFindings, m.currentFile)
	m.searchHighlight = ""
	if had {
		m.applyDecorator()
	}
}

// maybeOpenAppleSuggestion opens the suggestion bar when an Apple (Tier 2) finding with
// replacements covers the cursor (set by a preceding click). Returns true if it opened one.
func (m *model) maybeOpenGrammarSuggestion() bool {
	f, ok := m.grammarFindingUnderCursor()
	if !ok || len(f.Replacements) == 0 {
		return false
	}
	runes := []rune(m.editor.CurrentLine())
	m.suggesting = true
	m.suggestions = f.Replacements
	m.suggestIndex = 0
	m.suggestWord = string(runes[f.Start:f.End])
	m.suggestStart, m.suggestEnd = f.Start, f.End
	m.status = f.Message
	return true
}

// syncGoal rolls the current project's daily baseline over to today on the first
// writing activity each day, persisting only on change.
func (m *model) syncGoal() {
	pg := m.goalsAll[m.files.dir].applyEnvDefaults()
	total := computeProjStats(m.files.dir, m.files.view, m.files.wc).words
	pg, changed := rolloverIfNeeded(pg, total, today())
	if m.goalsAll == nil {
		m.goalsAll = map[string]projectGoals{}
	}
	m.goalsAll[m.files.dir] = pg
	if changed {
		saveGoals(goalsPath(), m.goalsAll)
	}
}

// wordUnderCursor returns the token spanning the editor cursor on its line (O(line)).
func (m *model) wordUnderCursor() (word string, start, end int, ok bool) {
	line := m.editor.CurrentLine()
	col := m.editor.CursorColumn()
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		if col >= s[0] && col <= s[1] {
			return string(runes[s[0]:s[1]]), s[0], s[1], true
		}
	}
	return "", 0, 0, false
}

// cursorSpellHint returns the misspelled word under the cursor and its suggestions
// for passive display, or ok=false. Active only when spellcheck is on, on the
// writing screen, and no modal/prompt is up.
func (m *model) cursorSpellHint() (word string, suggestions []string, ok bool) {
	if !m.analysis.spell || m.screen != screenWriting {
		return "", nil, false
	}
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.previewing || m.exportPrompt || m.creatingFile {
		return "", nil, false
	}
	w, _, _, found := m.wordUnderCursor()
	if !found || spellOK(w) {
		return "", nil, false
	}
	sugg := append(spellSuggest(w, 4), dictItem) // + add-to-dictionary slot
	return w, sugg, true
}

// grammarFindingUnderCursor returns the grammar finding spanning the cursor: an Apple
// (Tier 2) finding if one covers it, else a live heuristic (Tier 1) finding on the line.
func (m *model) grammarFindingUnderCursor() (grammarFinding, bool) {
	if !m.analysis.grammar {
		return grammarFinding{}, false
	}
	line := m.editor.Line()
	col := m.editor.CursorColumn()
	cur := m.editor.CurrentLine()
	nr := len([]rune(cur))
	for _, f := range m.appleFindings[m.currentFile] {
		if f.Line == line && col >= f.Start && col <= f.End && f.Start < f.End && f.End <= nr {
			return f, true
		}
	}
	for _, f := range grammarFindings(cur, true) { // cursor line → terminal-punct suppressed
		if col >= f.Start && col <= f.End && f.Start < f.End && len(f.Replacements) > 0 {
			f.Line = line
			return f, true
		}
	}
	return grammarFinding{}, false
}

// cursorGrammarHint returns the wrong text, its fix(es), and the reason for the grammar
// finding under the cursor — the passive bottom-bar hint, mirroring the spell one.
func (m *model) cursorGrammarHint() (word string, suggestions []string, reason string, ok bool) {
	if !m.analysis.grammar || m.screen != screenWriting {
		return "", nil, "", false
	}
	if m.renaming || m.goalPromptField != 0 || m.suggesting || m.previewing || m.exportPrompt || m.creatingFile {
		return "", nil, "", false
	}
	f, found := m.grammarFindingUnderCursor()
	if !found || len(f.Replacements) == 0 {
		return "", nil, "", false
	}
	runes := []rune(m.editor.CurrentLine())
	return string(runes[f.Start:f.End]), f.Replacements, f.Message, true
}

// applyGrammarHint applies the i-th replacement for the grammar finding under the cursor.
func (m *model) applyGrammarHint(i int) {
	f, ok := m.grammarFindingUnderCursor()
	if !ok || len(f.Replacements) == 0 {
		return
	}
	runes := []rune(m.editor.CurrentLine())
	m.suggestions = f.Replacements
	m.suggestWord = string(runes[f.Start:f.End])
	m.suggestStart, m.suggestEnd = f.Start, f.End
	m.applySuggestion(i)
}

// matchCase applies orig's capitalization pattern to sugg.
func matchCase(orig, sugg string) string {
	if orig == "" || sugg == "" {
		return sugg
	}
	if isAllCaps(orig) {
		return strings.ToUpper(sugg)
	}
	or := []rune(orig)
	if unicode.IsUpper(or[0]) {
		sr := []rune(sugg)
		sr[0] = unicode.ToUpper(sr[0])
		return string(sr)
	}
	return sugg
}

// applySuggestion applies the i-th suggestion to the editor, replacing the word.
func (m *model) applySuggestion(i int) {
	if i < 0 || i >= len(m.suggestions) {
		m.suggesting = false
		return
	}
	if m.suggestions[i] == dictItem { // add the word to the personal dictionary instead
		m.suggesting = false
		if addToDictionary(m.suggestWord) {
			m.status = "added '" + m.suggestWord + "' to dictionary"
		} else {
			m.status = "'" + m.suggestWord + "' is already known"
		}
		m.applyDecorator() // the word is now known — clear its underline
		return
	}
	chosen := matchCase(m.suggestWord, m.suggestions[i])
	m.editor.ReplaceRange(m.suggestStart, m.suggestEnd, chosen)
	m.status = "'" + m.suggestWord + "' → '" + chosen + "'"
	m.suggesting = false
	m.invalidateAppleFindings() // the edit shifts offsets; refresh via Check grammar
}

// spellHintSuggestionAtX maps a content column on the spell-hint status row to a
// suggestion index. The hint is "✗ " + word + " → " + sugg joined by " · " + "  ·  ^R".
func spellHintSuggestionAtX(word string, sugg []string, localX int) (int, bool) {
	col := lipgloss.Width("✗ " + word + " → ")
	for i, s := range sugg {
		w := lipgloss.Width(s)
		if localX >= col && localX < col+w {
			return i, true
		}
		col += w + lipgloss.Width(" · ")
	}
	return 0, false
}

// openSpellMenuAndApply applies suggestion i for the misspelled word under the cursor.
func (m *model) openSpellMenuAndApply(i int, sugg []string) {
	w, s, e, ok := m.wordUnderCursor()
	if !ok {
		return
	}
	m.suggestions = sugg
	m.suggestWord = w
	m.suggestStart, m.suggestEnd = s, e
	m.applySuggestion(i)
}

func (m model) Init() tea.Cmd {
	return autosaveTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if t, ok := msg.(autosaveTickMsg); ok {
		now := time.Time(t)
		m.now = now
		if m.autosaveDue(now) {
			m.save()
		}
		if m.autoRecheckDue(now) {
			m.checkingGrammar = true
			m.lastGrammarCheck = now
			return m, tea.Batch(checkGrammarCmd(m.grammarChecker, m.currentFile, m.editor.Value()), autosaveTick())
		}
		return m, autosaveTick()
	}

	if msg, ok := msg.(grammarResultMsg); ok {
		m.appleFindings[msg.file] = msg.findings
		m.checkingGrammar = false
		if msg.err == nil {
			m.applyDecorator()
			n := len(msg.findings)
			if n == 1 {
				m.status = "1 grammar note"
			} else {
				m.status = fmt.Sprintf("%d grammar notes", n)
			}
		} else {
			m.status = "grammar check failed"
		}
		return m, nil
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

	if m.screen == screenSearch {
		return m.updateSearch(msg)
	}

	if m.screen == screenStructure {
		return m.updateStructure(msg)
	}

	if m.screen == screenMover {
		return m.updateMover(msg)
	}

	// While naming a new file, the prompt captures all input.
	if m.creatingFile {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.creatingFile = false
				m.creatingInPane = false
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
				m.renamingInPane = false
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

	if m.deleting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				m.confirmDelete()
				return m, nil
			default:
				m.deleting = false
				m.status = "delete cancelled"
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

	if m.goalPromptField != 0 {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.goalPromptField = 0
				m.nameInput.Blur()
				return m, nil
			case "enter":
				n, _ := strconv.Atoi(strings.TrimSpace(m.nameInput.Value()))
				if m.goalsAll == nil {
					m.goalsAll = map[string]projectGoals{}
				}
				pg := m.goalsAll[m.files.dir]
				if m.goalPromptField == 1 {
					if n >= 0 {
						pg.DailyGoal = n
					}
					m.goalsAll[m.files.dir] = pg
					m.goalPromptField = 2
					m.nameInput.SetValue(strconv.Itoa(pg.applyEnvDefaults().ProjectGoal))
					return m, nil
				}
				if m.goalPromptField == 2 {
					if n >= 0 {
						pg.ProjectGoal = n
					}
					m.goalsAll[m.files.dir] = pg
					m.goalPromptField = 3
					m.nameInput.SetValue(strconv.Itoa(pg.SessionGoalMin))
					return m, nil
				}
				if n >= 0 {
					pg.SessionGoalMin = n
				}
				m.goalsAll[m.files.dir] = pg
				saveGoals(goalsPath(), m.goalsAll)
				m.goalPromptField = 0
				m.nameInput.Blur()
				m.status = "goals saved"
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}

	if m.showHelp {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.showHelp = false
			return m, nil
		}
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyF1 {
		m.showHelp = true
		return m, nil
	}

	if m.suggesting {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.suggesting = false
				m.status = "suggestion cancelled"
				return m, nil
			case "left":
				if m.suggestIndex > 0 {
					m.suggestIndex--
				}
				return m, nil
			case "right":
				if m.suggestIndex < len(m.suggestions)-1 {
					m.suggestIndex++
				}
				return m, nil
			case "enter":
				m.applySuggestion(m.suggestIndex)
				return m, nil
			default:
				if len(key.String()) == 1 && key.String() >= "1" && key.String() <= "9" {
					i := int(key.String()[0] - '1')
					if i < len(m.suggestions) {
						m.applySuggestion(i)
					}
				}
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
		if m.previewing {
			m.renderPreview()
		}
		return m, nil

	case tea.MouseMsg:
		showSidebar, _, _ := m.effectivePanels()
		inSidebar := showSidebar && msg.X < sidebarWidth

		_, showInspector, _ := m.effectivePanels()
		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == 1 {
			localX := msg.X - (m.width - inspectorWidth) - 2 // panel-left + border + padding
			if localX >= 0 {
				if tb, ok := inspectorTabAtX(localX); ok {
					m.inspector.tab = tb
					return m, nil
				}
			}
		}

		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.inspector.tab == tabAnalysis {
			localX := msg.X - (m.width - inspectorWidth) - 2
			if localX >= 0 {
				if row, ok := inspectorAnalysisRowAtY(msg.Y - 1); ok {
					switch row {
					case 0:
						m.analysis.spell = !m.analysis.spell
					case 1:
						m.analysis.grammar = !m.analysis.grammar
					case 2:
						m.analysis.adverb = !m.analysis.adverb
					case 3:
						m.analysis.adjective = !m.analysis.adjective
					case 4:
						m.analysis.passive = !m.analysis.passive
					}
					m.applyDecorator()
					return m, nil
				}
			}
		}

		// Action row: "Check grammar" — only when grammar is on and a backend is available.
		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.inspector.tab == tabAnalysis {
			localX := msg.X - (m.width - inspectorWidth) - 2
			if m.analysis.grammar && m.grammarChecker != nil && !m.checkingGrammar && localX >= 0 && msg.Y-1 == analysisActionRowY {
				m.checkingGrammar = true
				m.lastGrammarCheck = time.Now()
				m.status = "checking grammar…"
				return m, checkGrammarCmd(m.grammarChecker, m.currentFile, m.editor.Value())
			}
			// Auto-recheck toggle row.
			if m.analysis.grammar && m.grammarChecker != nil && localX >= 0 && msg.Y-1 == analysisAutoRowY {
				m.autoRecheck = !m.autoRecheck
				return m, nil
			}
		}

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

		// Click on the status bar applies a spell suggestion if one is active.
		// The status renders inside the editor column, so its content starts at
		// editorStart + 1 (column left + the style's left padding).
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 {
			if w, sugg, ok := m.cursorSpellHint(); ok {
				showSidebar, _, _ := m.effectivePanels()
				editorStart := 0
				if showSidebar {
					editorStart = sidebarWidth
				}
				if i, hit := spellHintSuggestionAtX(w, sugg, msg.X-editorStart-1); hit {
					m.openSpellMenuAndApply(i, sugg)
					return m, nil
				}
			}
			// Same row for a grammar finding's fix (the reason after it isn't clickable).
			if w, sugg, _, ok := m.cursorGrammarHint(); ok {
				showSidebar, _, _ := m.effectivePanels()
				editorStart := 0
				if showSidebar {
					editorStart = sidebarWidth
				}
				if i, hit := spellHintSuggestionAtX(w, sugg, msg.X-editorStart-1); hit {
					m.applyGrammarHint(i)
					return m, nil
				}
			}
		}

		// Click in the editor area positions the cursor (enables click-to-suggest).
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			showSidebar, showInspector, editorArea := m.effectivePanels()
			editorStart := 0
			if showSidebar {
				editorStart = sidebarWidth
			}
			inEditor := msg.X >= editorStart && (!showInspector || msg.X < m.width-inspectorWidth) && msg.Y < m.height-2
			if inEditor && !m.previewing {
				cw := min(m.colWidth, editorArea-2)
				textLeft := editorStart + (editorArea-cw)/2
				m.editor.ClickTo(msg.Y, msg.X-textLeft)
				m.focus = focusEditor
				m.editor.Focus()
				if !m.maybeOpenGrammarSuggestion() { // click a flagged span → suggestion bar;
					m.suggesting = false // otherwise dismiss any open bar
				}
				return m, nil
			}
		}

		// Right-click in the sidebar starts an in-place rename.
		if inSidebar && msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
			if row := sidebarRow(msg.Y, 1, m.files.height); row >= 0 && row < len(m.files.entries) {
				m.focus = focusSidebar
				m.editor.Blur()
				m.files.selectRow(row)
				m.startRename()
				return m, textinput.Blink
			}
		}

		// Click the '+' in the sidebar title bar starts an in-pane create.
		// The '+' is rendered at column sidebarWidth-2 (right side of top border, before ╮).
		if inSidebar && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress &&
			msg.Y == 0 && msg.X == sidebarWidth-2 {
			m.startInPaneCreate()
			return m, textinput.Blink
		}

		// Click selection / open is file-pane only.
		if !inSidebar || msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// Framed sidebar: top border occupies row 0; the file list starts at row 1.
		row := sidebarRow(msg.Y, 1, m.files.height)
		if row < 0 || row >= len(m.files.entries) { // ignore clicks on blank rows below the list
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
		m.syncGoal()
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+o":
			m.previewing = false
			m.screen = screenHome
			m.homeItems = buildHomeItems(loadRecents(recentPath()), m.activeSourceRoot(), m.pinned)
			m.resetHomeSelection()
			return m, nil
		case "ctrl+f":
			m.previewing = false // leave preview if active
			m.searchReturn = m.screen
			m.screen = screenSearch
			m.searchScope = scopeProject
			m.searchInput.SetValue(m.wordUnderCursorOrEmpty())
			m.searchInput.CursorEnd()
			m.searchInput.Focus()
			m.recomputeSearch()
			return m, textinput.Blink
		case "ctrl+n":
			m.startInPaneCreate()
			return m, textinput.Blink
		case "f2":
			m.focus = focusSidebar
			m.editor.Blur()
			m.startRename()
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
			// ctrl+l always toggles the planning outline.md (ctrl+k is the binder).
			outlinePath := filepath.Join(m.files.dir, "outline.md")
			if m.currentFile == outlinePath {
				m.save()
				if m.outlineReturnFile != "" {
					m.loadFile(m.outlineReturnFile)
				}
			} else {
				m.save()
				m.outlineReturnFile = m.currentFile
				if _, err := os.Stat(outlinePath); err != nil {
					if werr := atomicWrite(outlinePath, []byte("- \n"), 0o644); werr != nil {
						m.status = "couldn't create outline: " + werr.Error()
						return m, nil
					}
					m.files.SetDir(m.files.dir) // surface outline.md in the sidebar
				}
				m.loadFile(outlinePath)
			}
			return m, nil
		case "ctrl+k":
			if m.files.view.ordered() {
				m.enterOutline()
			} else {
				m.status = "not a manuscript"
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
		case "ctrl+y":
			m.inspector.cycle()
			m.layout()
			return m, nil
		case "ctrl+g":
			pg := m.goalsAll[m.files.dir].applyEnvDefaults()
			m.goalPromptField = 1
			m.nameInput.SetValue(strconv.Itoa(pg.DailyGoal))
			m.nameInput.Focus()
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
		case "ctrl+r":
			if !m.renaming && m.goalPromptField == 0 && !m.suggesting && !m.previewing {
				w, s, e, ok := m.wordUnderCursor()
				if !ok {
					m.status = "no word under cursor"
					return m, nil
				}
				if spellOK(w) {
					m.status = "'" + w + "' looks correct"
					return m, nil
				}
				sugg := append(spellSuggest(w, 7), dictItem) // + add-to-dictionary slot
				m.suggesting = true
				m.suggestions = sugg
				m.suggestIndex = 0
				m.suggestWord = w
				m.suggestStart, m.suggestEnd = s, e
				return m, nil
			}
		}
	}

	// Route everything else to whichever pane has focus. Preview is a modal
	// read-only state, so it wins over pane focus — keys/wheel scroll it and
	// nothing reaches the editor or file list underneath.
	var cmd tea.Cmd
	if m.previewing {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "t" {
			m.previewTufte = !m.previewTufte // toggle Default ⇄ Tufte
			m.renderPreview()
			return m, tea.Batch(cmds...)
		}
		m.preview, cmd = m.preview.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.focus == focusSidebar && m.sidebarVisible {
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.Type == tea.KeyDelete {
				m.startDelete()
			} else {
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
				case "d":
					m.duplicateSelected()
				case "M":
					m.enterMover()
				}
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
				m.invalidateAppleFindings() // buffer mutated — offsets/line indices shift
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
			m.invalidateAppleFindings()
			return m, nil
		}
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings() // offsets are stale once the text changes
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

	if m.screen == screenSearch {
		return m.searchView()
	}

	if m.screen == screenStructure {
		return m.structureView()
	}

	if m.screen == screenMover {
		return m.moverView()
	}

	bodyH := m.height - 1 // status only; no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}

	if m.showHelp {
		card := framedPanel("Keys", helpText, 36, min(bodyH, lipgloss.Height(helpText)+2), "")
		body := lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card)
		return lipgloss.JoinVertical(lipgloss.Left, body, statusStyle.Width(m.width).Render("F1/esc close"))
	}

	// Refresh the decorator so the grammar "don't nag the line you're typing" rule
	// tracks the LIVE cursor line. applyDecorator captures the cursor line by value
	// and only re-runs on toggle/load, so without this the suppression would freeze
	// to whatever line was current when Grammar was switched on. Cheap (a closure
	// rebuild); only when grammar is active.
	if m.analysis.grammar {
		m.applyDecorator()
	}

	// The writing pane shows either the live editor or the rendered preview.
	pane := m.editor.View()
	if m.previewing {
		name := filepath.Base(m.currentFile)
		if m.currentFile == "" {
			name = "untitled"
		}
		style := "Default"
		if m.previewTufte {
			style = "Tufte"
		}
		header := breadcrumbStyle.Render("▌ PREVIEW · "+name) + lipgloss.NewStyle().Foreground(subtle).Render("  · "+style+" (t)")
		pane = lipgloss.JoinVertical(lipgloss.Left, header, m.preview.View())
	}

	showSidebar, showInspector, editorArea := m.effectivePanels()

	cols := []string{}
	if showSidebar {
		title := m.files.paneLabel()
		editRow, editField := -1, ""
		if m.renaming && m.renamingInPane {
			editRow, editField = m.files.selected, m.nameInput.View()
		} else if m.creatingFile && m.creatingInPane {
			editRow, editField = createRowSentinel, m.nameInput.View()
		}
		cols = append(cols, framedPanel(title, m.files.View(editRow, editField), sidebarWidth, m.height, "+"))
	}
	// Editor column: the editor pane, a blank line break, then the status bar —
	// all at the editor width, so the side panels render truly full height and the
	// status/spelling hint stay within the editor column.
	editorH := bodyH - 1 // leave one row for the blank line above the status
	if editorH < 1 {
		editorH = 1
	}
	editorPane := lipgloss.Place(editorArea, editorH, lipgloss.Center, lipgloss.Top, pane)
	statusRow := statusStyle.Width(editorArea).Render(m.statusBar())
	editorCol := lipgloss.JoinVertical(lipgloss.Left, editorPane, strings.Repeat(" ", editorArea), statusRow)
	cols = append(cols, editorCol)
	if showInspector {
		doc := computeDocStats(m.editor.Value())
		proj := computeProjStats(m.files.dir, m.files.view, m.files.wc)
		pg := m.goalsAll[m.files.dir].applyEnvDefaults()
		gs := goalStats{today: todayWords(pg, proj.words), dailyGoal: pg.DailyGoal, project: proj.words, projectGoal: pg.ProjectGoal,
			sessionSecs: int(m.now.Sub(m.sessionStart).Seconds()), sessionGoalMin: pg.SessionGoalMin}
		m.inspector.grammarBackend = ""
		if m.grammarChecker != nil {
			m.inspector.grammarBackend = m.grammarChecker.Name()
		}
		m.inspector.grammarChecking = m.checkingGrammar
		m.inspector.grammarAutoRecheck = m.autoRecheck
		insInner := m.inspector.View(inspectorInnerWidth(), doc, proj, readOutlineDoc(m.files.dir), gs, m.analysis)
		title := inspectorTabLabels()[m.inspector.tab]
		cols = append(cols, framedPanel(title, insInner, inspectorWidth, m.height, ""))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

// effectivePanels resolves which side panels are shown this render and the
// width left for the editor. When the inspector is open and showing both panels
// would squeeze the editor below minEditorMeasure, the sidebar is suppressed for
// this render (m.sidebarVisible is not mutated).
func (m model) effectivePanels() (showSidebar, showInspector bool, editorArea int) {
	showSidebar = m.sidebarVisible
	showInspector = m.inspector.visible
	if showInspector && showSidebar && m.width-sidebarWidth-inspectorWidth < minEditorMeasure {
		showSidebar = false
	}
	editorArea = m.width
	if showSidebar {
		editorArea -= sidebarWidth
	}
	if showInspector {
		editorArea -= inspectorWidth
	}
	return
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

	showSidebar, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	if showSidebar {
		m.files.height = m.height - 2 // full-height panel content (m.height minus top+bottom border)
		m.files.width = sidebarWidth - 4
	}
	editorH := bodyH - 1 // editor column reserves a blank line + the status row
	if editorH < 1 {
		editorH = 1
	}
	m.previewAvail = editorArea - 2
	if m.previewAvail < 0 {
		m.previewAvail = 0
	}
	m.editor.SetWidth(cw)
	m.editor.SetHeight(editorH)
	m.preview.Width = cw
	m.preview.Height = editorH - 1 // reserve one row for the PREVIEW header
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
	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			// esc cancels with no rename, so there is nothing to refresh.
			case "esc":
				m.renaming = false
				m.renamingInPane = false
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
		}
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
		return m, nil
	case "r":
		m.startRenameOutline()
	case "m":
		m.enterManuscript()
	case "s":
		m.enterStructure()
		return m, nil
	case "ctrl+e":
		m.exportPrompt = true
		m.status = "export: m manuscript · t tufte · esc cancel"
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
		return m, nil
	}
	return m, nil
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
	m.status = "manuscript · ↑↓ scroll · enter edit here · o binder · esc editor"
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
	m.status = "binder · ↑↓ select · enter open · s structure · r rename · m read · ctrl+e export · esc back"
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
	m.applyDecorator()
}

// confirmCreate turns the typed name into a new file or folder in the current
// pane dir. A trailing "/" (or an explicit New-project) makes a folder; an
// explicit New-project then enters it, while the sidebar "name/" convention
// creates-and-stays. Files default to .md and open a blank buffer.
func (m *model) confirmCreate() {
	name := strings.TrimSpace(m.nameInput.Value())
	explicitFolder := m.creatingFolder
	m.creatingFile = false
	m.creatingInPane = false
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

	if name == manifestName {
		m.status = "manifest.json is read-only (managed externally)"
		return
	}

	if folder {
		dir := filepath.Join(m.files.dir, name)
		if explicitFolder {
			// New Project → a real manuscript (folder + manifest + first chapter you land in).
			first, err := createManuscript(dir, name, "Untitled")
			if err != nil {
				m.status = "couldn't create project: " + err.Error()
				return
			}
			m.files.SetDir(dir)
			m.loadFile(filepath.Join(dir, first))
			m.focus = focusEditor
			m.editor.Focus()
			m.status = "new project " + name + " — start writing"
			return
		}
		// "name/" convention → a plain category folder; refresh and stay.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "couldn't create folder: " + err.Error()
			return
		}
		m.files.SetDir(m.files.dir)
		m.files.selectName(name)
		m.status = "created folder " + name
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

// startInPaneCreate begins an in-place new-document prompt in the sidebar.
func (m *model) startInPaneCreate() {
	m.previewing = false
	m.sidebarVisible = true // the field renders in the file pane — make sure it shows
	m.creatingFile = true
	m.creatingInPane = true
	m.creatingFolder = false
	m.nameInput.SetValue("")
	m.nameInput.Width = m.files.width
	m.nameInput.Focus()
	m.editor.Blur()
	m.focus = focusSidebar
	m.status = ""
}

// startRename begins renaming the selected sidebar entry (skips the ".." row).
func (m *model) startRename() {
	if len(m.files.entries) == 0 {
		return
	}
	m.sidebarVisible = true // the field renders in the file pane — make sure it shows
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	v := m.files.view
	if v.source == sourceManifest && v.warning != "" {
		m.status = "manifest unreadable — structure is read-only (external manifest)"
		return
	}
	if isChapterOf(v, e.name) {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.renamingInPane = true
			m.nameInput.Width = m.files.width
			m.beginRename(renameTarget{dir: m.files.dir, name: e.name, manifestChapter: true},
				m.files.chapterTitle(e.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
	m.renamingInPane = true
	m.nameInput.Width = m.files.width
	m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir}, e.name)
}

// startDelete begins a delete confirmation for the selected sidebar entry.
// Silently skips ".." and manifest.json; blocks manifest chapters.
func (m *model) startDelete() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	if e.name == manifestName {
		m.status = "manifest.json is read-only (managed externally)"
		return
	}
	v := m.files.view
	if isChapterOf(v, e.name) && v.source == sourceManifest {
		m.status = "chapter files are read-only (external manifest)"
		return
	}
	m.deleting = true
	m.deleteTarget = e.name
	m.status = "delete '" + e.name + "'? [y]es · esc cancel"
}

// confirmDelete removes the deleteTarget file or folder and refreshes the sidebar.
func (m *model) confirmDelete() {
	idx := m.files.selected // keep the position so the selection lands on the adjacent row
	path := filepath.Join(m.files.dir, m.deleteTarget)
	info, err := os.Lstat(path)
	if err == nil {
		if info.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
	}
	m.deleting = false
	m.deleteTarget = ""
	if err != nil {
		m.status = "couldn't delete: " + err.Error()
		return
	}
	m.files.SetDir(m.files.dir) // re-reads entries and resets selection to 0
	if n := len(m.files.entries); n > 0 {
		if idx >= n {
			idx = n - 1
		}
		m.files.selectRow(idx) // the row the deleted item occupied (now its neighbor)
	}
	m.status = "deleted"
}

// duplicateSelected copies the selected file to a free "name copy.ext" (then
// "name copy 2.ext", …) and selects the new file. Dirs and ".." are skipped.
func (m *model) duplicateSelected() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." || e.isDir {
		m.status = "duplicate: files only"
		return
	}
	ext := filepath.Ext(e.name)
	stem := strings.TrimSuffix(e.name, ext)
	target := copyFreeName(m.files.dir, stem, ext)
	data, err := os.ReadFile(filepath.Join(m.files.dir, e.name))
	if err != nil {
		m.status = "duplicate failed: " + err.Error()
		return
	}
	if err := atomicWrite(filepath.Join(m.files.dir, target), data, 0o644); err != nil {
		m.status = "duplicate failed: " + err.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.files.selectName(target)
	m.status = "duplicated → " + target
}

// copyFreeName returns "stem copy.ext", then "stem copy 2.ext", … that doesn't exist in dir.
func copyFreeName(dir, stem, ext string) string {
	name := stem + " copy" + ext
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	for i := 2; ; i++ {
		name = fmt.Sprintf("%s copy %d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// startRenameOutline begins renaming the selected outline row (section title or
// loose file). Mirrors startRename: manifest chapters are refused; legacy chapters
// get a prefix-preserving retitle; loose files get a plain rename.
func (m *model) startRenameOutline() {
	row, ok := m.outline.selectedRow()
	if !ok {
		return
	}
	// Resolve at the top so the refuse-mode guard covers both section and loose rows.
	// A refuse-mode folder has source==sourceManifest with a non-empty warning;
	// its files appear as loose (no chapters), so the isSection branch never fires —
	// the guard must precede it.
	v := resolveManuscript(m.outline.dir, readEntries(m.outline.dir))
	if v.source == sourceManifest && v.warning != "" {
		m.status = "manifest unreadable — structure is read-only (external manifest)"
		return
	}
	if row.isSection {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, manifestChapter: true},
				m.outline.chapterTitle(row.entry.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, isDir: false, section: true},
			sectionTitle(row.entry.name))
		return
	}
	m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, isDir: false, section: false}, row.entry.name)
}

// confirmRename applies the pending rename: builds the new name by target kind,
// refuses a collision, renames on disk, follows the open file, and refreshes.
func (m *model) confirmRename() {
	m.renaming = false
	m.renamingInPane = false
	m.nameInput.Blur()
	typed := strings.TrimSpace(m.nameInput.Value())
	t := m.renameTarget
	if typed == "" {
		m.status = "rename cancelled (empty)"
		m.refreshAfterRename()
		return
	}

	if t.manifestChapter {
		if err := renameChapterTitle(t.dir, t.name, typed); err != nil {
			m.status = "retitle failed: " + err.Error()
		} else {
			m.status = "retitled to " + typed
		}
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
	if newName == manifestName {
		m.status = "manifest.json is read-only (managed externally)"
		m.refreshAfterRename()
		return
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

	m.renderPreview()
	m.preview.GotoTop()
	m.previewing = true
	m.editor.Blur()
	m.status = "preview (read-only) · ctrl+p edit · t style · ↑/↓ scroll"
}

// NOTE: the sidenote helpers below (sidenoteGeometry, sidenotePlan, and in preview.go
// footnotesToSidenotes / layoutSidenotes) are DORMANT — the Tufte preview now renders footnotes
// as bottom endnotes. They are retained (with tests) to seed the planned margin/revision-notes
// feature, which will reuse the right-margin gutter for author annotations. Not currently wired.
const (
	sidenoteMinGutter = 18
	sidenoteMaxGutter = 30
)

// sidenoteGeometry reports the gutter width for a preview pane of `avail` columns holding a body
// of `measure` columns plus the 3-col " ┆ " gap, and whether a margin (>= sidenoteMinGutter) fits.
func sidenoteGeometry(avail, measure int) (gutter int, ok bool) {
	gutter = avail - measure - 3
	if gutter < sidenoteMinGutter {
		return 0, false
	}
	if gutter > sidenoteMaxGutter {
		gutter = sidenoteMaxGutter
	}
	return gutter, true
}

// sidenotePlan decides whether the Tufte preview should render margin sidenotes: the body measure
// stays the writing measure (colWidth), the gutter uses the pane's spare width, and it engages only
// when the pane is wide enough AND the doc has referenced footnotes. body/notes come from
// footnotesToSidenotes (body has superscript refs, no endnote section).
func sidenotePlan(avail, colWidth int, buffer string) (measure, gutter int, body string, notes []string, ok bool) {
	measure = colWidth
	if avail > 0 && avail < measure {
		measure = avail // tiny panes: never exceed the available width
	}
	g, gok := sidenoteGeometry(avail, measure)
	if !gok {
		return 0, 0, "", nil, false
	}
	b, ns := footnotesToSidenotes(buffer)
	if len(ns) == 0 {
		return 0, 0, "", nil, false
	}
	return measure, g, b, ns, true
}

// renderPreview rebuilds the preview viewport from the buffer: footnotes folded to endnotes
// (glamour can't render them), styled Default (the detected theme) or Tufte.
func (m *model) renderPreview() {
	wrap := min(m.colWidth, m.previewAvail)
	if wrap <= 0 {
		wrap = m.colWidth // pre-layout (previewAvail not set yet)
	}
	m.preview.Width = wrap
	styleOpt := glamour.WithStandardStyle(m.mdStyle)
	if m.previewTufte {
		styleOpt = glamour.WithStyles(tufteGlamourStyle(m.mdStyle == "dark"))
	}
	r, err := glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(wrap))
	if err != nil {
		m.status = "preview unavailable: " + err.Error()
		return
	}
	out, err := r.Render(footnotesToEndnotes(m.editor.Value()))
	if err != nil {
		m.status = "preview failed: " + err.Error()
		return
	}
	m.preview.SetContent(out)
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

// fmtDuration renders a duration as M:SS under an hour, H:MM:SS at or above. Negative clamps to 0.
func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// statsText is the live readout shown on the right of the status bar:
// total words in the buffer, plus net words added since this file was opened.
func (m model) statsText() string {
	words := wordCount(m.editor.Value())
	delta := words - m.sessionBaseline
	return fmt.Sprintf("%s words · %s session · ⏱ %s", commafy(words), signedComma(delta), fmtDuration(m.now.Sub(m.sessionStart)))
}

// statusBar composes the bottom line: the status message on the left, the live
// word-count readout right-aligned. While naming a new file the prompt owns the
// whole bar; on a terminal too narrow for both, the stats drop out rather than
// truncate. Width is the bar minus statusStyle's 1-col padding each side.
func (m model) statusBar() string {
	// An in-pane rename/create renders in the file row — but only if the sidebar
	// is actually drawn this frame; otherwise fall back to the bottom-bar prompt
	// so the field is never invisible.
	showSidebar, _, _ := m.effectivePanels()
	if m.creatingFile && (!m.creatingInPane || !showSidebar) {
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
	if m.suggesting {
		parts := make([]string, len(m.suggestions))
		for i, s := range m.suggestions {
			if i == m.suggestIndex {
				parts[i] = selectedStyle.Render(s)
			} else {
				parts[i] = s
			}
		}
		return "suggest ▸ " + strings.Join(parts, " · ")
	}
	if w, sugg, ok := m.cursorSpellHint(); ok {
		return "✗ " + w + " → " + strings.Join(sugg, " · ") + "  ·  ^R"
	}
	if w, sugg, reason, ok := m.cursorGrammarHint(); ok {
		_, _, editorArea := m.effectivePanels()
		hint := "✗ " + w + " → " + strings.Join(sugg, " · ")
		if reason != "" {
			hint += " · " + reason
		}
		return ansi.Truncate(hint, max(10, editorArea-2), "…") // keep the fix visible; trim the reason
	}
	if m.renaming && (!m.renamingInPane || !showSidebar) {
		return "rename ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 1 {
		return "daily goal ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 2 {
		return "project goal ▸ " + m.nameInput.View()
	}
	if m.goalPromptField == 3 {
		return "session minutes ▸ " + m.nameInput.View()
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

// composeStatus lays the stats at the editor text's left edge and the status
// (last save/open) right-aligned to the text's right edge, within the editor
// text column. Stats win if both don't fit.
func (m model) composeStatus(status, stats string) string {
	_, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	totalW := editorArea - 2 // status renders at editorArea width; statusStyle pads one col each side
	sw := lipgloss.Width(stats)
	if cw < sw+1 || totalW < sw {
		return stats // too narrow for the two-element layout
	}
	left := (editorArea-cw)/2 - 1 // content col of the text's left edge (within the editor column)
	if left < 0 {
		left = 0
	}
	if left+sw > totalW {
		left = totalW - sw
	}
	// status right-aligned to the text right edge (content col left+cw), truncated to fit.
	avail := cw - sw - 1
	st := status
	if lipgloss.Width(st) > avail {
		if avail <= 0 {
			return strings.Repeat(" ", left) + stats
		}
		st = ansi.Truncate(st, avail, "…")
	}
	stW := lipgloss.Width(st)
	statusStart := left + cw - stW
	used := left + sw
	if statusStart < used+1 {
		statusStart = used + 1
	}
	if statusStart+stW > totalW {
		statusStart = totalW - stW
	}
	if statusStart < used {
		return strings.Repeat(" ", left) + stats
	}
	return strings.Repeat(" ", left) + stats + strings.Repeat(" ", statusStart-used) + st
}

func (m *model) save() {
	if m.currentFile == "" {
		m.status = "no file open — pick one from the sidebar first"
		return
	}
	if err := atomicWrite(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
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
