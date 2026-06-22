# Launch Hub + Creation + Launch Mouse — Implementation Plan (Plan 1 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the launch screen into a hub with New document / New project / Browse actions, generalize file/folder creation (with a live prompt hint), and make the launch screen mouse-driven.

**Architecture:** `home.go` gains action items + a shared `homeRows` layout helper used by both render and mouse hit-test. `main.go` generalizes `confirmNewFile` → `confirmCreate` (files + folders) and composes a dynamic create-prompt in `statusBar`. The file-pane *look* and clickable breadcrumb are Plan 2.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, vendored textarea.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- New documents (no project) are created at the okashi workspace root (`writingDir()`).
- `name/` (trailing slash) makes a folder; the hub's **New project** makes a folder explicitly. Hub New-project ENTERS the new folder; sidebar `name/` creates-and-stays.
- Create-prompt label flips to `new folder ▸` when in folder mode (explicit or trailing `/`); static hint `end with / for a folder` shown otherwise.
- Launch mouse mirrors the file pane: single-click selects, double-click (<400ms, same item) activates, wheel moves selection.
- Render and mouse hit-test MUST share one layout calc (`homeRows`).
- Tests touching `writingDir()` set `t.Setenv("OKASHI_DIR", t.TempDir())` to stay hermetic.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Launch hub action items

**Files:**
- Modify: `icons.go` (add `action` glyph), `home.go` (`homeKind`, `buildHomeItems`, `homeView` icons/blank)
- Test: `home_test.go`, `icons_test.go`

**Interfaces:**
- Produces: `homeKind` values `homeNewDocument`, `homeNewProject`; `iconSet.action string`; `buildHomeItems` appends the three actions (New document, New project, Browse all files) after projects.

- [ ] **Step 1: Write the failing test**

Add to `home_test.go`:

```go
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
```

Add to `icons_test.go`:

```go
func TestIconSetHasAction(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	if resolveIcons().action == "" {
		t.Fatal("plain icon set should have a non-empty action glyph")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestBuildHomeItemsHasActions|TestIconSetHasAction' -v 2>&1 | tail`
Expected: build errors — `homeNewDocument undefined`, `resolveIcons().action undefined`.

- [ ] **Step 3: Add the `action` glyph to `icons.go`**

In `icons.go`, add `action string` to the `iconSet` struct (after `file string`). In `resolveIcons`, add to the plain set: `action: "+ "`; and to the nerd set: `action: " "` (nf-fa-plus, U+F067 — if the literal glyph risks mangling, the byte sequence must be U+F067 followed by a space).

- [ ] **Step 4: Add the action kinds and items in `home.go`**

Change the `homeKind` const block to:

```go
const (
	homeRecentFile homeKind = iota
	homeProject
	homeNewDocument
	homeNewProject
	homeOpenOther
)
```

In `buildHomeItems`, replace the final single append:

```go
	items = append(items, homeItem{kind: homeOpenOther, label: "Browse all files"})
```

with:

```go
	items = append(items,
		homeItem{kind: homeNewDocument, label: "New document"},
		homeItem{kind: homeNewProject, label: "New project"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
```

- [ ] **Step 5: Render the actions group in `homeView`**

In `homeView`, the group-header switch currently puts a blank line before `homeOpenOther`. Change that case so the blank line precedes the actions group (the first action) instead:

```go
		case homeNewDocument:
			b.WriteString("\n") // blank line before the actions group
```

(Remove the `case homeOpenOther: b.WriteString("\n")`.)

And the icon switch — add the action kinds:

```go
		var icon string
		switch it.kind {
		case homeProject, homeOpenOther:
			icon = m.icons.folder
		case homeNewDocument, homeNewProject:
			icon = m.icons.action
		default:
			icon = m.icons.icon(fileEntry{name: it.label})
		}
```

