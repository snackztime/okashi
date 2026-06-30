package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestPreviewToggle(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	// Give it a size so layout() sizes the viewport.
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)

	// A document tall enough that the preview must scroll.
	var sb strings.Builder
	sb.WriteString("# Hello\n\nSome **bold** prose, then a long list:\n\n")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "- item %d\n", i)
	}
	m.editor.SetValue(sb.String())

	// ctrl+p should enter preview and render via glamour. Note: focus is still
	// the launch default (focusSidebar) here — exactly the case that regressed.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if !m.previewing {
		t.Fatal("expected previewing=true after ctrl+p")
	}
	if m.focus != focusSidebar {
		t.Fatal("test precondition: expected focus to still be on the sidebar")
	}
	view := m.View()
	if strings.TrimSpace(view) == "" {
		t.Fatal("preview view is empty")
	}
	// glamour styles "Hello" — the literal "# " markdown marker should be gone.
	if strings.Contains(view, "# Hello") {
		t.Fatal("expected rendered markdown, found raw source")
	}

	// ↓ must scroll the preview, NOT the filepicker underneath it. With sidebar
	// focus this only works because previewing is routed first.
	if m.preview.YOffset != 0 {
		t.Fatalf("expected preview to start at top, got YOffset=%d", m.preview.YOffset)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	if m.preview.YOffset == 0 {
		t.Fatal("expected ↓ to scroll the preview, but YOffset stayed 0 (routed to sidebar?)")
	}

	// ctrl+p again returns to editing.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if m.previewing {
		t.Fatal("expected previewing=false after second ctrl+p")
	}
}

