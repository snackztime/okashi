# Launch Screen + Icons + Autosave — Implementation Plan (Plan 1 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add debounced autosave, Nerd-Font file-pane icons (with a plain fallback), and a Crush-style launch screen (recent files + projects) that collapses into a bannerless minimal writing zone.

**Architecture:** A new `screen` (home/writing) gates the top-level View; the banner renders only on `home`. Autosave is a single lifetime `tea.Tick`. Icons resolve once from env. Recents persist to a JSON file under the user config dir. Focus dimming (spec Section 5) is intentionally **deferred to Plan 2** — it needs a dedicated render patch.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, vendored `okashi/internal/textarea`.

## Global Constraints

- Module path `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- Icons: Nerd Font glyphs by default; `OKASHI_ICONS=plain` (or `ascii`) selects a plain Unicode set. Resolved once at startup.
- Autosave: single ~1s lifetime tick; writes only when `dirty && currentFile != "" && idle ≥ 2s`; on write error keep `dirty` (retry); never start a second tick loop.
- Recents: `<os.UserConfigDir()>/okashi/recent.json`, JSON `{"files":[...]}`, most-recent-first, deduped, capped **15**; missing/corrupt → empty list (no error surfaced); missing config dir → recents disabled.
- Banner (`bannerView`) renders ONLY in `screen == screenHome`. In `screenWriting` there is no banner; the file list starts at screen row 0 (mouse offset 0).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.
- Allowed writing extensions (unchanged): `.md .txt .wg .markdown`.

---

## File Structure

- **Create** `icons.go` — `iconSet`, `resolveIcons()`, `icon(fileEntry)`.
- **Create** `recent.go` — `recentPath()`, `loadRecents()`, `addRecent()`.
- **Create** `home.go` — `homeItem`/`homeKind`, `buildHomeItems()`, `homeView()`, `updateHome()`, `openHomeSelection()`.
- **Create** `icons_test.go`, `recent_test.go`, `home_test.go`.
- **Modify** `main.go` — model fields (autosave, screen, home, icons), `Init`, `Update` (screen gate, autosave tick, dirty tracking, `ctrl+o`, mouse offset), `View`/`layout` (screen branch, banner removal), `save`/`loadFile` (dirty + recents).
- **Modify** `filelist.go` — `icons` field, icon rendering, drop `/` suffix, folder tint.
- **Modify** `smoke_test.go`, `filelist_test.go` — integration tests.
- **Modify** `README.md` — keys, icons env, launch screen, autosave.

---

## Task 1: Autosave

**Files:**
- Modify: `main.go` (model fields, `Init`, `Update`, `save`, `statusBar`)
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `model.dirty bool`, `model.lastEditAt time.Time`; `type autosaveTickMsg time.Time`; `func autosaveTick() tea.Cmd`; `func (m model) autosaveDue(now time.Time) bool`. `save()` clears `dirty` on success, keeps it on error.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
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
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestAutosave -v 2>&1 | tail`
Expected: build error — `m.autosaveDue undefined`, `autosaveTickMsg undefined`.

- [ ] **Step 3: Add fields, tick, and due-check**

In `main.go` model struct, after `lastClickTime time.Time`:

```go
	dirty      bool
	lastEditAt time.Time
```

Add near `sidebarRow` (top-level):

```go
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
```

- [ ] **Step 4: Start the tick in Init**

Replace `func (m model) Init() tea.Cmd { return nil }` with:

```go
func (m model) Init() tea.Cmd {
	return autosaveTick()
}
```

- [ ] **Step 5: Handle the tick, and mark dirty on real edits**

In `Update`'s top-level `switch msg := msg.(type)`, add a case (next to `tea.WindowSizeMsg`):

```go
	case autosaveTickMsg:
		if m.autosaveDue(time.Time(msg)) {
			m.save()
		}
		return m, autosaveTick()
```

In the editor-routing `else` branch (currently `m.editor, cmd = m.editor.Update(msg)`), replace it with:

