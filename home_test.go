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

	// 2 recents + 2 folders (hidden excluded) + 1 Notes + 1 Browse action = 6
	// (New document / New project are now the inline + on the panels, not actions.)
	if len(items) != 6 {
		t.Fatalf("want 6 items, got %d: %+v", len(items), items)
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
	// The ◦ Notes entry follows projects+folders, before the single Browse action.
	if items[4].kind != homeLoose || items[4].label != "◦ Notes" {
		t.Fatalf("fifth item should be ◦ Notes, got %+v", items[4])
	}
	if items[5].kind != homeOpenOther || items[5].label != "Browse all files" {
		t.Fatalf("last item should be the Browse action, got %+v", items[5])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	// Should have ◦ Notes + the single Browse action = 2
	if len(items) != 2 || items[0].kind != homeLoose || items[1].kind != homeOpenOther {
		t.Fatalf("empty state should be Notes + Browse, got %+v", items)
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

func TestBuildHomeItemsHasBrowseAction(t *testing.T) {
	dir := t.TempDir()
	items := buildHomeItems(nil, dir) // no recents, no projects
	// The only action is Browse (creation moved to the inline + on the panels).
	acts := homeGroupsActions(items)
	if len(acts) != 1 || acts[0].kind != homeOpenOther || acts[0].label != "Browse all files" {
		t.Fatalf("actions should be exactly [Browse all files], got %+v", acts)
	}
}

// homeGroupsActions returns just the action items from a home-item list (test helper).
func homeGroupsActions(items []homeItem) []homeItem {
	_, _, _, _, a := homeGroups(items)
	return a
}

func TestActionsRowHorizontalNav(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = nm.(model)
	// Production has a single Browse action; use a synthetic multi-action list to exercise the
	// horizontal nav logic (left/right between actions, up-exit, down no-op).
	m.homeItems = []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeNewDocument, label: "New document"},
		{kind: homeNewProject, label: "New project"},
		{kind: homeOpenOther, label: "Browse all files"},
	}
	m.resetHomeSelection()
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

func TestFilesCascadeDrillDown(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	cat := filepath.Join(root, "worldbuild")
	os.MkdirAll(filepath.Join(cat, "characters"), 0o755)
	os.WriteFile(filepath.Join(cat, "overview.md"), []byte("# Overview\n\ntext"), 0o644)
	os.WriteFile(filepath.Join(cat, "characters", "alice.md"), []byte("# Alice\n\nhi"), 0o644)

	m := initialModel()
	lib := m.library()
	sel := -1
	for i, it := range lib {
		if it.label == "worldbuild" {
			sel = i
		}
	}
	if sel < 0 {
		t.Fatalf("worldbuild not in library: %+v", lib)
	}
	m.focusAt(regionLibrary, sel)

	// FILES shows the subfolder (drillable) first, then the doc.
	if len(m.homeFiles) < 2 || !m.homeFiles[0].isDir || m.homeFiles[0].name != "characters" {
		t.Fatalf("FILES should lead with the 'characters' subfolder, got %+v", m.homeFiles)
	}
	// Drill into it — stays on the home screen.
	m.focusAt(regionFiles, 0)
	m.openHomeSelection()
	if m.screen != screenHome {
		t.Fatalf("drilling a folder should stay on home, got %v", m.screen)
	}
	if len(m.homeFiles) == 0 || m.homeFiles[0].name != ".." {
		t.Fatalf("drilled FILES should start with '..', got %+v", m.homeFiles)
	}
	foundAlice := false
	for _, f := range m.homeFiles {
		if !f.isDir && (f.name == "alice.md" || f.name == "Alice") {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Fatalf("drilled FILES should show alice.md, got %+v", m.homeFiles)
	}
	// ".." returns to the category root.
	m.focusAt(regionFiles, 0)
	m.openHomeSelection()
	if m.homeFilesDir != cat {
		t.Fatalf("'..' should return to %q, got %q", cat, m.homeFilesDir)
	}
	// Selecting a file opens it (leaves home).
	oi := -1
	for i, f := range m.homeFiles {
		if !f.isDir {
			oi = i
			break
		}
	}
	m.focusAt(regionFiles, oi)
	m.openHomeSelection()
	if m.screen != screenWriting {
		t.Fatalf("selecting a file should open it, got %v", m.screen)
	}
}

func TestNotesStaysFlat(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "acat"), 0o755) // a subfolder at the root
	os.WriteFile(filepath.Join(root, "loose.md"), []byte("hi"), 0o644)
	m := initialModel()
	lib := m.library()
	m.focusAt(regionLibrary, len(lib)-1) // ◦ Notes is last
	for _, f := range m.homeFiles {
		if f.isDir {
			t.Fatalf("Notes must not show folders/drill, got %+v", m.homeFiles)
		}
	}
	found := false
	for _, f := range m.homeFiles {
		if f.name == "loose.md" || f.name == "loose" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Notes should show the root loose doc, got %+v", m.homeFiles)
	}
}

func TestHomeVerticalFlow(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 26})
	m = nm.(model)
	// A recent (strip), a folder (library), and the Browse action — no files under the folder.
	m.homeItems = []homeItem{
		{kind: homeRecentFile, label: "x.md", path: "/r/x.md"},
		{kind: homeFolder, label: "research", path: "/p/research"},
		{kind: homeOpenOther, label: "Browse all files"},
	}
	m.resetHomeSelection()
	if m.homeRegion != regionRecent {
		t.Fatalf("focus should start on the RECENT strip, got %d", m.homeRegion)
	}
	// down: strip → a column
	m.homeMove(0, 1)
	col := m.homeRegion
	if col != regionLibrary && col != regionFiles {
		t.Fatalf("down from the strip should enter a column, got %d", col)
	}
	// up from the column top → back to the strip
	m.homeMove(0, -1)
	if m.homeRegion != regionRecent {
		t.Fatalf("up from a column top should return to the strip, got %d", m.homeRegion)
	}
	// bottom of the column, then down → actions
	m.focusAt(col, m.regionCount(col)-1)
	m.homeMove(0, 1)
	if m.homeRegion != regionActions {
		t.Fatalf("down from a column bottom should enter the actions row, got %d", m.homeRegion)
	}
	// up from actions → leaves the actions row
	m.homeMove(0, -1)
	if m.homeRegion == regionActions {
		t.Fatal("up from the actions row should leave it")
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
	// 3 item cells (recent, project, action) + 2 inline-+ cells (LIBRARY, FILES panels) = 5.
	if len(cells) != 5 {
		t.Fatalf("want 5 cells, got %d", len(cells))
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

func TestHomeInlineCreate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)

	// LIBRARY + roots creation at the active source root with New-Project semantics.
	m := initialModel()
	m.homeCreate(regionLibrary)
	if m.files.dir != root {
		t.Fatalf("LIBRARY + should root at the active source root, got %q", m.files.dir)
	}
	if !m.creatingFile || !m.creatingFolder {
		t.Fatalf("LIBRARY + should start a New-Project-style create, got file=%v folder=%v", m.creatingFile, m.creatingFolder)
	}
	if m.screen != screenWriting {
		t.Fatalf("create should switch to the writing screen, got %v", m.screen)
	}

	// FILES + roots creation at the selected library item's dir as a plain document.
	m2 := initialModel()
	lib := m2.library()
	m2.librarySelected = len(lib) - 1 // ◦ Notes → the source root
	m2.homeCreate(regionFiles)
	if m2.files.dir != root {
		t.Fatalf("FILES + should root at the selected item dir, got %q", m2.files.dir)
	}
	if !m2.creatingFile || m2.creatingFolder {
		t.Fatalf("FILES + should start a new-document create (not folder), got file=%v folder=%v", m2.creatingFile, m2.creatingFolder)
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
	files := m.homeFilesFor(filepath.Join(ws, "my-novel"), true)
	if len(files) != 2 || files[0].name != "Opening" || files[0].words == 0 || files[0].snippet == "" {
		t.Fatalf("project files: %+v", files)
	}
	cat := m.homeFilesFor(filepath.Join(ws, "research"), true)
	if len(cat) != 1 || cat[0].name != "sources.md" {
		t.Fatalf("category files: %+v", cat)
	}
}