- [ ] **Step 6: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestBuildHomeItems|TestIconSet' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w icons.go home.go home_test.go icons_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add icons.go home.go home_test.go icons_test.go
git commit -m "Launch hub: New document / New project action items"
```

---

## Task 2: `confirmCreate` (files + folders) + prompt hint

**Files:**
- Modify: `main.go` (rename `confirmNewFile`→`confirmCreate`, add folder handling, `creatingFolder` field, dynamic create-prompt in `statusBar`, clear `nameInput.Prompt`)
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `model.creatingFolder bool`; `func (m *model) confirmCreate()`. Folder creation via explicit `creatingFolder` (enters folder) or trailing `/` (stays). `statusBar` shows the live `new file ▸`/`new folder ▸` label + hint.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestConfirmCreate -v 2>&1 | tail`
Expected: build error — `m.confirmCreate undefined`.

- [ ] **Step 3: Add `creatingFolder` and rewrite the function as `confirmCreate`**

In the model struct, after `creatingFile bool`:

```go
	creatingFolder bool
```

Replace the whole `confirmNewFile` function with:

```go
// confirmCreate turns the typed name into a new file or folder in the current
// pane dir. A trailing "/" (or an explicit New-project) makes a folder; an
// explicit New-project then enters it, while the sidebar "name/" convention
// creates-and-stays. Files default to .md and open a blank buffer.
func (m *model) confirmCreate() {
	name := strings.TrimSpace(m.nameInput.Value())
	explicitFolder := m.creatingFolder
	m.creatingFile = false
	m.creatingFolder = false
	m.nameInput.Blur()
	if name == "" {
		m.status = "create cancelled (no name)"
		return
	}

	folder := explicitFolder || strings.HasSuffix(name, "/")
	name = strings.TrimSuffix(name, "/")

	if folder {
		dir := filepath.Join(m.files.dir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "couldn't create folder: " + err.Error()
			return
		}
		if explicitFolder {
			m.files.SetDir(dir) // New project → enter the folder
			m.status = "new project " + name
		} else {
			m.files.SetDir(m.files.dir) // name/ → refresh, stay
			m.files.selectName(name)
			m.status = "created folder " + name
		}
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
```

- [ ] **Step 4: Update the call site and clear the input prompt**

In `Update`, the `creatingFile` branch calls `m.confirmNewFile()` on Enter — change it to `m.confirmCreate()`.

In `initialModel`, change `ti.Prompt = "new file ▸ "` to `ti.Prompt = ""` (the label now lives in `statusBar`).

- [ ] **Step 5: Compose the dynamic create-prompt in `statusBar`**

Replace the `statusBar` prompt branch:

```go
	if m.creatingFile {
		return m.nameInput.View()
	}
```

with:

```go
	if m.creatingFile {
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
```

- [ ] **Step 6: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestConfirmCreate -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "confirmCreate: files + folders with a live prompt hint"
```

---

## Task 3: Wire hub New document / New project to the prompt

**Files:**
- Modify: `home.go` (`openHomeSelection` returns `tea.Cmd`, handles the new kinds), `main.go` (none beyond Task 2)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `confirmCreate`, `creatingFolder`, `creatingFile`.
- Produces: `func (m *model) openHomeSelection() tea.Cmd`; New document/New project enter `screenWriting` with the create prompt open (file vs folder mode) rooted at `writingDir()`.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestHubNew' -v 2>&1 | tail`
Expected: FAIL — the new kinds aren't handled; prompt not opened.

- [ ] **Step 3: Make `openHomeSelection` return a cmd and handle the new kinds**

Replace `openHomeSelection` with:

```go
func (m *model) openHomeSelection() tea.Cmd {
	if len(m.homeItems) == 0 {
		return nil
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
	case homeNewDocument:
		m.files.SetDir(writingDir())
		m.screen = screenWriting
		return m.startCreate(false)
	case homeNewProject:
		m.files.SetDir(writingDir())
		m.screen = screenWriting
		return m.startCreate(true)
	}
	m.screen = screenWriting
	m.layout()
	return nil
}

// startCreate opens the name prompt in file or folder mode.
func (m *model) startCreate(folder bool) tea.Cmd {
	m.creatingFile = true
	m.creatingFolder = folder
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m.editor.Blur()
	m.layout()
	return textinput.Blink
}
```

(Add `"github.com/charmbracelet/bubbles/textinput"` to `home.go` imports.)

- [ ] **Step 4: Return the cmd from `updateHome`**