```go
	} else {
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
		}
	}
```

- [ ] **Step 6: `save()` clears/keeps dirty**

In `save()`, change the error branch and success path:

```go
	if err := os.WriteFile(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
		m.status = "save failed: " + err.Error()
		return // dirty stays true → retried next tick
	}
	m.dirty = false
	m.status = "saved " + filepath.Base(m.currentFile)
```

(Leave the existing sidebar-refresh block below it unchanged.)

- [ ] **Step 7: Run the tests**

Run: `/opt/homebrew/bin/go test . -run TestAutosave -v 2>&1 | tail`
Expected: both PASS.

- [ ] **Step 8: Add the saved indicator to the status bar**

Find `statusBar()`. Where it builds `stats := m.statsText()`, change to:

```go
	mark := "✓"
	if m.dirty {
		mark = "●"
	}
	stats := mark + " " + m.statsText()
```

- [ ] **Step 9: gofmt, full suite, commit**

```bash
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Autosave: debounced ~2s-idle flush with dirty indicator"
```

---

## Task 2: Icons (`icons.go`)

**Files:**
- Create: `icons.go`, `icons_test.go`

**Interfaces:**
- Produces: `type iconSet struct { folder, parent, file string; byExt map[string]string }`; `func resolveIcons() iconSet`; `func (s iconSet) icon(e fileEntry) string`.

- [ ] **Step 1: Write the failing test**

Create `icons_test.go`:

```go
package main

import (
	"testing"
)

func TestResolveIconsPlainViaEnv(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	s := resolveIcons()
	if len(s.byExt) != 0 {
		t.Fatal("plain set should have no per-extension icons")
	}
	if s.icon(fileEntry{name: "a.md"}) != s.file {
		t.Fatal("plain: a file should use the generic file icon")
	}
}

func TestIconMapping(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set
	s := resolveIcons()
	if s.icon(fileEntry{name: ".."}) != s.parent {
		t.Fatal(".. should use the parent icon")
	}
	if s.icon(fileEntry{name: "proj", isDir: true}) != s.folder {
		t.Fatal("a dir should use the folder icon")
	}
	if s.icon(fileEntry{name: "ch.md"}) != s.byExt[".md"] {
		t.Fatal(".md should use its mapped icon")
	}
	if s.icon(fileEntry{name: "x.unknown"}) != s.file {
		t.Fatal("unknown ext should use the generic file icon")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestResolveIcons|TestIconMapping' -v 2>&1 | tail`
Expected: build error — `resolveIcons undefined`.

- [ ] **Step 3: Implement `icons.go`**

Create `icons.go` (the glyphs are Nerd Font code points; tweak freely — they're cosmetic):

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
)

// iconSet is the glyph set for the file pane and launch lists. Each glyph
// string includes its own trailing space for alignment.
type iconSet struct {
	folder string
	parent string
	file   string
	byExt  map[string]string
}

// resolveIcons picks the glyph set once at startup. OKASHI_ICONS=plain (or
// ascii) avoids Nerd Font glyphs for terminals without a patched font.
func resolveIcons() iconSet {
	switch strings.ToLower(os.Getenv("OKASHI_ICONS")) {
	case "plain", "ascii":
		return iconSet{folder: "▸ ", parent: "↑ ", file: "  ", byExt: map[string]string{}}
	}
	return iconSet{
		folder: " ", // nf-fa-folder
		parent: " ", // nf-fa-arrow_up
		file:   " ", // nf-fa-file
		byExt: map[string]string{
			".md":       " ", // nf-oct-markdown
			".markdown": " ",
			".txt":      " ", // nf-fa-file_text
			".wg":       " ",
			".go":       " ", // nf-seti-go
		},
	}
}

