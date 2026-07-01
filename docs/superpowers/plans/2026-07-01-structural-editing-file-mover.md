# Structural Editing — Chunk 3: File Mover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A full-page **file mover** (`screenMover`) — invoked with `M` on a selected file/folder in the editor's file pane — with a left "source" pane and a right destination-folder browser; picking a destination confirms (offering chapter/resource when a file lands in a manuscript) and applies the move via the chunk-1 `moveDocument`/`moveFolder` engine.

**Architecture:** A new `mover.go` holds the mover state machine. The right pane is a drillable folder browser rooted at the active source (`activeSourceRoot()`), bounded so `..` can't escape the corpus; a leading "→ move into <folder>" row commits the current folder as the destination. The confirm branches on source-type × dest-type and calls `moveDocument`/`moveFolder` (already atomic + manifest-safe from chunk 1). Contextual entry only in this chunk; **standalone entry** (pick the source in the left pane) and **cross-source destinations** are noted fast-follows.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `move.go` (`moveDocument`/`moveFolder`), `manifest.go` (`hasManifest`), `m.files` (the file pane), `activeSourceRoot()`, `framedPanel`, `homeWindowOffset`, lipgloss.

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`**, gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **All moves go through chunk-1 `moveDocument`/`moveFolder`** (atomic, read-modify-write, manifest-safe). The mover NEVER touches the filesystem or manifests directly — it only chooses arguments and calls those two.
- **`..` is bounded to `activeSourceRoot()`** — the destination browser never escapes the active source's tree.
- **Confirm before every apply** (§0 confirm-gate). A file into a manuscript offers **chapter / resource**; everything else is a plain confirm.
- **`View()` stays O(visible):** window the destination list with `homeWindowOffset` (mirror structure mode / the pager).
- **Refuse is delegated:** collisions / no-ops / unreadable manifests are already refused inside `moveDocument`/`moveFolder`; surface their error as a status, don't pre-check redundantly.
- **Default build stays pure-Go;** no new dependency.
- **Scoped out of this chunk (noted, not built):** standalone entry (source picked in the left pane), cross-source destinations (a source switch in the right pane). The model leaves room but the plan builds contextual-entry + same-source only.

---

### Task 1: Enter from the file pane + two-pane render + destination navigation

**Files:**
- Create: `mover.go`
- Modify: `main.go` — `screenMover` const; model fields; `Update`/`View` dispatch; the `M` key in the writing-screen sidebar key handling.
- Test: `mover_test.go` (create)

**Interfaces:**
- Consumes: `m.files.entries`, `m.files.selected`, `m.files.dir`, `m.files.root` (`filelist`); `activeSourceRoot()`; `hasManifest` (`manifest.go`); `framedPanel`, `homeWindowOffset`; `withinRoot` (`filelist.go`).
- Produces:
  - `const screenMover`
  - model fields `moverSource string`, `moverIsDir bool`, `moverFromDir string`, `moverDestDir string`, `moverEntries []moverEntry`, `moverSel int`, `moverConfirm bool`, `moverAsChapter bool`, `moverReturn screen`
  - `type moverEntry struct { name, path string; kind moverEntryKind }` with `moverMoveHere`/`moverUp`/`moverFolder`
  - `func (m *model) enterMover()`, `func (m *model) moverReload()`, `func (m model) updateMover(msg tea.Msg) (tea.Model, tea.Cmd)`, `func (m model) moverView() string`

- [ ] **Step 1: Write the failing tests**

Create `mover_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// moverModelAt builds a model whose file pane is at root with `sel` selected.
func moverModelAt(t *testing.T, root string, selName string) model {
	t.Helper()
	t.Setenv("OKASHI_DIR", root)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.files.root = root
	m.files.SetDir(root)
	m.files.selectName(selName)
	return m
}

func TestMoverEnterFromFilePane(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	if m.screen != screenMover {
		t.Fatalf("M should enter the mover, screen=%v", m.screen)
	}
	if m.moverSource != filepath.Join(root, "stray.md") || m.moverIsDir {
		t.Fatalf("source should be the selected file, got %q isDir=%v", m.moverSource, m.moverIsDir)
	}
	// The destination browser lists a "move into" row + the subfolder(s).
	out := ansi.Strip(m.moverView())
	if !strings.Contains(out, "move into") || !strings.Contains(out, "research") {
		t.Fatalf("mover view should show the destination browser (move-into + subfolders):\n%s", out)
	}
}

