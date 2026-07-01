# Structural Editing — Chunk 2: Structure Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A full-page **structure mode** (`screenStructure`) — entered with `s` from the `ctrl+k` binder — where you reorder / add / remove / retitle a manuscript's chapters in a staged buffer and commit them on exit with one confirm, using the chunk-1 structural writers.

**Architecture:** A new `structure.go` holds the structure-mode state machine (enter, update, view, commit) so `main.go` doesn't grow. The edits mutate an in-memory `[]manifestItem` buffer (`structureItems`) plus a `structurePendingNew` set of new-blank files to create; on commit, pending files are written and the whole buffer is persisted via the existing atomic `writeManifest`. Manifest manuscripts only.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `manifest.go` (`manifest`, `manifestItem`, `readManifest`, `writeManifest`, `hasManifest`), `atomicWrite`, `sectionTitle` (`project.go`), `bubbles/textinput` (`m.nameInput`), lipgloss. Chunk-1 writers exist but structure mode builds its committed order in-buffer and calls `writeManifest` directly (it does not need `manifestReorder`/`manifestInsert` for the commit — the buffer IS the order).

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`**, gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **Manifest manuscripts only.** Entry requires `hasManifest(dir)` + a readable manifest; otherwise a status and no entry (legacy numbered manuscripts are not reorderable).
- **Staged, commit-on-exit.** All edits mutate the in-memory buffer; nothing touches disk until the user confirms on `esc`. One atomic `writeManifest` per commit. Cancel discards.
- **Schema stays v1**; the commit writes `manifest{schemaVersion:1, title, items: structureItems}` through the existing `writeManifest` (sorted-keys / no-HTML / no-trailing-newline parity — never hand-marshal).
- **Read-modify-write:** on commit, re-read the on-disk manifest's `title` immediately before writing (its `items` is superseded by the buffer).
- **Contract flip (Task 5, both repos):** this is the first *user-reachable* structural edit, so `CLAUDE.md` §1 + inkmere docs move from "structural edits planned behind a confirmation" → "shipped, confirm-gated." Schema unchanged.
- **Default build stays pure-Go;** no new dependency.

---

### Task 1: Screen skeleton — enter from the binder, render, navigate, exit

**Files:**
- Create: `structure.go`
- Modify: `main.go` — model struct (add fields near `homeFilesDir`), the `screen` const block (add `screenStructure`), `updateOutline` (add `s` case), the `Update` + `View` screen dispatch.
- Test: `structure_test.go` (create)

**Interfaces:**
- Consumes: `readManifest`, `hasManifest`, `manifestItem` (`manifest.go`); `m.outline.dir`; `framedPanel` (`inspector.go`); lipgloss.
- Produces:
  - model fields `structureDir string`, `structureItems []manifestItem`, `structureSel int`, `structurePendingNew map[string]bool`, `structureDirty bool`, `structureAdding bool`, `structureAddSel int`, `structureRenaming bool`, `structureConfirm bool`
  - `const screenStructure` (in the screen iota block)
  - `func (m *model) enterStructure()`
  - `func (m model) updateStructure(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func (m model) structureView() string`

- [ ] **Step 1: Write the failing test**

Create `structure_test.go`:

```go
package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func enterStructureAt(t *testing.T, root, name, first string) model {
	t.Helper()
	dir := filepath.Join(root, name)
	createManuscript(dir, name, first)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()
	return m
}

func TestStructureEnterLoadsChaptersAndExits(t *testing.T) {
	root := t.TempDir()
	m := enterStructureAt(t, root, "novel", "Opening")
	if m.screen != screenStructure {
		t.Fatalf("s should enter structure mode, screen=%v", m.screen)
	}
	if len(m.structureItems) != 1 || m.structureItems[0].Title != "Opening" {
		t.Fatalf("structure should load the manifest items, got %+v", m.structureItems)
	}
	if !strings.Contains(ansiStrip(m.structureView()), "Opening") {
		t.Fatalf("structure view should show the chapter title")
	}
	// esc with no edits exits back to the binder (screenOutline).
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("esc (clean) should return to the binder, screen=%v", m.screen)
	}
}