func TestTypewriterToggle(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	if !m.typewriter || !m.editor.Typewriter {
		t.Fatal("typewriter should default on")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = nm.(model)
	if m.typewriter || m.editor.Typewriter {
		t.Fatal("ctrl+t should turn typewriter off")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = nm.(model)
	if !m.typewriter || !m.editor.Typewriter {
		t.Fatal("ctrl+t should turn typewriter back on")
	}
}

func TestFilelistOpensFileFromSidebar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("hello world words"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.root = "" // allow testing with arbitrary temp directories
	m.files.SetDir(dir)

	// Select the file (".." then "draft.md") and press enter.
	m.files.selected = 1
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)

	if m.currentFile != path {
		t.Fatalf("currentFile = %q, want %q", m.currentFile, path)
	}
	if m.focus != focusEditor {
		t.Fatal("opening a file should move focus to the editor")
	}
}

func TestSidebarRowMapping(t *testing.T) {
	// banner 4 rows tall, list 10 rows. Y=4 -> row 0; Y=13 -> row 9; Y=3 -> -1.
	if got := sidebarRow(4, 4, 10); got != 0 {
		t.Fatalf("sidebarRow(4,4,10) = %d, want 0", got)
	}
	if got := sidebarRow(13, 4, 10); got != 9 {
		t.Fatalf("sidebarRow(13,4,10) = %d, want 9", got)
	}
	if got := sidebarRow(3, 4, 10); got != -1 {
		t.Fatalf("sidebarRow above list = %d, want -1", got)
	}
	if got := sidebarRow(14, 4, 10); got != -1 {
		t.Fatalf("sidebarRow below list = %d, want -1", got)
	}
}

func TestMouseWheelScrollsFileList(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.entries = []fileEntry{{name: "a"}, {name: "b"}, {name: "c"}}
	m.files.selected = 0

	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.files.selected != 1 {
		t.Fatalf("wheel down: selected = %d, want 1", m.files.selected)
	}
	if m.focus != focusSidebar {
		t.Fatal("wheel over the pane should focus it")
	}
}

func TestSaveRefreshesSidebarForNewFile(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.root = "" // allow testing with arbitrary temp directories
	m.files.SetDir(dir)

	m.currentFile = filepath.Join(dir, "fresh.md")
	m.editor.SetValue("hello")
	if m.files.has("fresh.md") {
		t.Fatal("precondition: fresh.md should not be listed before save")
	}

	m.save()

	if !m.files.has("fresh.md") {
		t.Fatal("save should refresh the sidebar to show the new file")
	}
	if m.files.entries[m.files.selected].name != "fresh.md" {
		t.Fatalf("saved new file should be selected, got %q", m.files.entries[m.files.selected].name)
	}
}

func TestWheelOverEditorMovesCaret(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.editor.SetValue("l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7")
	for i := 0; i < 20; i++ {
		m.editor.CursorUp() // caret to top
	}
	// Wheel down over the editor area (X past the sidebar).
	nm, _ = m.Update(tea.MouseMsg{X: 50, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.editor.Line() != 3 {
		t.Fatalf("wheel down over editor: caret line = %d, want 3", m.editor.Line())
	}
}

func TestWheelOverPreviewScrolls(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	// Use a small height so the viewport is shorter than the rendered content
	// (no banner in writing mode, so bodyH = height-1 = 9; glamour renders ~16 lines).
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	m = nm.(model)
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	m.editor.SetValue(sb.String())
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP}) // enter preview
	m = nm.(model)
	if m.preview.YOffset != 0 {
		t.Fatalf("preview should start at top, got %d", m.preview.YOffset)
	}
	nm, _ = m.Update(tea.MouseMsg{X: 50, Y: 10, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.preview.YOffset == 0 {
		t.Fatal("wheel over preview should scroll it")
	}
}

func TestMouseClickSelectsAndDoubleClickOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("hi there words"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.root = "" // allow testing with arbitrary temp directories
	m.files.SetDir(dir)

	// entries: ["..", "draft.md"] → draft.md is visible row 1.
	// Framed sidebar: top border at row 0; file list starts at row 1: ".."=Y1, draft.md=Y2.
	click := tea.MouseMsg{X: 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}

	// Single click selects but does NOT open.
	nm, _ = m.Update(click)
	m = nm.(model)
	if got := m.files.entries[m.files.selected].name; got != "draft.md" {
		t.Fatalf("single click should select draft.md, got %q", got)
	}
	if m.currentFile != "" {
		t.Fatalf("single click should not open a file, currentFile = %q", m.currentFile)
	}

	// Second click on the same row (instant, within 400ms) opens it.
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.currentFile != path {
		t.Fatalf("double click should open the file, currentFile = %q", m.currentFile)
	}
	if m.focus != focusEditor {
		t.Fatal("opening via double click should focus the editor")
	}
}

func TestAutosaveDue(t *testing.T) {
	m := initialModel()
	now := time.Now()
	m.currentFile = "/tmp/x.md"

	m.dirty = true
	m.lastEditAt = now.Add(-3 * time.Second)
	if !m.autosaveDue(now) {
		t.Fatal("should be due: dirty, has file, idle 3s")
	}
	m.lastEditAt = now.Add(-500 * time.Millisecond)
	if m.autosaveDue(now) {
		t.Fatal("not due: only idle 0.5s")
	}
	m.dirty, m.lastEditAt = false, now.Add(-3*time.Second)
	if m.autosaveDue(now) {
		t.Fatal("not due: not dirty")
	}
	m.dirty, m.currentFile = true, ""
	if m.autosaveDue(now) {
		t.Fatal("not due: no current file")
	}
}

func TestAutosaveTickWritesWhenDue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auto.md")
	if err := os.WriteFile(path, []byte("start"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.currentFile = path
	m.editor.SetValue("updated text")
	m.dirty = true
	m.lastEditAt = time.Now().Add(-3 * time.Second)

	nm, _ = m.Update(autosaveTickMsg(time.Now()))
	m = nm.(model)

	data, _ := os.ReadFile(path)
	if string(data) != "updated text" {
		t.Fatalf("autosave did not write; file = %q", string(data))
	}
	if m.dirty {
		t.Fatal("dirty should clear after a successful autosave")
	}
}

func TestHomeOpenProjectAndCtrlOReturns(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	// Create a project path within the workspace root.
	projPath := filepath.Join(m.files.root, "testproject")
	m.homeItems = []homeItem{{kind: homeProject, label: "testproject", path: projPath}}
	m.homeSelected = 0

	// Enter on a project → writing mode, sidebar displays the project.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatal("opening a project should switch to writing")
	}
	if m.files.dir != projPath {
		t.Fatalf("sidebar should display the project, got %q", m.files.dir)
	}
	// But the workspace root must remain unchanged.
	if m.files.root == projPath {
		t.Fatal("workspace root must not change when opening a project")
	}

	// ctrl+o returns to home.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = nm.(model)
	if m.screen != screenHome {
		t.Fatal("ctrl+o should return to the home screen")
	}
}

func TestWritingViewHasNoBanner(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	// bannerArt's first line is part of the logo; it must not appear while writing.
	first := strings.SplitN(bannerArt, "\n", 2)[0]
	if strings.TrimSpace(first) != "" && strings.Contains(m.View(), first) {
		t.Fatal("writing view should not render the banner")
	}
}

func TestAutosaveTickSurvivesHomeScreen(t *testing.T) {
	m := initialModel() // starts on the home screen
	_, cmd := m.Update(autosaveTickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("autosave tick must reschedule even on the home screen")
	}
}

func TestLoadFileClearsDirty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.dirty = true
	m.loadFile(p)
	if m.dirty {
		t.Fatal("opening a file should clear dirty (fresh buffer is saved)")
	}
}

func TestNewFileDoesNotAutosaveEmpty(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	m.files.root = "" // allow testing with arbitrary temp directories
	m.files.SetDir(dir)
	// Simulate: prior edits left dirty set, idle long ago.
	m.dirty = true
	m.lastEditAt = time.Now().Add(-10 * time.Second)
	// Create a new file via the prompt flow.
	m.nameInput.SetValue("fresh.md")
	m.confirmCreate()
	if m.dirty {
		t.Fatal("a brand-new unedited buffer must not be dirty")
	}
	if m.autosaveDue(time.Now()) {
		t.Fatal("a new empty file must not be autosave-due before any edit")
	}
}

func TestResolveColumnWidth(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	if resolveColumnWidth() != 72 {
		t.Fatalf("default should be 72, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "90")
	if resolveColumnWidth() != 90 {
		t.Fatalf("env 90 should win, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "5") // out of range
	if resolveColumnWidth() != 72 {
		t.Fatalf("out-of-range should fall back to 72, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "abc")
	if resolveColumnWidth() != 72 {
		t.Fatalf("garbage should fall back to 72, got %d", resolveColumnWidth())
	}
}

func TestLaunchStartsOnHomeAndNavigates(t *testing.T) {
	m := initialModel()
	if m.screen != screenHome {
		t.Fatal("app should start on the home screen")
	}
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	// Force a known home list: two items.
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeOpenOther, label: "Open another folder…"},
	}
	m.homeSelected = 0

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	if m.homeSelected != 1 {
		t.Fatalf("down should move selection to 1, got %d", m.homeSelected)
	}
	view := m.View()
	if !strings.Contains(view, "novel") {
		t.Fatalf("home view should list the project; view=%q", view)
	}
}

func TestEscTogglesFocusAndTabIndents(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("hi")
	m.editor.SetCursor(2)

	// Tab indents the editor.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	if m.editor.Value() != "  hi" {
		t.Fatalf("tab should indent, got %q", m.editor.Value())
	}
	if !m.dirty {
		t.Fatal("indent should mark dirty")
	}

	// Shift+Tab outdents.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = nm.(model)
	if m.editor.Value() != "hi" {
		t.Fatalf("shift+tab should outdent, got %q", m.editor.Value())
	}

	// esc moves focus to the sidebar.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.focus != focusSidebar {
		t.Fatal("esc should toggle focus to the sidebar")
	}
}
func TestSmartQuoteHelper(t *testing.T) {
	cases := []struct {
		prev    rune
		hasPrev bool
		q       rune
		want    rune
	}{
		{0, false, '\'', rune(0x2018)},  // start of line → opening '
		{' ', true, '"', rune(0x201C)},  // after space → opening "
		{'n', true, '\'', rune(0x2019)}, // contraction don't → closing '
		{'d', true, '"', rune(0x201D)},  // after letter → closing "
		{'(', true, '\'', rune(0x2018)}, // after ( → opening
	}
	for _, c := range cases {
		if got := smartQuote(c.prev, c.hasPrev, c.q); got != c.want {
			t.Fatalf("smartQuote(%q,%v,%q) = %q, want %q", c.prev, c.hasPrev, c.q, got, c.want)
		}
	}
}

func TestResolveSmartQuotes(t *testing.T) {
	t.Setenv("OKASHI_SMARTQUOTES", "")
	if !resolveSmartQuotes() {
		t.Fatal("default should be on")
	}
	t.Setenv("OKASHI_SMARTQUOTES", "off")
	if resolveSmartQuotes() {
		t.Fatal("off should disable")
	}
}

func TestEditorSmartQuoteInsert(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true
	m.editor.SetValue("")
	m.editor.SetCursor(0)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'"'}})
	m = nm.(model)
	expected := string([]rune{rune(0x201C)}) // left double quote
	if m.editor.Value() != expected {
		t.Fatalf("typing \" at start should insert a left double curly quote, got %q", m.editor.Value())
	}
}

func TestListContinuation(t *testing.T) {
	cases := []struct {
		line   string
		prefix string
		clear  bool
		ok     bool
	}{
		{"- item", "- ", false, true},
		{"  - nested", "  - ", false, true},
		{"3. third", "4. ", false, true},
		{"- ", "", true, true},  // empty bullet → end list
		{"1. ", "", true, true}, // empty number → end list
		{"plain text", "", false, false},
	}
	for _, c := range cases {
		p, cl, ok := listContinuation(c.line)
		if p != c.prefix || cl != c.clear || ok != c.ok {
			t.Fatalf("listContinuation(%q) = (%q,%v,%v), want (%q,%v,%v)",
				c.line, p, cl, ok, c.prefix, c.clear, c.ok)
		}
	}
}

func TestEnterContinuesList(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("- one")
	m.editor.SetCursor(5) // end of line

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.editor.Value() != "- one\n- " {
		t.Fatalf("Enter should continue the list, got %q", m.editor.Value())
	}
}

func TestTabDoesNotEditDuringPreview(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("hello")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP}) // enter preview from the editor
	m = nm.(model)
	if !m.previewing {
		t.Fatal("expected previewing after ctrl+p")
	}
	before := m.editor.Value()
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	if m.editor.Value() != before {
		t.Fatalf("Tab during preview must not edit the buffer; got %q", m.editor.Value())
	}
}

