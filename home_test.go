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

	// 2 recents + 2 folders (hidden excluded) + 1 Notes + 3 actions = 8
	if len(items) != 8 {
		t.Fatalf("want 8 items, got %d: %+v", len(items), items)
	}
	if items[0].kind != homeRecentFile || items[0].path != "/abs/chapter-03.md" {
		t.Fatalf("first item should be the most-recent file, got %+v", items[0])
	}
	if items[0].label != "chapter-03.md" {
		t.Fatalf("recent label should be the basename, got %q", items[0].label)
	}
	// Plain dirs (no manifest / no numbered files) classify as FOLDERS, alpha-sorted after recents.
	if items[2].kind != homeFolder || items[2].label != "journal" {
		t.Fatalf("third item should be the first folder 'journal', got %+v", items[2])
	}
	// The ◦ Notes entry follows projects+folders, before the actions.
	if items[4].kind != homeLoose || items[4].label != "◦ Notes" {
		t.Fatalf("fifth item should be ◦ Notes, got %+v", items[4])
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
	// Should have ◦ Notes + the 3 actions = 4
	if len(items) != 4 || items[0].kind != homeLoose || items[3].kind != homeOpenOther {
		t.Fatalf("empty state should be Notes + 3 actions, got %+v", items)
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
	// ◦ Notes + the three actions, in order (empty workspace → Notes is first).
	if len(items) != 4 {
		t.Fatalf("want 4 items (Notes + 3 actions), got %d: %+v", len(items), items)
	}
	if items[0].kind != homeLoose || items[0].label != "◦ Notes" {
		t.Fatalf("item 0 should be ◦ Notes, got %+v", items[0])
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

func TestActionsRowHorizontalNav(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = nm.(model)
	m.focusAt(regionActions, 0)
	n := m.regionCount(regionActions)
	if n < 2 {
		t.Fatalf("expected multiple actions to navigate, got %d", n)
	}
	// Right/left move within the horizontal actions row.
	m.homeMove(1, 0)
	if m.homeRegion != regionActions || m.homeIndex != 1 {
		t.Fatalf("right should move to actions[1], got region=%d idx=%d", m.homeRegion, m.homeIndex)
	}
	m.homeMove(-1, 0)
	if m.homeIndex != 0 {
		t.Fatalf("left should move back to actions[0], got idx=%d", m.homeIndex)
	}
	m.homeMove(-1, 0) // clamp at 0
	if m.homeIndex != 0 {
		t.Fatalf("left at start should clamp at 0, got idx=%d", m.homeIndex)
	}
	for i := 0; i < n+2; i++ {
		m.homeMove(1, 0) // clamp at n-1
	}
	if m.homeIndex != n-1 {
		t.Fatalf("right past end should clamp at %d, got idx=%d", n-1, m.homeIndex)
	}
	// Down within the actions row is a no-op (nothing below).
	before := m.homeIndex
	m.homeMove(0, 1)
	if m.homeRegion != regionActions || m.homeIndex != before {
		t.Fatalf("down in actions should be a no-op, got region=%d idx=%d", m.homeRegion, m.homeIndex)
	}
	// Up exits the actions row back to a column.
	m.homeMove(0, -1)
	if m.homeRegion == regionActions {
		t.Fatal("up should exit the actions row to a column")
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
	// The ◦ Notes entry renders LAST in the library (under its own OTHER section).
	if len(lib) == 0 {
		t.Fatalf("library should be non-empty")
	}
	last := lib[len(lib)-1]
	if last.kind != homeLoose || last.label != "◦ Notes" {
		t.Fatalf("last library item should be '◦ Notes', got %+v", lib)
	}
	// Select ◦ Notes and confirm FILES shows the root's loose doc.
	m.librarySelected = len(lib) - 1
	m.recomputeHomeFiles()
	found := false
	for _, f := range m.homeFiles {
		if f.name == "stray.md" || f.name == "Stray" {
			found = true
		}
	}
	if !found {
		t.Fatalf("◦ Notes should list the root's loose docs, got %+v", m.homeFiles)
	}
}

func TestConfirmAddSourcePersistsAndSwitches(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	t.Cleanup(func() { os.Remove(sourcesPath()) }) // don't pollute shared config for later tests
	m := initialModel()
	newDir := t.TempDir()

	m.confirmAddSource(newDir)

	// It was appended, persisted, and became active.
	last := m.sources[len(m.sources)-1]
	if last.Kind != sourceKindFolder || last.root() != newDir {
		t.Fatalf("added source wrong: %+v", last)
	}
	if m.activeSource != len(m.sources)-1 {
		t.Fatalf("adding a source should switch to it, active=%d", m.activeSource)
	}
	if len(loadSources(sourcesPath())) < 2 {
		t.Fatalf("added source should be persisted")
	}
}

func TestConfirmAddSourceRejectsUnreachable(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	before := len(m.sources)
	m.confirmAddSource(filepath.Join(t.TempDir(), "does-not-exist"))
	if len(m.sources) != before {
		t.Fatalf("an unreachable path must not be added")
	}
}

func TestRemoveActiveSourceKeepsPrimary(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	m.confirmAddSource(t.TempDir()) // now on a folder source
	m.removeActiveSource()
	if len(m.sources) != 1 || m.activeSource != 0 {
		t.Fatalf("removing the active folder source should return to [primary], got %d sources active=%d", len(m.sources), m.activeSource)
	}
	m.removeActiveSource() // now on primary — must be a no-op
	if len(m.sources) != 1 {
		t.Fatalf("primary must not be removable")
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
