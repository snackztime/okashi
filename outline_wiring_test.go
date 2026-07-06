package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOutlineManifestTitleAndOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("a b"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":1,"title":"Windermere","items":[`+
			`{"file":"the-letter.md","title":"The Letter"},`+
			`{"file":"opening.md","title":"Chapter One"}]}`), 0o644)
	var o outlineModel
	o.wc = newWordCountCache()
	o.width = 60
	o.height = 10
	o.load(dir, o.wc)
	// Manifest order: the-letter before opening despite filename alpha.
	if len(o.working) != 2 || o.working[0].name != "the-letter.md" || o.working[1].name != "opening.md" {
		t.Fatalf("outline should follow manifest order; working = %+v", o.working)
	}
	// Titles come from the manifest.
	if o.chapterTitle("the-letter.md") != "The Letter" {
		t.Fatalf("title for the-letter.md should be 'The Letter', got %q", o.chapterTitle("the-letter.md"))
	}
	if o.chapterTitle("opening.md") != "Chapter One" {
		t.Fatalf("title for opening.md should be 'Chapter One', got %q", o.chapterTitle("opening.md"))
	}
	// View contains manifest titles, not filename slugs.
	view := o.View()
	if !strings.Contains(view, "The Letter") || !strings.Contains(view, "Chapter One") {
		t.Fatalf("outline View should show manifest titles:\n%s", view)
	}
}

func setupManuscript(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("three"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

// NOTE: the pop-down binder (screenOutline) is being retired (Task 5); ctrl+k now toggles the
// pane corkboard (see TestCtrlKTogglesCorkboard). These tests still exercise the binder screen —
// which remains live until Task 5 removes it — by entering it directly via enterOutline().

func TestOutlineEnterOpensSection(t *testing.T) {
	m, proj := setupManuscript(t)
	m.enterOutline()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select 02-b
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should return to the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("Enter should open the selected section, currentFile = %q", m.currentFile)
	}
}

func TestOutlineEscReturnsToEditor(t *testing.T) {
	m, _ := setupManuscript(t)
	m.enterOutline()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("esc from the outline should return to the editor, got %v", m.screen)
	}
}

func TestOutlineHandlesResize(t *testing.T) {
	m, _ := setupManuscript(t)
	m.enterOutline()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m = nm.(model)
	if m.outline.width != 70 || m.outline.height != 19 {
		t.Fatalf("resize on the outline should update outline dims to 70x19, got %dx%d", m.outline.width, m.outline.height)
	}
}

func TestOutlineClickSelectsThenDoubleClickOpens(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a (row 0), 02-b (row 1)
	m.enterOutline()
	var nm tea.Model
	// Click row 1 (02-b): mouse Y = header height + 1.
	clickY := outlineHeaderHeight + 1
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.outline.selected != 1 {
		t.Fatalf("click should select row 1, got %d", m.outline.selected)
	}
	// Second click on the same row opens it.
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.screen != screenWriting || m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("double-click should open 02-b.md, screen=%v file=%q", m.screen, m.currentFile)
	}
}