In `updateHome`, change the `enter` case from `m.openHomeSelection()` to:

```go
		case "enter":
			return m, m.openHomeSelection()
```

- [ ] **Step 5: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestHubNew|TestHomeOpenProject|TestOpenProjectKeeps' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w home.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add home.go smoke_test.go
git commit -m "Wire hub New document / New project to the create prompt"
```

---

## Task 4: Launch hub mouse (shared `homeRows` layout)

**Files:**
- Modify: `home.go` (`homeRows` helper, `homeView` uses it, `homeItemAtY`, `updateHome` mouse), `main.go` (model `lastClickRow`/`lastClickTime` already exist — reused)
- Test: `home_test.go`, `smoke_test.go`

**Interfaces:**
- Produces: `func homeRows(items []homeItem, sel int, icons iconSet) (lines []string, itemRow []int, height int)`; `func homeItemAtY(items []homeItem, sel int, icons iconSet, screenH, y int) int`. `updateHome` handles `tea.MouseMsg`.

- [ ] **Step 1: Write the failing test**

Add to `home_test.go`:

```go
func TestHomeRowsAndHitTest(t *testing.T) {
	items := []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeNewDocument, label: "New document"},
	}
	ic := resolveIcons()
	lines, itemRow, h := homeRows(items, 0, ic)
	if h != len(lines) {
		t.Fatalf("height %d != len(lines) %d", h, len(lines))
	}
	if len(itemRow) != len(items) {
		t.Fatalf("itemRow should have one entry per item, got %d", len(itemRow))
	}
	if itemRow[0] >= itemRow[1] {
		t.Fatal("item rows should be strictly increasing")
	}
	// A click on item 0's content row (centered in a tall screen) hits item 0.
	screenH := 40
	off := (screenH - h) / 2
	if got := homeItemAtY(items, 0, ic, screenH, off+itemRow[0]); got != 0 {
		t.Fatalf("hit-test at item 0's row = %d, want 0", got)
	}
	// A click on a non-item row (row 0 of content = a header/logo area) misses.
	if got := homeItemAtY(items, 0, ic, screenH, off+0); got == 0 && itemRow[0] != 0 {
		t.Fatal("hit-test on a non-item row should not return item 0")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestHomeRowsAndHitTest -v 2>&1 | tail`
Expected: build error — `homeRows undefined`.

- [ ] **Step 3: Extract `homeRows`; refactor `homeView` to use it**

Replace `homeView` with a `homeRows` helper plus a thin `homeView`:

```go
// homeRows builds the launch content: the logo, group headers, and item rows.
// It returns the lines, the content-row of each item (by index), and the total
// height — so the renderer and the mouse hit-test share one layout.
func homeRows(items []homeItem, sel int, icons iconSet) (lines []string, itemRow []int, height int) {
	header := lipgloss.NewStyle().Foreground(subtle).Bold(true)
	logo := bannerView(0) // width-independent height; horizontal centering is applied in homeView
	for _, l := range strings.Split(logo, "\n") {
		lines = append(lines, l)
	}
	lines = append(lines, "") // gap under the logo

	itemRow = make([]int, len(items))
	printedRecent, printedProjects := false, false
	for i, it := range items {
		switch it.kind {
		case homeRecentFile:
			if !printedRecent {
				lines = append(lines, header.Render("RECENT"))
				printedRecent = true
			}
		case homeProject:
			if !printedProjects {
				lines = append(lines, "", header.Render("PROJECTS"))
				printedProjects = true
			}
		case homeNewDocument:
			lines = append(lines, "")
		}

		var icon string
		switch it.kind {
		case homeProject, homeOpenOther:
			icon = icons.folder
		case homeNewDocument, homeNewProject:
			icon = icons.action
		default:
			icon = icons.icon(fileEntry{name: it.label})
		}
		row := "  " + icon + it.label
		if i == sel {
			row = selectedStyle.Render(" " + icon + it.label + " ")
		}
		itemRow[i] = len(lines)
		lines = append(lines, row)
	}
	return lines, itemRow, len(lines)
}

func (m model) homeView() string {
	lines, _, _ := homeRows(m.homeItems, m.homeSelected, m.icons)
	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// homeItemAtY maps an absolute screen Y to a launch item index, or -1.
func homeItemAtY(items []homeItem, sel int, icons iconSet, screenH, y int) int {
	_, itemRow, h := homeRows(items, sel, icons)
	off := (screenH - h) / 2
	if off < 0 {
		off = 0
	}
	contentRow := y - off
	for i, r := range itemRow {
		if r == contentRow {
			return i
		}
	}
	return -1
}
```

(Note: `bannerView(0)` is used only for its line count/text; `homeView` re-centers horizontally via `lipgloss.Place`. The banner art is fixed-width ASCII, so its rows are identical regardless of width — `bannerView` centers horizontally but the row strings are what we join. If `bannerView(0)` produces an odd horizontal pad, switch to `bannerArt` directly: `strings.Split(bannerArt, "\n")`. Use `bannerArt`.)

Correction for determinism — in `homeRows` use the raw art, not `bannerView`:

```go
	for _, l := range strings.Split(bannerArt, "\n") {
		lines = append(lines, l)
	}
```

- [ ] **Step 4: Add mouse handling to `updateHome`**

In `updateHome`, add a `tea.MouseMsg` case to the `switch msg := msg.(type)`:

```go
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.homeSelected > 0 {
				m.homeSelected--
			}
		case tea.MouseButtonWheelDown:
			if m.homeSelected < len(m.homeItems)-1 {
				m.homeSelected++
			}
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			idx := homeItemAtY(m.homeItems, m.homeSelected, m.icons, m.height, msg.Y)
			if idx < 0 {
				return m, nil
			}
			m.homeSelected = idx
			now := time.Now()
			if idx == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
				m.lastClickTime = time.Time{}
				return m, m.openHomeSelection()
			}
			m.lastClickRow = idx
			m.lastClickTime = now
		}
		return m, nil