// icon returns the glyph for an entry.
func (s iconSet) icon(e fileEntry) string {
	if e.name == ".." {
		return s.parent
	}
	if e.isDir {
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}
```

- [ ] **Step 4: Run the tests; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestResolveIcons|TestIconMapping' -v 2>&1 | tail -5
/opt/homebrew/bin/gofmt -w icons.go icons_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add icons.go icons_test.go
git commit -m "icons: Nerd Font glyph set with plain fallback"
```

---

## Task 3: Wire icons into the file pane

**Files:**
- Modify: `filelist.go` (add `icons` field, render icons, drop `/`, tint dirs), `main.go` (model `icons` field)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `iconSet` (Task 2).
- Produces: `filelist.icons iconSet` (set by `newFilelist`); `model.icons iconSet`.

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go`:

```go
func TestFilelistViewShowsIconsNoSlash(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	f := newFilelist()
	f.width = 20
	f.height = 5
	f.entries = []fileEntry{{name: "proj", isDir: true}, {name: "a.md"}}

	view := f.View()
	if strings.Contains(view, "proj/") {
		t.Fatal("dir should not get a trailing slash (the icon conveys it)")
	}
	if !strings.Contains(view, "▸ proj") {
		t.Fatalf("plain folder icon missing; view=%q", view)
	}
}
```

(Confirm `filelist_test.go` imports `strings`; add it if missing.)

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestFilelistViewShowsIcons -v 2>&1 | tail`
Expected: FAIL — current View has no icon / still uses `/`.

- [ ] **Step 3: Add the `icons` field and set it**

In `filelist.go`, add to the `filelist` struct:

```go
	icons iconSet
```

In `newFilelist()`, add to the returned literal:

```go
		icons: resolveIcons(),
```

- [ ] **Step 4: Render icons, drop the slash, tint dirs**

In `filelist.View()`, replace the per-entry loop body. Current:

```go
		e := f.entries[i]
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		label = ansi.Truncate(label, f.width, "…")
		if i == f.selected {
			b.WriteString(selectedStyle.Width(f.width).Render(label))
		} else {
			b.WriteString(label)
		}
```

becomes:

```go
		e := f.entries[i]
		label := ansi.Truncate(f.icons.icon(e)+e.name, f.width, "…")
		switch {
		case i == f.selected:
			b.WriteString(selectedStyle.Width(f.width).Render(label))
		case e.isDir:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(label))
		default:
			b.WriteString(label)
		}
```

- [ ] **Step 5: Add `model.icons` (for the launch screen later)**

In `main.go` model struct, after `mdStyle string`:

```go
	icons iconSet
```

In `initialModel`, set it in the returned literal:

```go
		icons: resolveIcons(),
```

- [ ] **Step 6: Run tests, commit**

```bash
/opt/homebrew/bin/go test . -run TestFilelist -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w filelist.go main.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go main.go filelist_test.go
git commit -m "Render file-pane icons (folder tint, no slash suffix)"
```

---

## Task 4: Recent-files store (`recent.go`)

**Files:**
- Create: `recent.go`, `recent_test.go`

**Interfaces:**
- Produces: `func recentPath() string`; `func loadRecents(path string) []string`; `func addRecent(path, file string)`. Cap 15, dedup, most-recent-first; load filters non-existent paths; empty `path` → no-op/empty.

- [ ] **Step 1: Write the failing test**

Create `recent_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecentsAddDedupCapAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")

	// Create real files so load doesn't filter them out.
	var paths []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".md")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}

	addRecent(store, paths[0])
	addRecent(store, paths[1])
	addRecent(store, paths[2])
	addRecent(store, paths[0]) // re-add moves to front, no dup

	got := loadRecents(store)
	want := []string{paths[0], paths[2], paths[1]}
	if len(got) != 3 {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v, want %v", got, want)
		}
	}
}

func TestRecentsLoadFiltersMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")

	// Missing file → empty.
	if len(loadRecents(store)) != 0 {
		t.Fatal("missing store should load empty")
	}
	// Corrupt → empty, no panic.
	if err := os.WriteFile(store, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadRecents(store)) != 0 {
		t.Fatal("corrupt store should load empty")
	}
	// A path that no longer exists is dropped.
	gone := filepath.Join(dir, "gone.md")
	addRecent(store, gone)
	if len(loadRecents(store)) != 0 {
		t.Fatal("non-existent recent path should be filtered on load")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestRecents -v 2>&1 | tail`
Expected: build error — `addRecent undefined`.

- [ ] **Step 3: Implement `recent.go`**

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const recentCap = 15

type recentFile struct {
	Files []string `json:"files"`
}

// recentPath returns the recent-files store path, or "" if there is no usable
// user config dir (recents then silently disabled).
func recentPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "recent.json")
}

