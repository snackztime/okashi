package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPreviewToggle(t *testing.T) {
	m := initialModel()
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
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
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
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
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