func TestEnterEndsEmptyListItem(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("- ")
	m.editor.SetCursor(2)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.editor.Value() != "" {
		t.Fatalf("Enter on an empty bullet should clear it with no extra newline, got %q", m.editor.Value())
	}
}

func TestSidebarClickRowAccountsForTopBorder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("hi words"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.root = dir
	m.files.SetDir(dir) // entries: ["draft.md"] (no ".." at root)

	// Framed sidebar: top border at row 0; row 0 of the list is at screen Y=1.
	click := tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.files.entries[m.files.selected].name != "draft.md" {
		t.Fatalf("click at Y=1 should select the first list row, got %q", m.files.entries[m.files.selected].name)
	}
}

func TestConfirmCreateFolderTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	m.files.root = ""
	m.files.SetDir(dir)
	m.creatingFile = true
	m.nameInput.SetValue("notes/")
	m.confirmCreate()

	if _, err := os.Stat(filepath.Join(dir, "notes")); err != nil {
		t.Fatalf("trailing-slash name should create a folder: %v", err)
	}
	if m.files.dir != dir {
		t.Fatalf("sidebar name/ should stay in the current dir, got %q", m.files.dir)
	}
	if !m.files.has("notes") {
		t.Fatal("the new folder should be listed")
	}
}

func TestConfirmCreateExplicitFolderEnters(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	m.files.root = ""
	m.files.SetDir(dir)
	m.creatingFile = true
	m.creatingFolder = true
	m.nameInput.SetValue("Book One")
	m.confirmCreate()

	if m.files.dir != filepath.Join(dir, "Book One") {
		t.Fatalf("explicit New project should enter the folder, got %q", m.files.dir)
	}
}