func TestStructureRefusesNonManifest(t *testing.T) {
	root := t.TempDir()
	// a plain folder with a numbered file = legacy manuscript (no manifest)
	dir := filepath.Join(root, "legacy")
	if err := writeFileP(filepath.Join(dir, "01-a.md"), "x"); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	m.outline.dir = dir
	m.enterStructure()
	if m.screen == screenStructure {
		t.Fatal("a legacy (manifest-less) manuscript must not enter structure mode")
	}
}
```

Add these tiny test helpers at the bottom of `structure_test.go` (or reuse existing equivalents if `ansiStrip`/a write helper already exist — check `smoke_test.go`; if `ansi.Strip` is used directly elsewhere, import `github.com/charmbracelet/x/ansi` and call `ansi.Strip` instead of `ansiStrip`):

```go
func ansiStrip(s string) string { return ansi.Strip(s) } // if not already defined; import x/ansi
func writeFileP(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
```

**Note:** before writing helpers, grep for existing ones — `ansi.Strip` is imported in `home_test.go`; a temp-file writer may already exist. Prefer the existing pattern; only add what's missing so the package still compiles (no duplicate symbols).

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestStructure -v`
Expected: FAIL — `undefined: screenStructure` / `enterStructure`.

- [ ] **Step 3: Add the screen constant + model fields + dispatch**

In `main.go`, add `screenStructure` to the screen iota block (after `screenSearch`):

```go
	screenStructure
```

Add to the model struct (near `homeFilesDir`):

```go
	structureDir        string           // the manuscript being restructured
	structureItems      []manifestItem   // staged chapter order/membership (committed on exit)
	structureSel        int              // cursor row
	structurePendingNew map[string]bool  // new-blank files to create on commit
	structureDirty      bool             // any staged edit?
	structureAdding     bool             // the add-pick sub-mode is open
	structureAddSel     int              // cursor in the add-pick
	structureRenaming   bool             // the retitle field is open (reuses nameInput)
	structureConfirm    bool             // the commit confirm bar is open
```

In `main.go`'s `Update`, add dispatch alongside the other screens (near `if m.screen == screenSearch`):

```go
	if m.screen == screenStructure {
		return m.updateStructure(msg)
	}
```

In `main.go`'s `View`, likewise:

```go
	if m.screen == screenStructure {
		return m.structureView()
	}
```

In `updateOutline`'s key switch (after `case "m":`), add:

```go
		case "s":
			m.enterStructure()
			return m, nil
```

- [ ] **Step 4: Create `structure.go` (enter + nav + esc + render)**

```go
package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// enterStructure opens structure mode for the binder's current manuscript. Manifest manuscripts
// only — a legacy/absent/unreadable manifest keeps the binder with a status.
func (m *model) enterStructure() {
	dir := m.outline.dir
	sm, present, err := readManifest(dir)
	if !present || err != nil {
		m.status = "not reorderable — no manifest"
		return
	}
	m.structureDir = dir
	m.structureItems = append([]manifestItem{}, sm.Items...)
	m.structureSel = 0
	m.structurePendingNew = map[string]bool{}
	m.structureDirty = false
	m.structureAdding = false
	m.structureRenaming = false
	m.structureConfirm = false
	m.screen = screenStructure
}

// structureTitle is the manuscript's display title (from the on-disk manifest, falling back to the
// folder name).
func (m model) structureTitle() string {
	if sm, present, err := readManifest(m.structureDir); present && err == nil && sm.Title != "" {
		return sm.Title
	}
	return projectTitle(filepathBase(m.structureDir))
}

// exitStructure returns to the binder (screenOutline), reloading it.
func (m *model) exitStructure() {
	m.outline.load(m.structureDir, m.files.wc)
	m.screen = screenOutline
}

func (m model) updateStructure(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.structureSel > 0 {
				m.structureSel--
			}
		case "down", "j":
			if m.structureSel < len(m.structureItems)-1 {
				m.structureSel++
			}
		case "esc":
			m.exitStructure()
			return m, nil
		}
	}
	return m, nil
}

func (m model) structureView() string {
	var b strings.Builder
	rows := make([]string, 0, len(m.structureItems))
	for i, it := range m.structureItems {
		num := lipgloss.NewStyle().Foreground(subtle).Render(fmtNum(i + 1))
		label := it.Title
		if m.structureSel == i {
			label = selectedStyle.Render(label)
		}
		rows = append(rows, num+"  "+label)
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("(no chapters)"))
	}
	body := framedPanel(m.structureTitle()+" — structure", strings.Join(rows, "\n"),
		max(40, min(m.width-8, 72)), len(rows)+2, "")
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	foot := lipgloss.NewStyle().Foreground(subtle).Render(
		"J/K move · a add · x remove · r retitle · esc commit")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
```

Add two tiny helpers to `structure.go` (or reuse existing `filepath.Base` / a number formatter — check first; `fmtNum` should zero-pad to 2 like the sidebar's index):

```go
func filepathBase(p string) string { return filepath.Base(p) } // import path/filepath
func fmtNum(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
```

(Import `path/filepath` and `strconv` in `structure.go`. If a zero-pad-2 helper already exists in the codebase, use it instead of `fmtNum`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestStructure -v` then the full suite `/opt/homebrew/bin/go test ./...`. vet + gofmt clean.

- [ ] **Step 6: Commit**

```bash
git add structure.go main.go structure_test.go
git commit -m "feat: structure mode skeleton (enter from binder, render chapters, navigate, exit)"
```

---

### Task 2: Buffer edits — reorder (`J`/`K`), remove (`x`), retitle (`r`)

**Files:**
- Modify: `structure.go` (`updateStructure` key handling; add the retitle sub-mode); `structure_test.go`

**Interfaces:**
- Consumes: `m.structureItems`, `m.structureSel`, `m.structureDirty`, `m.structureRenaming`, `m.nameInput`, `m.structurePendingNew` (Task 1).
- Produces: `J`/`K` reorder, `x` remove, `r` retitle behavior; the retitle field renders in the view.

- [ ] **Step 1: Write the failing tests**

Append to `structure_test.go`:

```go
func TestStructureReorderRemoveRetitle(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	// add two more chapters to the manifest on disk so we have [One, Two, Three]
	writeManifest(dir, manifest{SchemaVersion: 1, Title: "Novel", Items: []manifestItem{
		{File: "01-one.md", Title: "One"}, {File: "02-two.md", Title: "Two"}, {File: "03-three.md", Title: "Three"},
	}})
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	// Move chapter 1 (One) down with J → [Two, One, Three]
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	if m.structureItems[0].Title != "Two" || m.structureItems[1].Title != "One" {
		t.Fatalf("J should move the selected chapter down, got %v", titles(m.structureItems))
	}
	if !m.structureDirty {
		t.Fatal("reorder should mark dirty")
	}
	// The cursor follows the moved item (now at index 1).
	if m.structureSel != 1 {
		t.Fatalf("cursor should follow the moved item, sel=%d", m.structureSel)
	}

	// Remove the selected chapter (One) with x → [Two, Three]
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = nm.(model)
	if len(m.structureItems) != 2 || titlesJoined(m.structureItems) != "Two|Three" {
		t.Fatalf("x should remove the selected chapter, got %v", titles(m.structureItems))
	}

	// Retitle the selected chapter with r + type + enter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.structureRenaming {
		t.Fatal("r should open the retitle field")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "Second")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.structureItems[m.structureSel].Title != "Second" {
		t.Fatalf("retitle should update the buffer title, got %q", m.structureItems[m.structureSel].Title)
	}
}

func titles(items []manifestItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}
func titlesJoined(items []manifestItem) string { return strings.Join(titles(items), "|") }
```

**Note:** `typeInto(t, m, "Second")` is the existing test helper that feeds runes to the model's active `nameInput` (used by other prompt tests — grep for `func typeInto`). Reuse it; do not redefine.

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run TestStructureReorder -v`
Expected: FAIL — `J`/`x`/`r` unhandled (no reorder/remove/retitle).

- [ ] **Step 3: Implement the edits**

In `updateStructure`, extend the `tea.KeyMsg` switch. First, the retitle sub-mode must capture input BEFORE the normal keys (mirror the add-source prompt pattern in `home.go`). At the top of the `tea.KeyMsg` case:

```go
	case tea.KeyMsg:
		if m.structureRenaming {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.structureRenaming = false
				m.nameInput.Blur()
				return m, nil
			case "enter":
				m.structureRenaming = false
				m.nameInput.Blur()
				if t := strings.TrimSpace(m.nameInput.Value()); t != "" && m.structureSel < len(m.structureItems) {
					m.structureItems[m.structureSel].Title = t
					m.structureDirty = true
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(msg)
			return m, cmd
		}
		switch msg.String() {
```

Then add these cases to the normal switch (alongside up/down/esc):

```go
		case "J", "shift+down":
			if m.structureSel < len(m.structureItems)-1 {
				i := m.structureSel
				m.structureItems[i], m.structureItems[i+1] = m.structureItems[i+1], m.structureItems[i]
				m.structureSel++
				m.structureDirty = true
			}
		case "K", "shift+up":
			if m.structureSel > 0 {
				i := m.structureSel
				m.structureItems[i], m.structureItems[i-1] = m.structureItems[i-1], m.structureItems[i]
				m.structureSel--
				m.structureDirty = true
			}
		case "x":
			if m.structureSel < len(m.structureItems) {
				f := m.structureItems[m.structureSel].File
				delete(m.structurePendingNew, f) // if it was a not-yet-created new chapter, forget it
				m.structureItems = append(m.structureItems[:m.structureSel], m.structureItems[m.structureSel+1:]...)
				if m.structureSel >= len(m.structureItems) && m.structureSel > 0 {
					m.structureSel--
				}
				m.structureDirty = true
			}
		case "r":
			if m.structureSel < len(m.structureItems) {
				m.structureRenaming = true
				m.nameInput.SetValue(m.structureItems[m.structureSel].Title)
				m.nameInput.CursorEnd()
				m.nameInput.Focus()
				return m, textinput.Blink
			}
```

Import `github.com/charmbracelet/bubbles/textinput` in `structure.go` (for `textinput.Blink`).

In `structureView`, render the retitle field when active — after building `body`, before the footer:

```go
	if m.structureRenaming {
		field := "retitle ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestStructure -v` then the full suite. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add structure.go structure_test.go
git commit -m "feat: structure mode buffer edits (J/K reorder, x remove, r retitle)"
```

---

### Task 3: Add (`a`) — new blank chapter or promote a loose Resource

**Files:**
- Modify: `structure.go` (`updateStructure`: the add sub-mode; a `structureAddChoices` helper; `uniqueChapterFile`); `structureView` (the add-pick overlay); `structure_test.go`

**Interfaces:**
- Consumes: `resolveManuscript` (`manifest.go`/`manuscript.go`) or `readEntries` (`outline.go`) to find loose Resources; `sectionTitle` (`project.go`); `m.structureItems`, `m.structurePendingNew`.
- Produces: `a` opens the add-pick; choosing "new blank chapter" inserts a pending-new item; choosing a Resource inserts an existing file; `func (m model) structureAddChoices() []structAdd`; `func uniqueChapterFile(dir string, taken map[string]bool) string`.

**Context:** A loose Resource = an on-disk `.md` in `structureDir` that is NOT currently in `structureItems` (nor pending-new). New items insert AFTER the cursor.

- [ ] **Step 1: Write the failing tests**

Append to `structure_test.go`:

```go
func TestStructureAddNewBlank(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = nm.(model)
	if !m.structureAdding {
		t.Fatal("a should open the add pick")
	}
	// The first choice is "new blank chapter"; enter selects it.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if len(m.structureItems) != 2 {
		t.Fatalf("new blank should insert a chapter, got %d", len(m.structureItems))
	}
	if len(m.structurePendingNew) != 1 {
		t.Fatalf("a new-blank chapter should be pending-create, got %d", len(m.structurePendingNew))
	}
	if !m.structureDirty {
		t.Fatal("add should mark dirty")
	}
}

func TestStructureAddPromoteResource(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	createManuscript(dir, "Novel", "One")
	// a loose Resource on disk, not in the manifest
	os.WriteFile(filepath.Join(dir, "extra-scene.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	choices := m.structureAddChoices()
	// choices[0] = new blank; a resource entry for extra-scene.md must exist
	foundRes := false
	resIdx := 0
	for i, c := range choices {
		if c.file == "extra-scene.md" {
			foundRes = true
			resIdx = i
		}
	}
	if !foundRes {
		t.Fatalf("the loose resource should be offered, choices=%+v", choices)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = nm.(model)
	m.structureAddSel = resIdx
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// extra-scene.md is now a chapter; it is NOT pending-new (it already exists).
	found := false
	for _, it := range m.structureItems {
		if it.File == "extra-scene.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("promoting a resource should add it to items, got %v", titles(m.structureItems))
	}
	if m.structurePendingNew["extra-scene.md"] {
		t.Fatal("a promoted existing file must NOT be pending-create")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'StructureAdd' -v`
Expected: FAIL — `a` unhandled / `structureAddChoices` undefined.

- [ ] **Step 3: Implement the add sub-mode**

Add to `structure.go`:

```go
// structAdd is one row in the add pick: the "new blank chapter" sentinel (file == "") or an
// existing loose Resource file.
type structAdd struct {
	file  string // "" = new blank chapter
	label string
}

// structureAddChoices is [new blank chapter] followed by the manuscript's loose Resources (on-disk
// .md not currently listed in the buffer nor pending-new), de-slug-titled.
func (m model) structureAddChoices() []structAdd {
	out := []structAdd{{file: "", label: "＋ new blank chapter"}}
	listed := map[string]bool{}
	for _, it := range m.structureItems {
		listed[it.File] = true
	}
	for _, e := range readEntries(m.structureDir) { // non-hidden document files
		if listed[e.name] {
			continue
		}
		out = append(out, structAdd{file: e.name, label: "◦ " + sectionTitle(e.name)})
	}
	return out
}

// uniqueChapterFile returns a filename not present on disk in dir and not already taken by the
// buffer/pending set — "untitled.md", "untitled-2.md", …
func uniqueChapterFile(dir string, taken map[string]bool) string {
	for n := 1; ; n++ {
		name := "untitled.md"
		if n > 1 {
			name = "untitled-" + strconv.Itoa(n) + ".md"
		}
		if taken[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// applyAdd inserts the chosen add option after the cursor.
func (m *model) applyAdd(c structAdd) {
	at := m.structureSel + 1
	if at > len(m.structureItems) {
		at = len(m.structureItems)
	}
	var it manifestItem
	if c.file == "" { // new blank chapter
		taken := map[string]bool{}
		for _, x := range m.structureItems {
			taken[x.File] = true
		}
		for f := range m.structurePendingNew {
			taken[f] = true
		}
		f := uniqueChapterFile(m.structureDir, taken)
		m.structurePendingNew[f] = true
		it = manifestItem{File: f, Title: "Untitled"}
	} else { // promote an existing loose Resource
		it = manifestItem{File: c.file, Title: sectionTitle(c.file)}
	}
	m.structureItems = append(m.structureItems[:at], append([]manifestItem{it}, m.structureItems[at:]...)...)
	m.structureSel = at
	m.structureDirty = true
}
```

Import `os`, `path/filepath` (already for `filepathBase`), `strconv` in `structure.go`.

In `updateStructure`, add the add sub-mode capture at the top of `tea.KeyMsg` (after the `structureRenaming` block):

```go
		if m.structureAdding {
			choices := m.structureAddChoices()
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.structureAdding = false
				return m, nil
			case "up", "k":
				if m.structureAddSel > 0 {
					m.structureAddSel--
				}
			case "down", "j":
				if m.structureAddSel < len(choices)-1 {
					m.structureAddSel++
				}
			case "enter":
				if m.structureAddSel < len(choices) {
					m.applyAdd(choices[m.structureAddSel])
				}
				m.structureAdding = false
			}
			return m, nil
		}
```

And the `a` case in the normal switch:

```go
		case "a":
			m.structureAdding = true
			m.structureAddSel = 0
```

In `structureView`, render the add-pick overlay when active — replace the footer region with the pick when `m.structureAdding`:

```go
	if m.structureAdding {
		var picks []string
		for i, c := range m.structureAddChoices() {
			label := c.label
			if i == m.structureAddSel {
				label = selectedStyle.Render(label)
			}
			picks = append(picks, label)
		}
		pick := framedPanel("add", strings.Join(picks, "\n"), max(30, min(m.width-8, 40)), len(picks)+2, "")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pick))
		return b.String()
	}
```

(Place this block right before the existing footer write in `structureView`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestStructure -v` then the full suite. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add structure.go structure_test.go
git commit -m "feat: structure mode add (new blank chapter or promote a loose resource)"
```

---

### Task 4: Commit on exit (`esc` → confirm → write)

**Files:**
- Modify: `structure.go` (`updateStructure`: the confirm sub-mode + `commitStructure`); `structureView` (the confirm bar); `structure_test.go`

**Interfaces:**
- Consumes: `atomicWrite`, `readManifest`, `writeManifest` (`manifest.go`); `m.structureItems`, `m.structurePendingNew`, `m.structureDir`.
- Produces: `esc` with edits opens a confirm; `y` commits (creates pending files + one `writeManifest`); `n`/`esc` discards; `func (m *model) commitStructure() error`.

- [ ] **Step 1: Write the failing tests**

Append to `structure_test.go`:

```go
func TestStructureCommitWritesReorderAndCreatesPending(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	writeManifest(dir, manifest{SchemaVersion: 1, Title: "Novel", Items: []manifestItem{
		{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "B"},
	}})
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("y"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()

	// Swap A and B (K on the 2nd), add a new blank at the end, then commit.
	m.structureSel = 1
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}}) // [B, A]
	m = nm.(model)
	m.structureSel = 1
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // new blank after A → [B, A, Untitled]
	m = nm.(model)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // dirty → confirm
	m = nm.(model)
	if !m.structureConfirm {
		t.Fatal("esc with edits should open the commit confirm")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}) // commit
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("commit should return to the binder, screen=%v", m.screen)
	}
	got, _, _ := readManifest(dir)
	if titlesJoined(got.Items) != "B|A|Untitled" {
		t.Fatalf("manifest order after commit = %v", titles(got.Items))
	}
	// The new blank chapter's file was created on disk.
	last := got.Items[len(got.Items)-1].File
	if _, err := os.Stat(filepath.Join(dir, last)); err != nil {
		t.Fatalf("the new blank chapter's file should exist: %v", err)
	}
}

func TestStructureCancelDiscards(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "novel")
	writeManifest(dir, manifest{SchemaVersion: 1, Title: "Novel", Items: []manifestItem{
		{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "B"},
	}})
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.outline.dir = dir
	m.enterStructure()
	m.structureSel = 1
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // confirm
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // esc in the confirm = discard
	m = nm.(model)
	got, _, _ := readManifest(dir)
	if titlesJoined(got.Items) != "A|B" {
		t.Fatalf("cancel must NOT change the manifest, got %v", titles(got.Items))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'StructureCommit|StructureCancel' -v`
Expected: FAIL — `esc` with edits exits instead of confirming; `structureConfirm` never set.

- [ ] **Step 3: Implement confirm + commit**

Replace the plain `esc` case in `updateStructure`'s normal switch with a dirty-aware one, and add the confirm sub-mode capture (place the confirm block right after the `structureAdding` block, before the normal switch):

```go
		if m.structureConfirm {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				if err := m.commitStructure(); err != nil {
					m.status = "commit failed: " + err.Error()
					m.structureConfirm = false
					return m, nil
				}
				m.structureConfirm = false
				m.exitStructure()
				m.status = "structure saved"
				return m, nil
			case "esc", "n":
				m.structureConfirm = false
				m.exitStructure()
				m.status = "structure changes discarded"
				return m, nil
			}
			return m, nil
		}
```

Change the normal `esc` case to:

```go
		case "esc":
			if m.structureDirty {
				m.structureConfirm = true
			} else {
				m.exitStructure()
			}
			return m, nil
```

Add `commitStructure`:

```go
// commitStructure writes the staged order/membership: it creates any pending new-blank files, then
// persists the whole buffer via the atomic writeManifest (re-reading the on-disk title first).
func (m *model) commitStructure() error {
	for f := range m.structurePendingNew {
		// only create files that survived to the final buffer
		inBuf := false
		for _, it := range m.structureItems {
			if it.File == f {
				inBuf = true
				break
			}
		}
		if !inBuf {
			continue
		}
		if err := atomicWrite(filepath.Join(m.structureDir, f), []byte(""), 0o644); err != nil {
			return err
		}
	}
	title := m.structureTitle() // re-reads the on-disk manifest title (read-modify-write)
	return writeManifest(m.structureDir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         m.structureItems,
	})
}
```

In `structureView`, render the confirm bar when active (before the footer, similar to the add overlay):

```go
	if m.structureConfirm {
		msg := "Apply changes to " + m.structureTitle() + "?  y apply · esc cancel"
		bar := lipgloss.NewStyle().Foreground(accent).Render(msg)
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestStructure -v` then the full suite. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add structure.go structure_test.go
git commit -m "feat: structure mode commit-on-exit (confirm; create pending + writeManifest, or discard)"
```

---

### Task 5: Shared-contract flip (both repos) + binder hint

**Files:**
- Modify: `CLAUDE.md` (okashi) — SHARED CONTRACTS §1 + the Project-model rename bullet; `main.go` (the binder status line mentions `s`)
- Modify (inkmere): `docs/superpowers/specs/2026-06-27-multi-source-library-design.md`, `docs/superpowers/specs/2026-06-27-project-model-design.md`, `CLAUDE.md`

**Interfaces:**
- Consumes: nothing (docs + a status-string tweak).
- Produces: the contract reflects okashi as a *shipped* structural editor (confirm-gated).

- [ ] **Step 1: Update okashi `CLAUDE.md` §1**

Find the §1 "Authority (revised 2026-06-30)" line that says structural edits (reorder/insert/move) are "planned behind a **confirmation** (structuring mode / file mover)." Change "planned behind" → "shipped behind" and note structure mode is live (reorder/insert/remove via `s` in the binder, commit-on-exit confirm). Keep the schema HARD-GATE unchanged. Update the Project-model manuscript bullet's "structural edits ... planned behind a confirmation" the same way.

- [ ] **Step 2: Add the binder hint**

In `main.go`, the binder status line (search for `"binder · ↑↓ select · enter open · r rename · m read`) — append ` · s structure` so the entry point is discoverable.

- [ ] **Step 3: Mirror in inkmere**

In the three inkmere files, change the reciprocal "okashi ... structural edits planned behind a confirmation" wording to "shipped (structure mode; reorder/insert/remove, commit-on-exit confirm)." Keep it aligned with okashi's wording. No wicklight code change; verify wicklight reloads an okashi structural write (its file-presenter + per-source index rebuild).

- [ ] **Step 4: Verify build + full suite still green** (docs + a one-line status change only)

Run: `/opt/homebrew/bin/go build ./...` and `/opt/homebrew/bin/go test ./...`. Both pass.

- [ ] **Step 5: Commit (okashi) and commit (inkmere) separately**

```bash
git -C /Users/michael/dev/okashi add CLAUDE.md main.go
git -C /Users/michael/dev/okashi commit -m "docs: structure mode ships — structural edits confirm-gated (mirror in inkmere)"
git -C /Users/michael/dev/inkmere add docs/superpowers/specs/2026-06-27-multi-source-library-design.md docs/superpowers/specs/2026-06-27-project-model-design.md CLAUDE.md
git -C /Users/michael/dev/inkmere commit -m "docs: okashi ships structural edits (structure mode, confirm-gated) — mirror okashi §1"
```

---

## Self-Review

**Spec coverage (against `2026-07-01-structural-editing-file-mover-design.md` §2 + §0):**
- Enter from the binder with `s`, manifest manuscripts only → Task 1. ✅
- `J`/`K` reorder, `x` remove (demote), `r` retitle (buffer) → Task 2. ✅
- `a` add: new blank chapter OR promote a loose Resource, inserted at the cursor → Task 3. ✅
- Staged; `esc` → confirm → single atomic write; create pending new files; cancel discards → Task 4. ✅
- Contract flip planned→shipped, both repos + binder hint → Task 5. ✅ (§0)
- NOT here (chunk 3): the file mover.

**Placeholder scan:** every step has complete code. Two spots defer to "reuse the existing helper if present" (`ansiStrip`/`typeInto`/a zero-pad-2 formatter) with an explicit instruction to grep first and only add what's missing — this is correct (avoid duplicate symbols), not a placeholder; the fallback code is provided.

**Type consistency:** model fields (`structureItems`/`structureSel`/`structurePendingNew`/`structureDirty`/`structureAdding`/`structureAddSel`/`structureRenaming`/`structureConfirm`) are defined in Task 1 and consumed by Tasks 2-4. `structureAddChoices() []structAdd`, `applyAdd(structAdd)`, `uniqueChapterFile(dir, taken)`, `commitStructure() error`, `exitStructure()`, `structureTitle()` are each defined where first used. `manifestItem{File,Title}` is the shared shape. The commit builds `manifest{schemaVersion:1, title, items}` and calls the existing `writeManifest` (no hand-marshal).

**Open follow-through for the executor:** structure mode reuses `m.nameInput` for the retitle sub-mode (like `home.go`'s add-source prompt) — confirm no other active prompt flag is set on `screenStructure`. The rendering (framedPanel widths, overlays) is string-assertable; the controller should eyeball a `structureView()` dump between tasks to confirm the layout reads well.
