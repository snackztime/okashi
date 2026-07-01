package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestCycleSourceSwitchesAndRepopulates(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	// A second (folder) source with its own project.
	other := t.TempDir()
	createManuscript(filepath.Join(other, "other-novel"), "Other Novel", "Untitled")

	m := initialModel()
	m.sources = append(m.sources, newFolderSource(other))

	m.cycleSource(1)
	if m.activeSource != 1 {
		t.Fatalf("cycleSource should move to the folder source, got %d", m.activeSource)
	}
	if m.activeSourceRoot() != other {
		t.Fatalf("active root = %q, want %q", m.activeSourceRoot(), other)
	}
	// The library now reflects the other source's project.
	found := false
	for _, it := range m.library() {
		if it.label == "other-novel" {
			found = true
		}
	}
	if !found {
		t.Fatalf("library should show the folder source's project, got %+v", m.library())
	}
}

func TestCycleSourceSkipsUnreachable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	m := initialModel()
	m.sources = append(m.sources, newFolderSource(filepath.Join(t.TempDir(), "gone"))) // unreachable
	m.cycleSource(1)
	if m.activeSource != 0 {
		t.Fatalf("cycling should skip an unreachable source and stay on primary, got %d", m.activeSource)
	}
}

func TestBuildHomeItems(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"novel", "journal", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	recents := []string{"/abs/chapter-03.md", "/abs/note.md"}

	items := buildHomeItems(recents, dir)

	// 2 recents + 1 Loose + 2 folders (hidden excluded) + 3 actions = 8
	if len(items) != 8 {
		t.Fatalf("want 8 items, got %d: %+v", len(items), items)
	}
	if items[0].kind != homeRecentFile || items[0].path != "/abs/chapter-03.md" {
		t.Fatalf("first item should be the most-recent file, got %+v", items[0])
	}
	if items[0].label != "chapter-03.md" {
		t.Fatalf("recent label should be the basename, got %q", items[0].label)
	}
	// Loose entry follows recents.
	if items[2].kind != homeLoose || items[2].label != "◦ Loose" {
		t.Fatalf("third item should be ◦ Loose, got %+v", items[2])
	}
	// Plain dirs (no manifest / no numbered files) classify as FOLDERS now.
	if items[3].kind != homeFolder || items[3].label != "journal" {
		t.Fatalf("folders should be alpha-sorted after Loose, got %+v", items[3])
	}
	if items[5].kind != homeNewDocument {
		t.Fatalf("6th item should be new document action, got %+v", items[5])
	}
	if items[7].kind != homeOpenOther {
		t.Fatalf("last item should be open-other, got %+v", items[7])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	// Should have ◦ Loose + the 3 actions = 4
	if len(items) != 4 || items[0].kind != homeLoose || items[3].kind != homeOpenOther {
		t.Fatalf("empty state should be Loose + 3 actions, got %+v", items)
	}
}

func TestHomeViewShowsRecentName(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.homeItems = []homeItem{{kind: homeRecentFile, label: "chapter.md", path: "/x/chapter.md"}}
	m.resetHomeSelection()
	if !strings.Contains(ansi.Strip(m.homeView()), "chapter.md") {
		t.Fatal("RECENT column should list the recent file name")
	}
}

func TestBuildHomeItemsHasActions(t *testing.T) {
	dir := t.TempDir()
	items := buildHomeItems(nil, dir) // no recents, no projects
	// ◦ Loose + the three actions, in order.
	if len(items) != 4 {
		t.Fatalf("want 4 items (Loose + 3 actions), got %d: %+v", len(items), items)
	}
	if items[0].kind != homeLoose || items[0].label != "◦ Loose" {
		t.Fatalf("item 0 should be ◦ Loose, got %+v", items[0])
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
		if items[i+1].kind != w.kind || items[i+1].label != w.label {
			t.Fatalf("item %d = %+v, want kind %d label %q", i+1, items[i+1], w.kind, w.label)
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

func TestActiveSourceRootDefaultsToWritingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	if len(m.sources) < 1 || m.sources[0].Kind != sourceKindPrimary {
		t.Fatalf("model should load sources with primary first, got %+v", m.sources)
	}
	if m.activeSource != 0 {
		t.Fatalf("activeSource should start at 0 (primary), got %d", m.activeSource)
	}
	if m.activeSourceRoot() != dir {
		t.Fatalf("activeSourceRoot() = %q, want writingDir() %q", m.activeSourceRoot(), dir)
	}
}

func TestLooseEntryShowsRootDocs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("# Stray\n\nA loose note."), 0o644)
	os.MkdirAll(filepath.Join(root, "acat"), 0o755) // a category folder (not loose)

	m := initialModel()
	lib := m.library()
	if len(lib) == 0 || lib[0].kind != homeLoose || lib[0].label != "◦ Loose" {
		t.Fatalf("first library item should be '◦ Loose', got %+v", lib)
	}
	// Select ◦ Loose and confirm FILES shows the root's loose doc.
	m.librarySelected = 0
	m.recomputeHomeFiles()
	found := false
	for _, f := range m.homeFiles {
		if f.name == "stray.md" || f.name == "Stray" {
			found = true
		}
	}
	if !found {
		t.Fatalf("◦ Loose should list the root's loose docs, got %+v", m.homeFiles)
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
