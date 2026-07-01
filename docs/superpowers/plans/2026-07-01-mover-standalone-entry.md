# File Mover — Standalone Entry (left-pane source picker) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the file mover a self-contained full-screen tool: a **left pane that browses and picks the file/folder to move** (source), then the existing right-pane destination browser + confirm. Reachable from a discoverable **"Move files" home action** (plus the existing contextual `Shift+M` in the sidebar).

**Architecture:** Add a two-phase model to the existing `mover.go` — `moverPickSource` (left pane is a source browser) → `moverPickDest` (the current right-pane destination flow). The left picker mirrors the right browser's row model (`..`, `▸ folders`, and now `› files` + a `→ move this folder` row). The contextual entry (`enterMover`) starts at `pickDest` with the source pre-set; a new `enterMoverStandalone` starts at `pickSource`. All actual moves still go through chunk-1 `moveDocument`/`moveFolder`.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `mover.go`, `move.go`, `home.go` (home actions), `activeSourceRoot()`, `withinRoot`, `homeWindowOffset`, `framedPanel`.

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`**, gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **All moves still go ONLY through chunk-1 `moveDocument`/`moveFolder`.** This plan adds source-picking UI, not new move logic.
- **`..` in BOTH panes is bounded to `activeSourceRoot()`** (via `withinRoot`) — neither browser escapes the active source.
- **Both panes window with `homeWindowOffset`** (O(visible)).
- **Reliable keys only:** entries are the home "Move files" action + the existing sidebar `Shift+M`. Do NOT add `ctrl+m`/`ctrl+shift+m` (terminal-ambiguous with Enter).
- **Default build stays pure-Go;** no new dependency.

---

### Task 1: Two-phase mover — left-pane source picker + standalone entry

**Files:**
- Modify: `mover.go` (phase field usage, `moverEntryKind` additions, `enterMoverStandalone`, `moverSrcReload`, `updateMover` source-phase routing, `moverView` two-phase render); `main.go` (the `moverPhase` + source-browser model fields)
- Test: `mover_test.go` (append)

**Interfaces:**
- Consumes: `activeSourceRoot()`, `withinRoot`, `homeWindowOffset`, `framedPanel`; existing mover fields/functions.
- Produces:
  - model fields `moverPhase int`, `moverSrcDir string`, `moverSrcEntries []moverEntry`, `moverSrcSel int`
  - `const moverPickSource = 0`, `moverPickDest = 1`
  - `moverEntryKind` values `moverFile`, `moverMoveThis` (added to the existing `moverMoveHere`/`moverUp`/`moverFolder`)
  - `func (m *model) enterMoverStandalone()`, `func (m *model) moverSrcReload()`, `func (m *model) pickMoverSource(e moverEntry)`

- [ ] **Step 1: Write the failing tests**

Append to `mover_test.go`:

```go
func TestMoverStandalonePicksFileThenDest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "stray.md"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(root, "research"), 0o755)
	os.WriteFile(filepath.Join(root, "research", "deep.md"), []byte("y"), 0o644)

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.enterMoverStandalone()
	if m.screen != screenMover || m.moverPhase != moverPickSource {
		t.Fatalf("standalone entry should open the mover in pick-source phase, screen=%v phase=%d", m.screen, m.moverPhase)
	}
	// Left pane lists the root's folders + files.
	names := map[string]moverEntryKind{}
	for _, e := range m.moverSrcEntries {
		names[e.name] = e.kind
	}
	if names["research"] != moverFolder || names["stray.md"] != moverFile {
		t.Fatalf("source picker should list research/ (folder) + stray.md (file), got %+v", m.moverSrcEntries)
	}
	// Select stray.md as the source → advances to pick-dest with the source set.
	for i, e := range m.moverSrcEntries {
		if e.kind == moverFile && e.name == "stray.md" {
			m.moverSrcSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverPhase != moverPickDest {
		t.Fatalf("picking a file should advance to pick-dest, phase=%d", m.moverPhase)
	}
	if m.moverSource != filepath.Join(root, "stray.md") || m.moverIsDir {
		t.Fatalf("source should be stray.md, got %q isDir=%v", m.moverSource, m.moverIsDir)
	}
	if m.moverFromDir != root {
		t.Fatalf("moverFromDir should be the file's container, got %q", m.moverFromDir)
	}
}

func TestMoverStandaloneDrillAndPickFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "worldbuild", "characters"), 0o755)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = nm.(model)
	m.enterMoverStandalone()
	// drill into worldbuild
	for i, e := range m.moverSrcEntries {
		if e.kind == moverFolder && e.name == "worldbuild" {
			m.moverSrcSel = i
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // drill
	m = nm.(model)
	if m.moverSrcDir != filepath.Join(root, "worldbuild") {
		t.Fatalf("enter on a folder should drill in, srcDir=%q", m.moverSrcDir)
	}
	// A "→ move this folder" row now exists (we're below the source root); pick it.
	moveThis := -1
	for i, e := range m.moverSrcEntries {
		if e.kind == moverMoveThis {
			moveThis = i
		}
	}
	if moveThis < 0 {
		t.Fatalf("a 'move this folder' row should exist below root, got %+v", m.moverSrcEntries)
	}
	m.moverSrcSel = moveThis
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.moverPhase != moverPickDest || !m.moverIsDir || m.moverSource != filepath.Join(root, "worldbuild") {
		t.Fatalf("'move this folder' should pick worldbuild as a folder source; phase=%d isDir=%v src=%q", m.moverPhase, m.moverIsDir, m.moverSource)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run TestMoverStandalone -v`
Expected: FAIL — `enterMoverStandalone` / `moverPickSource` undefined.

- [ ] **Step 3: Add the model fields + kinds**

In `main.go`, add near the other `mover*` fields:

```go
	moverPhase      int          // moverPickSource | moverPickDest
	moverSrcDir     string       // left-pane browse dir (pick-source phase)
	moverSrcEntries []moverEntry // left-pane rows
	moverSrcSel     int
```

In `mover.go`, extend the `moverEntryKind` const block:

```go
	moverFile     // "› name" (a document — selectable as the source)
	moverMoveThis // "→ move this folder (<current>)" (pick the browsed folder as the source)
```

and add the phase consts:

```go
const (
	moverPickSource = 0
	moverPickDest   = 1
)
```

- [ ] **Step 4: Implement the source phase in `mover.go`**

```go
// enterMoverStandalone opens the mover with no source chosen — the left pane browses the active
// source so the user picks the file/folder to move (home action / global entry).
func (m *model) enterMoverStandalone() {
	m.moverPhase = moverPickSource
	m.moverSrcDir = m.activeSourceRoot()
	m.moverSrcSel = 0
	m.moverConfirm = false
	m.moverAsChapter = true
	m.moverReturn = screenWriting
	m.moverSrcReload()
	m.screen = screenMover
}

// moverSrcReload rebuilds the source-picker rows for moverSrcDir: a "move this folder" row (when
// below the source root), a ".." row (bounded to the root), subfolders (drillable), then files.
func (m *model) moverSrcReload() {
	root := m.activeSourceRoot()
	var rows []moverEntry
	below := m.moverSrcDir != root && withinRoot(m.moverSrcDir, root)
	if below {
		rows = append(rows, moverEntry{name: filepath.Base(m.moverSrcDir), path: m.moverSrcDir, kind: moverMoveThis})
		rows = append(rows, moverEntry{name: "..", path: filepath.Dir(m.moverSrcDir), kind: moverUp})
	}
	if ents, err := os.ReadDir(m.moverSrcDir); err == nil {
		var dirs, files []moverEntry
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(m.moverSrcDir, e.Name())
			if e.IsDir() {
				dirs = append(dirs, moverEntry{name: e.Name(), path: p, kind: moverFolder})
			} else if m.files.allowed[strings.ToLower(filepath.Ext(e.Name()))] {
				files = append(files, moverEntry{name: e.Name(), path: p, kind: moverFile})
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
		rows = append(rows, dirs...)
		rows = append(rows, files...)
	}
	m.moverSrcEntries = rows
	if m.moverSrcSel >= len(rows) {
		m.moverSrcSel = len(rows) - 1
	}
	if m.moverSrcSel < 0 {
		m.moverSrcSel = 0
	}
}

// pickMoverSource selects e as the thing to move (a file, or a folder via "move this folder") and
// advances to the destination phase.
func (m *model) pickMoverSource(e moverEntry) {
	switch e.kind {
	case moverFile:
		m.moverSource = e.path
		m.moverIsDir = false
		m.moverFromDir = filepath.Dir(e.path)
	case moverMoveThis:
		m.moverSource = e.path
		m.moverIsDir = true
		m.moverFromDir = filepath.Dir(e.path)
	default:
		return
	}
	m.moverPhase = moverPickDest
	m.moverDestDir = m.activeSourceRoot()
	m.moverSel = 0
	m.moverReload()
}
```

In `updateMover`, route the source phase. Immediately after the `moverConfirm` capture block and BEFORE the existing (destination) key handling, add — inside the `tea.KeyMsg` case:

```go
		if m.moverPhase == moverPickSource {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.screen = m.moverReturn
				return m, nil
			case "up", "k":
				if m.moverSrcSel > 0 {
					m.moverSrcSel--
				}
			case "down", "j":
				if m.moverSrcSel < len(m.moverSrcEntries)-1 {
					m.moverSrcSel++
				}
			case "enter":
				if m.moverSrcSel < 0 || m.moverSrcSel >= len(m.moverSrcEntries) {
					return m, nil
				}
				e := m.moverSrcEntries[m.moverSrcSel]
				switch e.kind {
				case moverUp, moverFolder:
					m.moverSrcDir = e.path
					m.moverSrcSel = 0
					m.moverSrcReload()
				case moverFile, moverMoveThis:
					m.pickMoverSource(e)
				}
			}
			return m, nil
		}
```

Also set `enterMover` (contextual) to `m.moverPhase = moverPickDest` so it skips the source phase (add that one line to the existing `enterMover`).

- [ ] **Step 5: Render both phases in `moverView`**

Update `moverView` so the LEFT pane is the active source browser in `moverPickSource`, and the chosen-source display in `moverPickDest`. Replace the left-pane construction:

```go
	// LEFT pane: the source browser (pick phase) or the chosen source (dest phase).
	var leftInner string
	var leftTitle string
	if m.moverPhase == moverPickSource {
		leftTitle = "MOVE · pick a file"
		visRows := m.height - 8
		if visRows < 1 {
			visRows = 1
		}
		off := homeWindowOffset(len(m.moverSrcEntries), m.moverSrcSel, visRows)
		var lines []string
		for i := off; i < len(m.moverSrcEntries) && len(lines) < visRows; i++ {
			e := m.moverSrcEntries[i]
			var text string
			switch e.kind {
			case moverMoveThis:
				text = "→ move this folder"
			case moverUp:
				text = "‹ .."
			case moverFolder:
				text = "▸ " + e.name + "/"
			default:
				text = "› " + e.name
			}
			if i == m.moverSrcSel {
				text = selectedStyle.Render(text)
			}
			lines = append(lines, text)
		}
		leftInner = strings.Join(lines, "\n")
	} else {
		leftTitle = "MOVE"
		kindLabel := "file"
		if m.moverIsDir {
			kindLabel = "folder"
		}
		leftInner = "moving " + kindLabel + ":\n" + filepath.Base(m.moverSource) + "\n\nfrom: " + filepath.Base(m.moverFromDir)
	}
	leftPanel := framedPanel(leftTitle, leftInner, max(26, min(m.width-34, 40)), max(len(strings.Split(leftInner, "\n"))+2, 8), "")
```

The RIGHT pane stays the destination browser (unchanged) — but in `moverPickSource` it should be a dim placeholder. Guard the right-pane build: when `m.moverPhase == moverPickSource`, set the right inner to `homeDim("pick a source first →")` (reuse `homeDim` from home.go) and skip windowing. Keep the confirm/footer logic. Update the footer to reflect the phase:

```go
	footText := "↑↓ browse · enter drill/select · esc cancel"
```

(That footer already reads correctly for both phases.)

- [ ] **Step 6: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestMover -v` then the full suite. vet + gofmt clean. Confirm the contextual mover tests from chunk 3 still pass (their `enterMover` now sets `moverPhase = moverPickDest`).

- [ ] **Step 7: Commit**

```bash
git add mover.go main.go mover_test.go
git commit -m "feat: standalone mover — left pane picks the source file/folder (two-phase)"
```

---

### Task 2: Discoverable entries — home "Move files" action + sidebar hint

**Files:**
- Modify: `home.go` (`homeKind` add `homeMoveFiles`; `buildHomeItems` add the action; `openHomeSelection` handle it); `main.go` (the sidebar status hint)
- Test: `home_test.go` (append)

**Interfaces:**
- Consumes: `enterMoverStandalone` (Task 1); the home actions machinery.
- Produces: a "Move files" action on the home screen; the sidebar footer advertises `M move`.

- [ ] **Step 1: Write the failing test**

Append to `home_test.go`:

```go
func TestHomeMoveFilesActionOpensStandaloneMover(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = nm.(model)
	// find the Move-files action and open it.
	acts := homeGroupsActions(m.homeItems)
	found := false
	for _, a := range acts {
		if a.kind == homeMoveFiles && a.label == "Move files" {
			found = true
		}
	}
	if !found {
		t.Fatalf("home actions should include 'Move files', got %+v", acts)
	}
	m.homeItems = []homeItem{{kind: homeMoveFiles, label: "Move files"}}
	m.resetHomeSelection()
	m.focusAt(regionActions, 0)
	cmd := m.openHomeSelection()
	_ = cmd
	if m.screen != screenMover || m.moverPhase != moverPickSource {
		t.Fatalf("the Move-files action should open the standalone mover, screen=%v phase=%d", m.screen, m.moverPhase)
	}
}
```

(`homeGroupsActions` is the existing test helper in `home_test.go`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestHomeMoveFiles -v`
Expected: FAIL — `homeMoveFiles` undefined.

- [ ] **Step 3: Add the action**

In `home.go`, add `homeMoveFiles` to the `homeKind` const block (near `homeOpenOther`). In `buildHomeItems`, add it alongside Browse:

```go
	items = append(items,
		homeItem{kind: homeMoveFiles, label: "Move files"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
```

In `openHomeSelection`'s actions branch, handle it:

```go
		if acts[m.homeIndex].kind == homeMoveFiles {
			m.enterMoverStandalone()
			return nil
		}
		if acts[m.homeIndex].kind == homeOpenOther {
			// ... existing Browse handling ...
		}
```

(Adjust to the existing structure — the actions branch currently checks `kind == homeOpenOther`; add the `homeMoveFiles` check before it.)

- [ ] **Step 4: Add the sidebar hint**

In `main.go`, find the sidebar/file-pane status or help text that lists sidebar keys (search for `ctrl+n` / the sidebar hint) and add `· M move` so the contextual entry is discoverable. If there is no persistent sidebar hint, add `M   move file/folder` to the `helpText` (F1 cheatsheet) block.

- [ ] **Step 5: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'HomeMoveFiles|Mover|Home' -v` then the full suite. vet + gofmt clean. (The action-count changes from 1 to 2 — update any test asserting exactly one home action, e.g. `TestBuildHomeItemsHasBrowseAction`, to expect `Move files` + `Browse all files`.)

- [ ] **Step 6: Commit**

```bash
git add home.go main.go home_test.go
git commit -m "feat: 'Move files' home action + sidebar M hint (discoverable mover entry)"
```

---

## Self-Review

**Spec coverage (against the user's approved direction + spec §3 standalone entry):**
- Left pane picks the source (browse: drill folders, `enter` on a file, `→ move this folder` for a folder, `..` bounded) → Task 1. ✅
- Two-phase: pick-source → pick-dest (existing destination flow + confirm/apply unchanged) → Task 1. ✅
- Contextual `Shift+M` still works (source pre-set, starts at pick-dest) → Task 1 (`enterMover` sets `moverPickDest`). ✅
- Discoverable entries: home "Move files" action + sidebar hint → Task 2. ✅
- Reliable keys only (no `ctrl+shift+m`); home action is the primary discoverable entry → design + Task 2. ✅
- NOT here (unchanged fast-follows): cross-source destinations, persistent move-error UX.

**Placeholder scan:** every step has complete code; the two `moverView`/`openHomeSelection` edits say "adjust to the existing structure" with the exact surrounding condition named — that's a splice instruction, not a placeholder (the code to insert is given).

**Type consistency:** `moverPhase`/`moverSrcDir`/`moverSrcEntries`/`moverSrcSel` (Task 1 model fields) consumed by `moverSrcReload`/`updateMover`/`moverView`. `moverEntryKind` gains `moverFile`/`moverMoveThis`; `pickMoverSource` sets `moverSource`/`moverIsDir`/`moverFromDir` then `moverReload()` (the existing dest reload). `enterMoverStandalone` (Task 1) consumed by the home action (Task 2). `homeMoveFiles` kind routed through `buildHomeItems`/`openHomeSelection`.