func TestConfirmCreateFile(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	m.files.root = ""
	m.files.SetDir(dir)
	m.creatingFile = true
	m.nameInput.SetValue("draft")
	m.confirmCreate()

	if m.currentFile != filepath.Join(dir, "draft.md") {
		t.Fatalf("a bare name should make a .md file, got %q", m.currentFile)
	}
}

func TestHubNewProjectOpensFolderPrompt(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	m.homeItems = []homeItem{{kind: homeNewProject, label: "New project"}}
	m.homeSelected = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatal("New project should switch to writing")
	}
	if !m.creatingFile || !m.creatingFolder {
		t.Fatal("New project should open the create prompt in folder mode")
	}
}

func TestHubNewDocumentOpensFilePrompt(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	m.homeItems = []homeItem{{kind: homeNewDocument, label: "New document"}}
	m.homeSelected = 0
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if !m.creatingFile || m.creatingFolder {
		t.Fatal("New document should open the create prompt in file mode")
	}
	if m.files.dir != writingDir() {
		t.Fatalf("New document should root at the workspace, got %q", m.files.dir)
	}
}

func TestOpenProjectKeepsWorkspaceRoot(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	want := m.files.root // writingDir(), the workspace
	if want == "" {
		t.Skip("no workspace root resolved")
	}
	proj := filepath.Join(want, "someproject")
	m.homeItems = []homeItem{{kind: homeProject, label: "someproject", path: proj}}
	m.homeSelected = 0
	m.openHomeSelection()
	if m.files.root != want {
		t.Fatalf("opening a project must keep the workspace root %q, got %q", want, m.files.root)
	}
}

func TestHomeMouseWheelMovesSelection(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(model)
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeNewDocument, label: "New document"},
	}
	m.homeSelected = 0

	nm, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.homeSelected != 1 {
		t.Fatalf("wheel down should move selection to 1, got %d", m.homeSelected)
	}
	nm, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.homeSelected != 0 {
		t.Fatalf("wheel up should move selection back to 0, got %d", m.homeSelected)
	}
}

func TestHomeMouseClickSelectsItem(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(model)
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeNewDocument, label: "New document"},
	}
	m.homeSelected = 0

	// Compute the Y coordinate for item 1 using homeRows, as the handler does.
	_, itemRow, h := homeRows(m.homeItems, m.homeSelected, m.icons)
	off := (m.height - h) / 2

	// Single click on item 1 should select it without activating.
	click := tea.MouseMsg{X: 10, Y: off + itemRow[1], Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.homeSelected != 1 {
		t.Fatalf("click should select item 1, got %d", m.homeSelected)
	}
	if m.screen != screenHome {
		t.Fatal("single click should not leave the home screen")
	}
}

func TestHomeMouseDoubleClickActivates(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(model)
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: t.TempDir()},
		{kind: homeNewDocument, label: "New document"},
	}
	m.homeSelected = 0

	// Compute the Y coordinate for item 0 using homeRows.
	_, itemRow, h := homeRows(m.homeItems, m.homeSelected, m.icons)
	off := (m.height - h) / 2

	click := tea.MouseMsg{X: 10, Y: off + itemRow[0], Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}

	// First click selects but doesn't activate.
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.homeSelected != 0 {
		t.Fatalf("first click should select item 0, got %d", m.homeSelected)
	}
	if m.screen != screenHome {
		t.Fatal("first click should stay on home")
	}

	// Second click immediately (within 400ms) activates the item.
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("double click on project should switch to writing, screen=%d", m.screen)
	}
}

func TestConfirmCreateRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	m.files.root = dir
	m.files.SetDir(dir)
	m.creatingFile = true
	m.nameInput.SetValue("../evil/")
	m.confirmCreate()
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "evil")); err == nil {
		t.Fatal("a traversal name must not create anything outside the pane dir")
	}
	m.creatingFile = true
	m.nameInput.SetValue("../escape.md")
	m.confirmCreate()
	if m.currentFile == filepath.Join(filepath.Dir(dir), "escape.md") {
		t.Fatal("a traversal file name must be rejected, not set as currentFile")
	}
}

func TestCtrlNResetsFolderMode(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.creatingFolder = true // leaked from a prior New project
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = nm.(model)
	if !m.creatingFile || m.creatingFolder {
		t.Fatal("ctrl+n should open the prompt in file mode, resetting folder mode")
	}
}

func TestSidebarFrameFitsAndAligns(t *testing.T) {
	// framedPanel: 2 borders + 2 padding cols = 4; inner width = sidebarWidth-4.
	inner := strings.Repeat("x", sidebarWidth-4)
	out := framedPanel("Files", inner, sidebarWidth, 4)
	if w := lipgloss.Width(out); w != sidebarWidth {
		t.Fatalf("sidebar framedPanel total width = %d, want %d", w, sidebarWidth)
	}
	if h := lipgloss.Height(out); h != 4 {
		t.Fatalf("sidebar framedPanel total height = %d, want 4", h)
	}
}

func TestLayoutFilePaneWidth(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	if m.files.width != sidebarWidth-4 {
		t.Fatalf("files.width = %d, want %d", m.files.width, sidebarWidth-4)
	}
}

func TestDimFollowsTypewriterAndToggle(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	// default: typewriter on, dimEnabled on → editor.Dim on
	if !m.dimEnabled || !m.editor.Dim {
		t.Fatal("dim should default on (typewriter on, dimEnabled on)")
	}
	// ctrl+d turns dimming off but keeps typewriter
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = nm.(model)
	if m.dimEnabled || m.editor.Dim {
		t.Fatal("ctrl+d should turn dimming off")
	}
	if !m.typewriter {
		t.Fatal("ctrl+d must not affect typewriter")
	}
	// ctrl+t off → dim off regardless of dimEnabled
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD}) // dim back on
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}) // typewriter off
	m = nm.(model)
	if m.editor.Dim {
		t.Fatal("editor.Dim must be off when typewriter is off")
	}
}

func TestStatsAtEditorTextLeftEdge(t *testing.T) {
	m := initialModel()
	m.width = 100
	m.sidebarVisible = true
	m.inspector.visible = false
	stats := "✓ 1,240 words · +142 session"
	bar := m.composeStatus("", stats)
	leading := len(bar) - len(strings.TrimLeft(bar, " "))
	// Stats should start at the editor text's left edge:
	// editorArea = 100-sidebarWidth; cw = min(colWidth, editorArea-2)
	_, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	// composeStatus is now relative to the editor column (the View places that
	// column at the sidebar offset), so the leading spaces are column-relative.
	want := (editorArea-cw)/2 - 1
	if want < 0 {
		want = 0
	}
	if leading < want-1 || leading > want+1 {
		t.Fatalf("stats start col = %d, want ~%d (at the editor text left edge within the column)", leading, want)
	}
}

func TestStatusBarLeftRight(t *testing.T) {
	m := initialModel()
	m.width = 120
	m.height = 30
	m.colWidth = 72
	m.sidebarVisible = false
	m.inspector.visible = false
	stats := "✓ 1,240 words · +142 session"
	out := ansi.Strip(m.composeStatus("saved draft.md", stats))
	si := strings.Index(out, "1,240")
	di := strings.Index(out, "saved")
	if si < 0 || di < 0 {
		t.Fatalf("both stats and status should render: %q", out)
	}
	if si >= di {
		t.Fatalf("stats must be left of the status: stats@%d status@%d in %q", si, di, out)
	}
	// A very long status truncates rather than pushing the stats off / overrunning.
	long := strings.Repeat("verylongmessage ", 20)
	out2 := ansi.Strip(m.composeStatus(long, stats))
	if !strings.Contains(out2, "1,240") {
		t.Fatalf("stats must survive a long status: %q", out2)
	}
	if lipgloss.Width(out2) > m.width {
		t.Fatalf("status row overflows width: %d > %d", lipgloss.Width(out2), m.width)
	}
}