// loadRecents reads the store, dropping entries whose path no longer exists.
// Missing/corrupt/empty-path all yield an empty slice.
func loadRecents(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r recentFile
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	out := make([]string, 0, len(r.Files))
	for _, f := range r.Files {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// addRecent prepends file to the store (dedup, cap recentCap). No-ops on an
// empty path or any write error.
func addRecent(path, file string) {
	if path == "" || file == "" {
		return
	}
	existing := readRecentsRaw(path)
	out := []string{file}
	for _, f := range existing {
		if f != file {
			out = append(out, f)
		}
	}
	if len(out) > recentCap {
		out = out[:recentCap]
	}
	data, err := json.Marshal(recentFile{Files: out})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// readRecentsRaw reads the stored list without existence-filtering (so adding a
// new file doesn't silently drop still-pending entries).
func readRecentsRaw(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r recentFile
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	return r.Files
}
```

- [ ] **Step 4: Run tests, commit**

```bash
/opt/homebrew/bin/go test . -run TestRecents -v 2>&1 | tail -5
/opt/homebrew/bin/gofmt -w recent.go recent_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add recent.go recent_test.go
git commit -m "recent-files store: load/add with dedup, cap, missing-path filter"
```

---

## Task 5: Home items builder (`home.go` part 1)

**Files:**
- Create: `home.go`, `home_test.go`

**Interfaces:**
- Produces:
  - `type homeKind int` with `homeRecentFile`, `homeProject`, `homeOpenOther`.
  - `type homeItem struct { kind homeKind; label, path string }`.
  - `func buildHomeItems(recents []string, projectsDir string) []homeItem` — recents (most-recent-first) then projects (subdirs of projectsDir, alpha, non-hidden), then a final `homeOpenOther`.

- [ ] **Step 1: Write the failing test**

Create `home_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
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

	// 2 recents + 2 projects (hidden excluded) + 1 open-other = 5
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d: %+v", len(items), items)
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
	if items[4].kind != homeOpenOther {
		t.Fatalf("last item should be open-other, got %+v", items[4])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	if len(items) != 1 || items[0].kind != homeOpenOther {
		t.Fatalf("empty state should be just open-other, got %+v", items)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestBuildHomeItems -v 2>&1 | tail`
Expected: build error — `buildHomeItems undefined`.

- [ ] **Step 3: Implement `home.go` (builder + types)**

```go
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type homeKind int

const (
	homeRecentFile homeKind = iota
	homeProject
	homeOpenOther
)

// homeItem is one selectable row on the launch screen.
type homeItem struct {
	kind  homeKind
	label string // basename / project name / action label
	path  string // file path, project dir, or "" for the action
}

// buildHomeItems composes the launch list: recent files (most-recent-first),
// then project folders (immediate non-hidden subdirs of projectsDir, alpha),
// then a final "Open another folder…" action.
func buildHomeItems(recents []string, projectsDir string) []homeItem {
	var items []homeItem
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}

	if entries, err := os.ReadDir(projectsDir); err == nil {
		var dirs []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
		for _, d := range dirs {
			items = append(items, homeItem{kind: homeProject, label: d, path: filepath.Join(projectsDir, d)})
		}
	}

	items = append(items, homeItem{kind: homeOpenOther, label: "Open another folder…"})
	return items
}
```

- [ ] **Step 4: Run tests, commit**

```bash
/opt/homebrew/bin/go test . -run TestBuildHomeItems -v 2>&1 | tail -5
/opt/homebrew/bin/gofmt -w home.go home_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add home.go home_test.go
git commit -m "home: launch-list builder (recents + projects + open-other)"
```

---

## Task 6: Screen state + home rendering & navigation (`home.go` part 2 + `main.go`)

**Files:**
- Modify: `main.go` (model fields, `initialModel`, `Update` screen gate), `home.go` (`updateHome`, `homeView`)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `buildHomeItems`, `model.icons`.
- Produces: `type screen int` (`screenHome`, `screenWriting`); `model.screen`, `model.homeItems []homeItem`, `model.homeSelected int`; `func (m model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd)`; `func (m model) homeView() string`.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestLaunchStartsOnHome -v 2>&1 | tail`
Expected: build error — `screenHome undefined` / `m.screen undefined`.

- [ ] **Step 3: Add screen + home fields and the constants**

In `main.go`, add near the `focus` const block:

```go
type screen int

const (
	screenHome screen = iota
	screenWriting
)
```

Add to the model struct (after `focus focus`):

```go
	screen       screen
	homeItems    []homeItem
	homeSelected int
```

In `initialModel`, set them in the returned literal:

```go
		screen:    screenHome,
		homeItems: buildHomeItems(loadRecents(recentPath()), writingDir()),
```

- [ ] **Step 4: Gate Update on the home screen**

At the very top of `Update`, before the `if m.creatingFile` block:

```go
	if m.screen == screenHome {
		return m.updateHome(msg)
	}
```

- [ ] **Step 5: Implement `updateHome` and `homeView` in `home.go`**

Add to `home.go` (add imports `tea "github.com/charmbracelet/bubbletea"` and `"github.com/charmbracelet/lipgloss"`):

```go
// updateHome handles input on the launch screen.
func (m model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.homeSelected > 0 {
				m.homeSelected--
			}
		case "down", "j":
			if m.homeSelected < len(m.homeItems)-1 {
				m.homeSelected++
			}
		case "enter":
			m.openHomeSelection()
		}
	}
	return m, nil
}

// homeView renders the centered logo and the launch list with group headers.
func (m model) homeView() string {
	header := lipgloss.NewStyle().Foreground(subtle).Bold(true)
	var b strings.Builder
	printedRecent, printedProjects := false, false
	for i, it := range m.homeItems {
		switch it.kind {
		case homeRecentFile:
			if !printedRecent {
				b.WriteString(header.Render("RECENT") + "\n")
				printedRecent = true
			}
		case homeProject:
			if !printedProjects {
				b.WriteString("\n" + header.Render("PROJECTS") + "\n")
				printedProjects = true
			}
		case homeOpenOther:
			b.WriteString("\n")
		}

		icon := m.icons.file
		if it.kind == homeProject {
			icon = m.icons.folder
		} else if it.kind == homeOpenOther {
			icon = m.icons.folder
		}
		line := "  " + icon + it.label
		if i == m.homeSelected {
			line = selectedStyle.Render(" " + icon + it.label + " ")
		}
		b.WriteString(line + "\n")
	}

	logo := bannerView(m.width)
	content := lipgloss.JoinVertical(lipgloss.Center, logo, "", b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
```

- [ ] **Step 6: Stub `openHomeSelection` (filled in Task 7)**

So Task 6 compiles and tests pass, add a minimal version to `home.go` now; Task 7 replaces it:

```go
// openHomeSelection acts on the highlighted launch item. (Completed in the
// transitions task.)
func (m *model) openHomeSelection() {
	if len(m.homeItems) == 0 {
		return
	}
	m.screen = screenWriting
}
```

- [ ] **Step 7: Branch `View` on screen**

In `View`, right after the `if m.width == 0` guard, add:

```go
	if m.screen == screenHome {
		return m.homeView()
	}
```

- [ ] **Step 8: Run the test, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestLaunchStartsOnHome -v 2>&1 | tail -5
/opt/homebrew/bin/gofmt -w main.go home.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go home.go smoke_test.go
git commit -m "Launch screen: home/writing state + home render & navigation"
```

---

## Task 7: Transitions, banner removal, recents wiring

**Files:**
- Modify: `home.go` (`openHomeSelection`), `main.go` (`ctrl+o`, `View`/`layout` banner removal, mouse offset, `loadFile`/`save` recents)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: everything prior.
- Produces: `openHomeSelection` dispatches by kind into `screenWriting`; `ctrl+o` returns to `screenHome` (rebuilding the list); writing-mode View/layout omit the banner; mouse row offset is 0 in writing mode; opening/first-saving updates recents.

- [ ] **Step 1: Write the failing tests**

Add to `smoke_test.go`:

```go
func TestHomeOpenProjectAndCtrlOReturns(t *testing.T) {
	dir := t.TempDir()
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.homeItems = []homeItem{{kind: homeProject, label: "p", path: dir}}
	m.homeSelected = 0

	// Enter on a project → writing mode, sidebar rooted at the project.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatal("opening a project should switch to writing")
	}
	if m.files.dir != dir {
		t.Fatalf("sidebar should be rooted at the project, got %q", m.files.dir)
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
```

- [ ] **Step 2: Run to verify they fail**

Run: `/opt/homebrew/bin/go test . -run 'TestHomeOpenProject|TestWritingViewHasNoBanner' -v 2>&1 | tail`
Expected: FAIL — project root not set / banner still present.

- [ ] **Step 3: Complete `openHomeSelection`**

Replace the stub in `home.go` with:

```go
// openHomeSelection acts on the highlighted launch item and enters writing mode.
func (m *model) openHomeSelection() {
	if len(m.homeItems) == 0 {
		return
	}
	it := m.homeItems[m.homeSelected]
	switch it.kind {
	case homeRecentFile:
		m.files.SetDir(filepath.Dir(it.path))
		m.loadFile(it.path)
		m.focus = focusEditor
		m.editor.Focus()
	case homeProject:
		m.files.SetDir(it.path)
		m.focus = focusSidebar
		m.editor.Blur()
	case homeOpenOther:
		m.files.SetDir(writingDir())
		m.focus = focusSidebar
		m.editor.Blur()
	}
	m.screen = screenWriting
	m.layout()
}
```

- [ ] **Step 4: Add `ctrl+o` (return to home) in writing mode**

In `Update`'s `tea.KeyMsg` switch (writing mode), add a case alongside `ctrl+n`:

```go
		case "ctrl+o":
			m.previewing = false
			m.screen = screenHome
			m.homeItems = buildHomeItems(loadRecents(recentPath()), writingDir())
			m.homeSelected = 0
			return m, nil
```

- [ ] **Step 5: Remove the banner in writing mode (View + layout)**

In `View`, replace the banner/bodyH block:

```go
	banner := bannerView(m.width)
	bodyH := m.height - lipgloss.Height(banner) - 1 // 1 row for status
	if bodyH < 1 {
		bodyH = 1
	}
```

with:

```go
	bodyH := m.height - 1 // status only; no banner in the writing zone
	if bodyH < 1 {
		bodyH = 1
	}
```

and replace the final return:

```go
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, banner, body, status)
```

with:

```go
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
```

In `layout`, replace:

```go
	bodyH := m.height - lipgloss.Height(bannerView(m.width)) - 1
```

with:

```go
	bodyH := m.height - 1 // no banner in the writing zone
```

- [ ] **Step 6: Fix the mouse row offset (no banner above the list now)**

In the `tea.MouseMsg` click branch, replace:

```go
		bannerH := lipgloss.Height(bannerView(m.width))
		row := sidebarRow(msg.Y, bannerH, m.files.height)
```

with:

```go
		// No banner in writing mode: the file list starts at screen row 0.
		row := sidebarRow(msg.Y, 0, m.files.height)
```

- [ ] **Step 7: Update recents on open and on save**

In `loadFile`, after `m.status = "opened " + filepath.Base(path)`:

```go
	addRecent(recentPath(), path)
```

In `save`, after `m.dirty = false` (success path), before the status line is fine too — add:

```go
	addRecent(recentPath(), m.currentFile)
```

- [ ] **Step 8: Run the tests, full suite, build, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestHomeOpenProject|TestWritingViewHasNoBanner|TestLaunch' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w main.go home.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go home.go smoke_test.go
git commit -m "Launch transitions, bannerless writing zone, recents wiring"
```

---

## Task 8: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the keys table**

Add a row to the keys table in `README.md`:

```
| `ctrl+o` | Back to the launch screen (recent files / projects) |
```

- [ ] **Step 2: Add a "Launch screen", "Icons", and "Autosave" note**

After the "Markdown preview" section, add:

```markdown
## Launch screen

okashi opens on a launch screen: your **recent files** and your **projects**
(the folders in your okashi dir), plus "Open another folder…". Pick a recent
file to jump straight in, or a project to browse it in the sidebar. Once you're
in a file the logo disappears — a full minimal writing zone. `ctrl+o` returns
to the launch screen.

## Icons

The file pane and launch lists use Nerd Font glyphs. If your terminal isn't
using a Nerd Font, set `OKASHI_ICONS=plain` for a plain-Unicode set.

## Autosave

Your work saves automatically a couple seconds after you stop typing (for any
file with a name — `ctrl+n` names it up front). The `●`/`✓` mark by the word
count shows unsaved vs saved. `ctrl+s` still saves on demand.
```

- [ ] **Step 3: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "Docs: launch screen, icons env, autosave"
```

- [ ] **Step 5: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` — verify: launch screen shows logo + recent/projects with icons; arrow keys + Enter open a project (sidebar) or recent (straight into the file); banner is gone while writing; type and pause → `●` flips to `✓` and the file is written; `ctrl+o` returns to launch. Try `OKASHI_ICONS=plain go run .` for the fallback glyphs. (Icon glyphs, centering, and autosave timing are only fully confirmed here.)

---

## Self-Review

**Spec coverage (Plan 1 scope):**
- Two screens / minimal zone → Tasks 6–7 (screen state, bannerless writing, `ctrl+o`).
- Launch screen Recent + Projects + Open-other → Tasks 5–7.
- Recent persistence (location, cap 15, dedup, missing-filter) → Task 4, wired in Task 7.
- Icons (Nerd + plain via env, folder tint, no slash, reused on home) → Tasks 2–3, home in Task 6.
- Autosave (1s lifetime tick, 2s idle, currentFile-only, keep-dirty-on-error, indicator) → Task 1.
- **Deferred to Plan 2:** focus dimming (spec Section 5). Settings pane + rope buffer remain out of scope per spec.

**Placeholder scan:** none — every code step shows full code; the Task 6 `openHomeSelection` stub is explicitly replaced in Task 7 (noted in both).

**Type consistency:** `iconSet`/`resolveIcons`/`icon`, `recentPath`/`loadRecents`/`addRecent`, `homeKind`/`homeItem`/`buildHomeItems`, `screen`/`screenHome`/`screenWriting`, `model.{screen,homeItems,homeSelected,icons,dirty,lastEditAt}`, `autosaveTickMsg`/`autosaveTick`/`autosaveDue`, `updateHome`/`homeView`/`openHomeSelection` are used consistently across tasks. Mouse offset switched from `lipgloss.Height(bannerView())` to `0` in the same task that removes the banner (Task 7) — no stale dependency.

**Cross-cutting check:** removing the banner (Task 7) updates BOTH `layout` height math AND the mouse `sidebarRow` offset in the same task, so click mapping stays correct.