func TestMoverDrillIntoSubfolderAndBack(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research", "deep"), 0o755)
	m := moverModelAt(t, root, "stray.md")
	m.enterMover()
	// entries: [move-here, ▸ research]  (no ".." at the source root)
	// select "research" and drill in.
	researchIdx := -1
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "research" {
			researchIdx = i
		}
	}
	if researchIdx < 0 {
		t.Fatalf("research folder should be listed: %+v", m.moverEntries)
	}
	m.moverSel = researchIdx
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into research
	m = nm.(model)
	if m.moverDestDir != filepath.Join(root, "research") {
		t.Fatalf("drilling should move destDir into research, got %q", m.moverDestDir)
	}
	// Now there is a ".." row and a "deep" subfolder.
	hasUp, hasDeep := false, false
	for _, e := range m.moverEntries {
		if e.kind == moverUp {
			hasUp = true
		}
		if e.kind == moverFolder && e.name == "deep" {
			hasDeep = true
		}
	}
	if !hasUp || !hasDeep {
		t.Fatalf("drilled browser should show '..' + 'deep', got %+v", m.moverEntries)
	}
	// Select ".." and go back to root; ".." must not escape the source root.
	for i, e := range m.moverEntries {
		if e.kind == moverUp {
			m.moverSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverDestDir != root {
		t.Fatalf("'..' should return to root, got %q", m.moverDestDir)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run TestMover -v`
Expected: FAIL — `undefined: screenMover` / `enterMover`.

- [ ] **Step 3: Add the screen const + model fields + dispatch + the `M` key**

In `main.go`: add `screenMover` to the screen iota block (after `screenStructure`). Add the model fields listed in Interfaces (near the `structure*` fields). Add `Update`/`View` dispatch:

```go
	if m.screen == screenMover {
		return m.updateMover(msg)
	}
```
```go
	if m.screen == screenMover {
		return m.moverView()
	}
```

Wire the `M` key. In the writing-screen sidebar key block (`main.go` ~1283–1286 — the switch with `case "r": m.startRename()` and `case "d": m.duplicateSelected()`, inside the `focus == focusSidebar` guard), add a sibling case. Match the surrounding cases, which do NOT `return` (they mutate `m` and fall through):

```go
				case "M":
					m.enterMover()
```

`enterMover` switches `m.screen = screenMover`, so the next frame renders the mover. Keep it under the same `focus == focusSidebar` guard so `M` never fires while editing prose.

- [ ] **Step 4: Create `mover.go`**

```go
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type moverEntryKind int

const (
	moverMoveHere moverEntryKind = iota // "→ move into <current folder>"
	moverUp                             // "‹ .."
	moverFolder                         // "▸ name/"
)

type moverEntry struct {
	name, path string
	kind       moverEntryKind
}

// enterMover opens the file mover for the file pane's selected entry (contextual entry). The
// destination browser starts at the active source root.
func (m *model) enterMover() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	m.moverSource = filepath.Join(m.files.dir, e.name)
	m.moverIsDir = e.isDir
	m.moverFromDir = m.files.dir
	m.moverDestDir = m.activeSourceRoot()
	m.moverSel = 0
	m.moverConfirm = false
	m.moverAsChapter = true
	m.moverReturn = screenWriting
	m.moverReload()
	m.screen = screenMover
}

// moverReload rebuilds the destination browser rows for moverDestDir: a leading "move here" row,
// a ".." row when below the active source root, then the subfolders (alpha-sorted).
func (m *model) moverReload() {
	root := m.activeSourceRoot()
	var rows []moverEntry
	rows = append(rows, moverEntry{name: filepath.Base(m.moverDestDir), path: m.moverDestDir, kind: moverMoveHere})
	if m.moverDestDir != root && withinRoot(m.moverDestDir, root) {
		rows = append(rows, moverEntry{name: "..", path: filepath.Dir(m.moverDestDir), kind: moverUp})
	}
	if ents, err := os.ReadDir(m.moverDestDir); err == nil {
		var dirs []moverEntry
		for _, e := range ents {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, moverEntry{name: e.Name(), path: filepath.Join(m.moverDestDir, e.Name()), kind: moverFolder})
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
		rows = append(rows, dirs...)
	}
	m.moverEntries = rows
	if m.moverSel >= len(rows) {
		m.moverSel = len(rows) - 1
	}
	if m.moverSel < 0 {
		m.moverSel = 0
	}
}

func (m model) updateMover(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = m.moverReturn
			return m, nil
		case "up", "k":
			if m.moverSel > 0 {
				m.moverSel--
			}
		case "down", "j":
			if m.moverSel < len(m.moverEntries)-1 {
				m.moverSel++
			}
		case "enter":
			if m.moverSel < 0 || m.moverSel >= len(m.moverEntries) {
				return m, nil
			}
			e := m.moverEntries[m.moverSel]
			switch e.kind {
			case moverUp, moverFolder:
				m.moverDestDir = e.path
				m.moverSel = 0
				m.moverReload()
			case moverMoveHere:
				// commit target = moverDestDir → confirm (Task 2)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m model) moverView() string {
	// LEFT: the item being moved.
	srcName := filepath.Base(m.moverSource)
	kindLabel := "file"
	if m.moverIsDir {
		kindLabel = "folder"
	}
	left := "moving " + kindLabel + ":\n" + srcName + "\n\nfrom: " + filepath.Base(m.moverFromDir)
	leftPanel := framedPanel("MOVE", left, 26, 8, "")

	// RIGHT: the destination browser (windowed).
	visRows := m.height - 8
	if visRows < 1 {
		visRows = 1
	}
	off := homeWindowOffset(len(m.moverEntries), m.moverSel, visRows)
	var rows []string
	for i := off; i < len(m.moverEntries) && len(rows) < visRows; i++ {
		e := m.moverEntries[i]
		var text string
		switch e.kind {
		case moverMoveHere:
			text = "→ move into " + e.name + "/"
		case moverUp:
			text = "‹ .."
		default:
			text = "▸ " + e.name + "/"
		}
		if i == m.moverSel {
			text = selectedStyle.Render(text)
		}
		rows = append(rows, text)
	}
	rightW := max(30, min(m.width-30, 44))
	rightPanel := framedPanel("TO "+filepath.Base(m.moverDestDir), strings.Join(rows, "\n"), rightW, len(rows)+2, "")

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ browse · enter drill/select · esc cancel")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestMover -v` then the full suite. vet + gofmt clean.

- [ ] **Step 6: Commit**

```bash
git add mover.go main.go mover_test.go
git commit -m "feat: file mover skeleton (enter from file pane, two-pane view, destination browser)"
```

---

### Task 2: Confirm + apply the move

**Files:**
- Modify: `mover.go` (`updateMover`: the confirm sub-mode + `applyMove`; the `moverMoveHere` branch opens the confirm); `moverView` (the confirm bar with the chapter/resource radio); `mover_test.go`
- Test: `mover_test.go` (append)

**Interfaces:**
- Consumes: `moveDocument(srcDir, file, dstDir, asChapter)`, `moveFolder(srcDir, dstParent)` (`move.go`); `hasManifest` (`manifest.go`); `m.files.SetDir`.
- Produces: `func (m *model) applyMove() error`; the confirm sub-mode (`moverConfirm`, `moverAsChapter` toggle); apply + refresh + return.

**Context:** On "move here", if the source is a FILE and the destination is a MANUSCRIPT (`hasManifest(moverDestDir)`), the confirm shows a chapter/resource radio (toggle with left/right); else a plain confirm. Apply: a folder → `moveFolder`; a file → `moveDocument(moverFromDir, base(src), moverDestDir, asChapter)` (asChapter forced false when the dest isn't a manuscript).

- [ ] **Step 1: Write the failing tests**

Append to `mover_test.go`:

```go
func TestMoverMoveFileIntoManuscriptAsChapter(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "scene.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Opening")
	m := moverModelAt(t, root, "scene.md")
	m.enterMover()
	// drill into novel, then "move here" as a chapter.
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "novel" {
			m.moverSel = i
		}
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into novel
	m = nm.(model)
	m.moverSel = 0 // the "move here" row
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // → confirm
	m = nm.(model)
	if !m.moverConfirm {
		t.Fatal("moving a file into a manuscript should open the confirm")
	}
	if !m.moverAsChapter {
		t.Fatal("the confirm should default to 'chapter'")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}) // apply
	m = nm.(model)
	// scene.md moved into novel and was appended as a chapter.
	if _, err := os.Stat(filepath.Join(proj, "scene.md")); err != nil {
		t.Fatalf("file should have moved into the manuscript: %v", err)
	}
	mf, _, _ := readManifest(proj)
	last := mf.Items[len(mf.Items)-1]
	if last.File != "scene.md" {
		t.Fatalf("file should be appended as a chapter, items=%+v", mf.Items)
	}
	if m.screen != screenWriting {
		t.Fatalf("after a move the mover should return, screen=%v", m.screen)
	}
}

func TestMoverMoveFolder(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "worldbuild"), 0o755)
	os.MkdirAll(filepath.Join(root, "trilogy"), 0o755)
	m := moverModelAt(t, root, "worldbuild")
	m.enterMover()
	if !m.moverIsDir {
		t.Fatal("source should be a folder")
	}
	for i, e := range m.moverEntries {
		if e.kind == moverFolder && e.name == "trilogy" {
			m.moverSel = i
		}
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill into trilogy
	m = nm.(model)
	m.moverSel = 0                                    // move here
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})  // folder → plain confirm (no radio)
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "trilogy", "worldbuild")); err != nil {
		t.Fatalf("folder should have moved under trilogy: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'TestMoverMove' -v`
Expected: FAIL — "move here" does nothing (confirm never opens).

- [ ] **Step 3: Implement the confirm + apply**

Add the confirm capture at the TOP of `updateMover`'s `tea.KeyMsg` case (before the normal switch):

```go
		if m.moverConfirm {
			fileIntoManuscript := !m.moverIsDir && hasManifest(m.moverDestDir)
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "left", "right", "tab":
				if fileIntoManuscript {
					m.moverAsChapter = !m.moverAsChapter
				}
			case "y", "enter":
				if err := m.applyMove(); err != nil {
					m.status = "move failed: " + err.Error()
				} else {
					m.status = "moved " + filepath.Base(m.moverSource)
				}
				m.moverConfirm = false
				m.files.SetDir(m.files.dir) // refresh the pane (source may have left it)
				m.screen = m.moverReturn
				return m, nil
			case "esc", "n":
				m.moverConfirm = false
				return m, nil
			}
			return m, nil
		}
```

Fill in the `moverMoveHere` branch in the normal `enter` switch (replace the empty comment):

```go
			case moverMoveHere:
				m.moverConfirm = true
```

Add `applyMove`:

```go
// applyMove performs the chosen move via the chunk-1 engine. A folder → moveFolder; a file →
// moveDocument (asChapter only when the destination is a manuscript AND the user chose chapter).
func (m *model) applyMove() error {
	if m.moverIsDir {
		return moveFolder(m.moverSource, m.moverDestDir)
	}
	asChapter := m.moverAsChapter && hasManifest(m.moverDestDir)
	return moveDocument(m.moverFromDir, filepath.Base(m.moverSource), m.moverDestDir, asChapter)
}
```

In `moverView`, render the confirm bar (with the radio when applicable) when `m.moverConfirm` — before the footer:

```go
	if m.moverConfirm {
		dst := filepath.Base(m.moverDestDir)
		var line string
		if !m.moverIsDir && hasManifest(m.moverDestDir) {
			chapter, resource := "( ) chapter", "( ) resource"
			if m.moverAsChapter {
				chapter = "(•) chapter"
			} else {
				resource = "(•) resource"
			}
			line = "move " + filepath.Base(m.moverSource) + " → " + dst + " as  " + chapter + "  " + resource + "   ←→ toggle · y move · esc cancel"
		} else {
			line = "move " + filepath.Base(m.moverSource) + " → " + dst + "?   y move · esc cancel"
		}
		bar := lipgloss.NewStyle().Foreground(accent).Render(line)
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
```

(Place this block right before the footer write in `moverView`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestMover -v` then the full suite. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add mover.go mover_test.go
git commit -m "feat: file mover confirm + apply (chapter/resource radio; moveDocument/moveFolder)"
```

---

## Self-Review

**Spec coverage (against `2026-07-01-structural-editing-file-mover-design.md` §3):**
- Contextual entry from the file pane (`M` on the selected entry) → Task 1. ✅
- Two-pane full-page: LEFT source, RIGHT destination folder browser (drillable, `..` bounded to the source) → Task 1. ✅
- "move here" commit → confirm; file-into-manuscript offers chapter/resource radio; folder/plain move → plain confirm → Task 2. ✅
- Apply via `moveDocument`/`moveFolder` (chunk-1, atomic + manifest-safe); refuse handled inside them → Task 2. ✅
- Windowed destination list (O(visible)) → Task 1. ✅
- NOT built (noted in Architecture + Global Constraints): standalone entry (left-pane source pick), cross-source destinations. These are fast-follows; the model fields don't preclude them.

**Placeholder scan:** every step has complete code. The `moverMoveHere` branch is intentionally an empty-comment stub in Task 1 (nav only) and filled in Task 2 — flagged in both tasks, so no task ships a dangling no-op as final.

**Type consistency:** `moverEntry{name,path,kind}` + `moverEntryKind` (Task 1) consumed by `updateMover`/`moverView`/`applyMove`. `enterMover`/`moverReload`/`updateMover`/`moverView` (Task 1); `applyMove` (Task 2). `moveDocument(srcDir, file, dstDir, asChapter)` and `moveFolder(srcDir, dstParent)` match the chunk-1 signatures exactly. `activeSourceRoot()`, `withinRoot`, `homeWindowOffset`, `framedPanel` are existing.

**Open follow-through for the executor:** confirm the `M` key lands in a sidebar-focused code path (acts on `m.files.entries[m.files.selected]`); if the writing-screen sidebar block guards on `m.focus == focusSidebar`, keep `M` under that guard so it doesn't fire while editing prose. Dump `moverView()` between tasks to confirm the two-pane layout and the confirm/radio read well.
