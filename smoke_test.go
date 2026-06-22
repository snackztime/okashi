package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	// Breadcrumb occupies row 0; the file list starts at screen row 1, so ".."=Y1, draft.md=Y2.
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
	if resolveColumnWidth() != 65 {
		t.Fatalf("default should be 65, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "72")
	if resolveColumnWidth() != 72 {
		t.Fatalf("env 72 should win, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "5") // out of range
	if resolveColumnWidth() != 65 {
		t.Fatalf("out-of-range should fall back to 65, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "abc")
	if resolveColumnWidth() != 65 {
		t.Fatalf("garbage should fall back to 65, got %d", resolveColumnWidth())
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

func TestSidebarClickRowAccountsForBreadcrumb(t *testing.T) {
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

	// Row 0 of the list is at screen Y=1 (breadcrumb is Y=0). Click it.
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