func TestEffectivePanels(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	m.sidebarVisible = true
	m.inspector.visible = true

	// Wide: all three columns; editor area is the remainder.
	m.width = 140
	ss, si, area := m.effectivePanels()
	if !ss || !si {
		t.Fatalf("wide: showSidebar=%v showInspector=%v, want both true", ss, si)
	}
	if area != 140-sidebarWidth-inspectorWidth {
		t.Fatalf("wide editorArea = %d, want %d", area, 140-sidebarWidth-inspectorWidth)
	}

	// Narrow: inspector open squeezes the editor → sidebar suppressed, editor keeps >= min.
	m.width = 90 // 90-34-32 = 24 < 50
	ss, si, area = m.effectivePanels()
	if ss {
		t.Fatal("narrow: sidebar should be suppressed while the inspector is open")
	}
	if !si {
		t.Fatal("narrow: inspector should stay visible")
	}
	if area < minEditorMeasure {
		t.Fatalf("narrow editorArea = %d, want >= %d", area, minEditorMeasure)
	}
}

func TestInspectorToggleAndRender(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = nm.(model)
	m.editor.SetValue("Some prose here.\n")

	// Inspector hidden by default — check against a string only the inspector emits.
	if strings.Contains(ansi.Strip(m.View()), "DOCUMENT") {
		t.Fatal("inspector should be hidden by default")
	}
	// 1st ctrl+y → Words tab visible.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(model)
	if !m.inspector.visible {
		t.Fatal("1st ctrl+y should make the inspector visible")
	}
	if !strings.Contains(ansi.Strip(m.View()), "DOCUMENT") {
		t.Fatal("writing View should contain the inspector (Words tab) after 1st ctrl+y")
	}
	// 2nd ctrl+y → Outline tab (still visible, NOT closed).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(model)
	if !m.inspector.visible {
		t.Fatal("2nd ctrl+y should keep the inspector visible (Outline tab)")
	}
	if m.inspector.tab != tabOutline {
		t.Fatalf("2nd ctrl+y: expected tabOutline, got %v", m.inspector.tab)
	}
	// 3rd ctrl+y → Goals tab (still visible).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(model)
	if !m.inspector.visible {
		t.Fatal("3rd ctrl+y should keep the inspector visible (Goals tab)")
	}
	if m.inspector.tab != tabGoals {
		t.Fatalf("3rd ctrl+y: expected tabGoals, got %v", m.inspector.tab)
	}
	// 4th ctrl+y → Analysis tab (still visible).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(model)
	if !m.inspector.visible {
		t.Fatal("4th ctrl+y should keep the inspector visible (Analysis tab)")
	}
	if m.inspector.tab != tabAnalysis {
		t.Fatalf("4th ctrl+y: expected tabAnalysis, got %v", m.inspector.tab)
	}
	// 5th ctrl+y → inspector closes (past the last tab).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = nm.(model)
	if m.inspector.visible || strings.Contains(ansi.Strip(m.View()), "DOCUMENT") {
		t.Fatal("5th ctrl+y should close the inspector (cycle past last tab)")
	}
}

func TestPreviewHeaderShown(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.currentFile = "/x/chapter-01.md"
	m.editor.SetValue("# Title\n\nbody text")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	view := m.View()
	if !strings.Contains(view, "PREVIEW") {
		t.Fatal("preview should show a PREVIEW header")
	}
	if !strings.Contains(view, "chapter-01.md") {
		t.Fatal("preview header should show the filename")
	}
}

func TestOutlineDocToggle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("chapter body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))

	// ctrl+l opens outline.md (created on disk).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if filepath.Base(m.currentFile) != "outline.md" {
		t.Fatalf("ctrl+l should open outline.md, got %q", m.currentFile)
	}
	if _, err := os.Stat(filepath.Join(dir, "outline.md")); err != nil {
		t.Fatal("outline.md should be created on disk")
	}
	// ctrl+l again returns to the chapter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if filepath.Base(m.currentFile) != "01-a.md" {
		t.Fatalf("second ctrl+l should return to the chapter, got %q", m.currentFile)
	}
}

func TestBinderOnCtrlK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("ctrl+k should open the binder, got screen %v", m.screen)
	}
}

func TestCtrlGSetsGoals(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate goals.json
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = nm.(model)

	// ctrl+g → daily prompt; type 400, enter → project prompt; type 90000, enter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	m = nm.(model)
	if m.goalPromptField != 1 {
		t.Fatalf("ctrl+g should start the daily prompt, field=%d", m.goalPromptField)
	}
	m.nameInput.SetValue("400")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.goalPromptField != 2 {
		t.Fatalf("after daily, should prompt project, field=%d", m.goalPromptField)
	}
	m.nameInput.SetValue("90000")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.goalPromptField != 0 {
		t.Fatalf("after project, prompt should close, field=%d", m.goalPromptField)
	}
	if m.goalsAll[dir].DailyGoal != 400 || m.goalsAll[dir].ProjectGoal != 90000 {
		t.Fatalf("goals not saved: %+v", m.goalsAll[dir])
	}
}