```

(Add `"time"` to `home.go` imports.)

- [ ] **Step 5: Run tests, full suite, build, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestHomeRows|TestLaunch|TestHubNew' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w home.go home_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add home.go home_test.go smoke_test.go
git commit -m "Launch hub mouse: click/double-click/wheel via shared homeRows"
```

---

## Task 5: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the launch-screen docs**

In `README.md`'s "Launch screen" section, add a sentence:

```markdown
From the launch screen you can open a recent file or project, **create a new
document or project**, or browse all files — by keyboard or mouse (click to
select, double-click to open). Type a name ending in `/` to make a folder.
```

- [ ] **Step 2: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Docs: launch hub (new document/project, mouse, folder creation)"
```

- [ ] **Step 4: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` → on the launch screen: arrow/click to New document → name it → it opens; New project → name it → you're inside the new folder; in the sidebar `ctrl+n` then type `foo/` → watch the prompt flip to "new folder ▸" and a folder appear; double-click a recent/project to open it; wheel scrolls the hub. (Hit-testing and the prompt flip are only fully confirmed here.)

---

## Self-Review

**Spec coverage (Plan 1 scope — spec Sections 1, 2, 3a):**
- Launch hub actions → Task 1. `confirmCreate` files+folders + prompt hint → Task 2. Hub New document/project wiring → Task 3. Launch mouse via shared `homeRows` → Task 4. Docs → Task 5.
- **Deferred to Plan 2:** clickable breadcrumb (3b) + clean look (4).

**Placeholder scan:** none — full code in every step. The `homeRows` banner-source note resolves to using `bannerArt` directly (deterministic rows).

**Type consistency:** `homeKind` (`homeNewDocument`/`homeNewProject`), `iconSet.action`, `confirmCreate`, `model.creatingFolder`, `openHomeSelection() tea.Cmd`, `startCreate(bool) tea.Cmd`, `homeRows`/`homeItemAtY` are used consistently across tasks. `updateHome`'s enter now returns `m.openHomeSelection()` (cmd) — matched in Task 3.

**Cross-cutting check:** render and hit-test both call `homeRows` (Task 4) → no layout drift. Mouse double-click reuses the existing `lastClickRow`/`lastClickTime`.
