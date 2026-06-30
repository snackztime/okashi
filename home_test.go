package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildHomeItems(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"novel", "journal", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	recents := []string{"/abs/chapter-03.md", "/abs/note.md"}

	items := buildHomeItems(recents, dir)

	// 2 recents + 2 projects (hidden excluded) + 3 actions = 7
	if len(items) != 7 {
		t.Fatalf("want 7 items, got %d: %+v", len(items), items)
	}
	if items[0].kind != homeRecentFile || items[0].path != "/abs/chapter-03.md" {
		t.Fatalf("first item should be the most-recent file, got %+v", items[0])
	}
	if items[0].label != "chapter-03.md" {
		t.Fatalf("recent label should be the basename, got %q", items[0].label)
	}
	if items[2].kind != homeProject || items[2].label != "journal" {
		t.Fatalf("projects should be alpha-sorted after recents, got %+v", items[2])
	}
	if items[4].kind != homeNewDocument {
		t.Fatalf("5th item should be new document action, got %+v", items[4])
	}
	if items[6].kind != homeOpenOther {
		t.Fatalf("last item should be open-other, got %+v", items[6])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	// Should have the 3 actions
	if len(items) != 3 || items[2].kind != homeOpenOther {
		t.Fatalf("empty state should be the 3 actions, got %+v", items)
	}
}

func TestHomeViewUsesPerExtensionIconForRecents(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set so .md has a distinct glyph
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.homeItems = []homeItem{{kind: homeRecentFile, label: "chapter.md", path: "/x/chapter.md"}}
	m.resetHomeSelection()
	want := resolveIcons().icon(fileEntry{name: "chapter.md"})
	if !strings.Contains(m.homeView(), want) {
		t.Fatalf("recent row should use the .md glyph %q", want)
	}
}

func TestBuildHomeItemsHasActions(t *testing.T) {
	dir := t.TempDir()
	items := buildHomeItems(nil, dir) // no recents, no projects
	// Just the three actions, in order.
	if len(items) != 3 {
		t.Fatalf("want 3 action items, got %d: %+v", len(items), items)
	}
	want := []struct {
		kind  homeKind
		label string
	}{
		{homeNewDocument, "New document"},
		{homeNewProject, "New project"},
		{homeOpenOther, "Browse all files"},
	}
	for i, w := range want {
		if items[i].kind != w.kind || items[i].label != w.label {
			t.Fatalf("item %d = %+v, want kind %d label %q", i, items[i], w.kind, w.label)
		}
	}
}

func TestHomeContentAndHitTest(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = nm.(model)
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeRecentFile, label: "ch.md", path: "/r/ch.md"},
		{kind: homeNewDocument, label: "New document"},
	}
	m.resetHomeSelection()

	lines, cells, blockW := m.homeContent()
	if blockW <= 0 || len(lines) == 0 {
		t.Fatalf("homeContent: blockW=%d lines=%d", blockW, len(lines))
	}
	// One clickable cell per item (project, recent, action).
	if len(cells) != 3 {
		t.Fatalf("want 3 cells, got %d", len(cells))
	}
	// Each cell's screen coords must hit-test back to itself (render == hit-test).
	for _, c := range cells {
		x, y := homeCellXY(m, c.region, c.index)
		r, idx, ok := m.homeItemAt(x, y)
		if !ok || r != c.region || idx != c.index {
			t.Fatalf("hit-test of (region %d, idx %d) → (%d,%d,%v)", c.region, c.index, r, idx, ok)
		}
	}
	// Top-left (logo area) misses.
	if _, _, ok := m.homeItemAt(0, 0); ok {
		t.Fatal("click at (0,0) should miss")
	}
}

func TestClassifyLibraryAndFiles(t *testing.T) {
	ws := t.TempDir()
	os.MkdirAll(filepath.Join(ws, "my-novel"), 0o755)
	os.WriteFile(filepath.Join(ws, "my-novel", "manifest.json"),
		[]byte(`{"schemaVersion":1,"title":"My Novel","items":[{"file":"01-open.md","title":"Opening"}]}`), 0o644)
	os.WriteFile(filepath.Join(ws, "my-novel", "01-open.md"), []byte("# Opening\n\nThe fog rolled in off the bay.\n"), 0o644)
	os.WriteFile(filepath.Join(ws, "my-novel", "notes.md"), []byte("Loose notes.\n"), 0o644)
	os.MkdirAll(filepath.Join(ws, "research"), 0o755)
	os.WriteFile(filepath.Join(ws, "research", "sources.md"), []byte("Sources to read.\n"), 0o644)
	os.MkdirAll(filepath.Join(ws, ".hidden"), 0o755)

	projects, folders := classifyLibrary(ws)
	if len(projects) != 1 || projects[0].label != "my-novel" || projects[0].kind != homeProject {
		t.Fatalf("projects: %+v", projects)
	}
	if len(folders) != 1 || folders[0].label != "research" || folders[0].kind != homeFolder {
		t.Fatalf("folders: %+v", folders)
	}
	m := initialModel()
	files := m.homeFilesFor(filepath.Join(ws, "my-novel"))
	if len(files) != 2 || files[0].name != "Opening" || files[0].words == 0 || files[0].snippet == "" {
		t.Fatalf("project files: %+v", files)
	}
	cat := m.homeFilesFor(filepath.Join(ws, "research"))
	if len(cat) != 1 || cat[0].name != "sources.md" {
		t.Fatalf("category files: %+v", cat)
	}
}