func TestSpellcheckToggleViaAnalysisClick(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabAnalysis
	m.layout()
	if m.analysis.spell {
		t.Fatal("spellcheck should default off")
	}
	// Click the Spellcheck checkbox row in the inspector body.
	// analysisRowY is content-relative; add 1 for the framed panel's top border.
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: analysisRowY(0) + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.spell {
		t.Fatal("clicking the Spellcheck row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("enabling spellcheck should set the editor Decorator")
	}
}

func TestPOSToggleViaAnalysisClick(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabAnalysis
	m.layout()
	// analysisRowY is content-relative; add 1 for the framed panel's top border.
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: analysisRowY(1) + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.adverb {
		t.Fatal("clicking the Adverb row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("a POS category on should set the editor Decorator")
	}
}

func TestInspectorTabClick(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabWords
	m.layout()
	// Click the "Outline" chip: inspector content starts at width-inspectorWidth+2;
	// Outline chip begins at localX 7 ("Words " = 6). Click at that column, row 1
	// (row 0 is the framed panel's top border, row 1 is the tab bar).
	x := m.width - inspectorWidth + 2 + 8 // mid-"Outline"
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.inspector.tab != tabOutline {
		t.Fatalf("click on Outline chip → tab=%v, want Outline", m.inspector.tab)
	}
}

func TestSuggestMenuReplacesWord(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // cursor inside "teh"
	// ctrl+r opens the menu.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(model)
	if !m.suggesting {
		t.Fatalf("ctrl+r on a misspelled word should open the menu; status=%q", m.status)
	}
	if len(m.suggestions) == 0 || m.suggestions[0] != "the" {
		t.Fatalf("expected suggestions led by \"the\", got %v", m.suggestions)
	}
	// enter applies the top suggestion.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.suggesting {
		t.Fatal("enter should close the menu")
	}
	if got := m.editor.Value(); got != "the cat" {
		t.Fatalf("after applying, value = %q, want \"the cat\"", got)
	}
}

func TestSuggestMenuCorrectWordNoMenu(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("the cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = nm.(model)
	if m.suggesting {
		t.Fatal("ctrl+r on a correct word should NOT open the menu")
	}
}

func TestCursorSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // inside "teh"

	// Spell OFF → no hint.
	m.analysis.spell = false
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint when spellcheck is off")
	}
	// Spell ON, cursor on misspelled word → hint with suggestions.
	m.analysis.spell = true
	w, sugg, ok := m.cursorSpellHint()
	if !ok || w != "teh" || len(sugg) == 0 {
		t.Fatalf("expected hint for teh, got w=%q sugg=%v ok=%v", w, sugg, ok)
	}
	// Cursor on a correct word → no hint.
	m.editor.SetCursor(5) // inside "cat"
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint on a correctly-spelled word")
	}
	// Inside a modal → no hint even on the misspelled word.
	m.editor.SetCursor(1)
	m.suggesting = true
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint while the interactive menu is open")
	}
	m.suggesting = false
	m.renaming = true
	if _, _, ok := m.cursorSpellHint(); ok {
		t.Fatal("no hint while renaming")
	}
	m.renaming = false
}

func TestFramedInspectorClickAlignment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabAnalysis
	m.layout()
	// Click the Adverb checkbox row at its on-screen position; it must toggle adverb.
	x := m.width - inspectorWidth + 4 // into the content (left border+padding+indent)
	// y must be wherever Adverb actually renders on screen — find it:
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	yAdverb := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Adverb") {
			yAdverb = i
		}
	}
	if yAdverb < 0 {
		t.Fatal("Adverb row not found on screen")
	}
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: yAdverb, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.adverb {
		t.Fatalf("clicking the on-screen Adverb row (y=%d) must toggle adverb — geometry misaligned", yAdverb)
	}
}

func TestStatusShowsSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1)
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "✗ teh") || !strings.Contains(out, "the") {
		t.Fatalf("status should show the spell hint for teh:\n%s", out)
	}
}

func TestTabClickColumnAlignment(t *testing.T) {
	// Clicking each tab at its real on-screen COLUMN (rune-based) must select it —
	// guards inspectorTabAtX against the panel's box-drawing offset.
	for _, tc := range []struct {
		label string
		want  inspectorTab
	}{{"Words", tabWords}, {"Outline", tabOutline}, {"Goals", tabGoals}, {"Analysis", tabAnalysis}} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
		t.Setenv("OKASHI_DIR", dir)
		m := initialModel()
		m.screen = screenWriting
		nm, _ := m.Update(tea.WindowSizeMsg{Width: 170, Height: 40})
		m = nm.(model)
		m.inspector.visible = true
		m.layout()
		col, row := -1, -1
		for y, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
			if i := strings.Index(ln, tc.label); i >= 0 {
				col, row = utf8.RuneCountInString(ln[:i]), y
				break
			}
		}
		if col < 0 {
			t.Fatalf("tab %q not on screen", tc.label)
		}
		nm, _ = m.Update(tea.MouseMsg{X: col, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if got := nm.(model).inspector.tab; got != tc.want {
			t.Errorf("click %q at col %d → tab %d, want %d", tc.label, col, got, tc.want)
		}
	}
}

func TestSidebarFramedClickAlignment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-alpha.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-bravo.md"), []byte("b"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 170, Height: 40})
	m = nm.(model)
	m.sidebarVisible = true
	m.inspector.visible = false
	m.layout()
	// Find a file entry's on-screen row and click it; it must become selected.
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	target, wantRow := "bravo", -1
	for y, ln := range lines {
		if strings.Contains(ln, target) {
			wantRow = y
			break
		}
	}
	if wantRow < 0 {
		t.Fatalf("file %q not on screen:\n%s", target, strings.Join(lines, "\n"))
	}
	nm, _ = m.Update(tea.MouseMsg{X: 3, Y: wantRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if sel := m.files.entries[m.files.selected].name; !strings.Contains(sel, target) {
		t.Fatalf("clicking the on-screen %q row (y=%d) selected %q instead — geometry misaligned", target, wantRow, sel)
	}
}

func TestF1HelpToggle(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	if m.showHelp {
		t.Fatal("help should default closed")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = nm.(model)
	if !m.showHelp {
		t.Fatal("F1 should open help")
	}
	if !strings.Contains(ansi.Strip(m.View()), "ctrl+b") {
		t.Fatalf("help view should list keybinds:\n%s", ansi.Strip(m.View()))
	}
	// esc closes; '?' would NOT toggle (it types text) — F1 closes here.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = nm.(model)
	if m.showHelp {
		t.Fatal("F1 again should close help")
	}
}

func TestEditorClickShowsSpellHint(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("the teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.sidebarVisible = false
	m.inspector.visible = false
	m.layout()
	// "the teh cat": "teh" is runes 4..7 on row 0. Editor text starts at editorStart + (editorArea-cw)/2.
	_, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	textLeft := (editorArea - cw) / 2 // editorStart 0 (no sidebar)
	nm, _ = m.Update(tea.MouseMsg{X: textLeft + 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	w, _, ok := m.cursorSpellHint()
	if !ok || w != "teh" {
		t.Fatalf("clicking 'teh' should land the cursor in it; hint word=%q ok=%v (col=%d)", w, ok, m.editor.CursorColumn())
	}
}

func TestClickSuggestionApplies(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("teh cat"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))
	m.analysis.spell = true
	m.sidebarVisible = false // editorStart = 0 so the click math below is simple
	m.inspector.visible = false
	m.layout()
	m.editor.MoveToLine(0)
	m.editor.SetCursor(1) // cursor in "teh" → hint active
	w, sugg, ok := m.cursorSpellHint()
	if !ok || len(sugg) == 0 {
		t.Fatalf("precondition: hint for %q should be active", w)
	}
	// The hint renders "✗ teh → the · ...". Click the first suggestion's column.
	// prefix = "✗ "(2) + "teh"(3) + " → "(3) = 8 display cols; first suggestion starts at
	// content col 8; status padding adds 1 → screen col 9. Click mid-suggestion.
	// Use display width (not byte offset) because "✗" and "→" are 3-byte/1-col each.
	stripped := ansi.Strip(m.statusBar())
	bidx := strings.Index(stripped, sugg[0])
	if bidx < 0 {
		t.Fatalf("suggestion %q not in the rendered hint", sugg[0])
	}
	idx := lipgloss.Width(stripped[:bidx]) // byte offset → display col
	nm, _ = m.Update(tea.MouseMsg{X: idx + 1 + 1, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if got := m.editor.Value(); got != sugg[0]+" cat" {
		t.Fatalf("clicking suggestion %q should apply it: value=%q", sugg[0], got)
	}
}

func TestPanelsFullHeight(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = nm.(model)
	m.sidebarVisible = true
	m.inspector.visible = true
	m.layout()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	// Panels are full height: their bottom border (╰) is on the very LAST row —
	// the status now lives inside the editor column on that same row.
	last := m.height - 1
	if !strings.Contains(lines[last], "╰") {
		t.Fatalf("panels should be full height — bottom border expected on row %d:\n%s", last, lines[last])
	}
	if m.files.height != m.height-2 {
		t.Fatalf("files.height = %d, want m.height-2 = %d", m.files.height, m.height-2)
	}
}

func TestRenameRendersInRow(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.focus = focusSidebar
	m.files.selectName("01-a.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming || !m.renamingInPane {
		t.Fatal("r should start an in-pane rename")
	}
	if strings.Contains(ansi.Strip(m.statusBar()), "rename ▸") {
		t.Fatal("in-pane rename must NOT use the bottom bar")
	}
}
