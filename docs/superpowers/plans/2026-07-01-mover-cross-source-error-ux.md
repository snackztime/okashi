# Mover fast-follow: cross-source + persistent error — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the file mover's destination browser reach any library source (a "Sources" level), and make move failures a persistent, visible error instead of a transient status line that closes the mover.

**Architecture:** Reuse the existing two-phase mover and the chunk-1 move engine. Cross-source browsing adds a `moverSource` row kind and a `moverDestDir == ""` "sources list" sentinel; errors gain a sticky `moverError` field surfaced in `moverView`. The engine gets a clear cross-volume message for folder moves.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea, lipgloss.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`.
- Contextual/standalone mover entry MUST still default the destination to `activeSourceRoot()` — the common same-source case is unchanged. The sources list is reached only by `..` from a source root.
- `moverView`'s render stays O(visible) (it already windows via `homeWindowOffset`).
- Move execution (`applyMove` → `moveDocument`/`moveFolder`) is unchanged; cross-source is just a different absolute path. No manifest-shape change.
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.
- `source` has fields `{ID, Name, Kind, Path}`; `s.root()` returns the absolute root; `s.reachable()` checks it exists. `withinRoot(dir, root)` reports whether dir is inside root. `m.sources` is the library source list.

---

## Task 1: Clear cross-volume error for folder moves

**Files:**
- Modify: `move.go` (`moveFolder`)
- Test: `move_test.go`

**Interfaces:**
- Consumes: `os.Rename`, `syscall.EXDEV`, `errors.Is` (errors + syscall are already imported by `safeMove` in this file).
- Produces: `moveFolder` returns a clear message on a cross-volume move; same signature.

- [ ] **Step 1: Write the failing/regression test**

Add to `move_test.go`:
```go
func TestMoveFolderSameVolumeSucceeds(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "chapters")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	dstParent := filepath.Join(root, "book")
	if err := os.MkdirAll(dstParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveFolder(src, dstParent); err != nil {
		t.Fatalf("same-volume move should succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstParent, "chapters", "sub")); err != nil {
		t.Fatalf("folder not moved: %v", err)
	}
}

func TestMoveFolderCollisionRefused(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "chapters")
	os.MkdirAll(src, 0o755)
	dstParent := filepath.Join(root, "book")
	os.MkdirAll(filepath.Join(dstParent, "chapters"), 0o755) // collision
	if err := moveFolder(src, dstParent); err == nil {
		t.Fatalf("expected a collision error")
	}
}
```

- [ ] **Step 2: Run to confirm the collision test passes and the move test passes/fails as expected**

Run: `/opt/homebrew/bin/go test . -run TestMoveFolder -v`
Expected: both PASS already if `moveFolder` works same-volume (this task mainly adds the EXDEV branch below; these are regression guards).

- [ ] **Step 3: Add the EXDEV-friendly branch**

In `move.go`, change the final line of `moveFolder` from `return os.Rename(srcDir, dst)` to:
```go
	if err := os.Rename(srcDir, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("can't move a folder across volumes yet — move its files individually")
		}
		return err
	}
	return nil
```
(`errors` and `syscall` are already imported in `move.go` for `safeMove`; confirm with `head` if unsure.)

- [ ] **Step 4: Run tests + build + vet**

```
/opt/homebrew/bin/gofmt -w move.go move_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: all pass.

- [ ] **Step 5: Commit**

```
git add move.go move_test.go
git commit -m "feat(move): clear cross-volume message for folder moves"
```

---

## Task 2: Cross-source destinations — a "Sources" level

**Files:**
- Modify: `mover.go` (`moverEntryKind` const, `moverReload`, `updateMover` pick-dest enter, `moverView`; add `moverBoundingSource`)
- Test: `mover_test.go` (create if absent)

**Interfaces:**
- Consumes: `m.sources`, `source.root()`, `source.reachable()`, `withinRoot`, `activeSourceRoot`, `homeWindowOffset`, `selectedStyle`, `framedPanel`.
- Produces: `moverSource` entry kind; `func (m model) moverBoundingSource(dir string) (source, bool)`; `moverReload` supports the `moverDestDir == ""` sources list.

- [ ] **Step 1: Write the failing tests**

Append to the existing `mover_test.go` (it already has `package main` and imports `os`,
`path/filepath`, `strings`, `testing`, plus `tea "github.com/charmbracelet/bubbletea"` and
`x/ansi` — no new imports needed):
```go
// twoSourceModel builds a model with two FOLDER sources. Folder sources' root() == Path, so a
// test controls the dirs; a PRIMARY source's root() is writingDir(), which a test can't set.
func twoSourceModel(t *testing.T, a, b string) model {
	t.Helper()
	return model{sources: []source{
		{ID: "a", Name: "Writing", Kind: sourceKindFolder, Path: a},
		{ID: "b", Name: "Notes", Kind: sourceKindFolder, Path: b},
	}}
}

func TestMoverBoundingSource(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	if s, ok := m.moverBoundingSource(filepath.Join(p, "book")); !ok || s.ID != "a" {
		t.Fatalf("dir under source a should bind to a: %v %v", s, ok)
	}
	if s, ok := m.moverBoundingSource(f); !ok || s.ID != "b" {
		t.Fatalf("folder root should bind to source b: %v %v", s, ok)
	}
	if _, ok := m.moverBoundingSource("/nowhere/else"); ok {
		t.Fatalf("unrelated path should not bind")
	}
}

func TestMoverReloadSourcesList(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	m.moverDestDir = "" // the sources list
	m.moverReload()
	if len(m.moverEntries) != 2 {
		t.Fatalf("want 2 source rows, got %d", len(m.moverEntries))
	}
	for _, e := range m.moverEntries {
		if e.kind != moverSource {
			t.Fatalf("sources list should only hold moverSource rows, got kind %d", e.kind)
		}
	}
}

func TestMoverReloadUpFromSourceRootGoesToSourcesList(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	m := twoSourceModel(t, p, f)
	m.moverDestDir = p // a source root
	m.moverReload()
	if m.moverEntries[0].kind != moverMoveHere {
		t.Fatalf("first row should be move-here")
	}
	if m.moverEntries[1].kind != moverUp || m.moverEntries[1].path != "" {
		t.Fatalf("`..` at a source root must target the sources list (path \"\"), got %q", m.moverEntries[1].path)
	}
}

func TestMoverReloadUpBelowSourceRootGoesToParent(t *testing.T) {
	p, f := t.TempDir(), t.TempDir()
	sub := filepath.Join(p, "book")
	os.MkdirAll(sub, 0o755)
	m := twoSourceModel(t, p, f)
	m.moverDestDir = sub
	m.moverReload()
	if m.moverEntries[1].kind != moverUp || m.moverEntries[1].path != p {
		t.Fatalf("`..` below a source root must go to the parent %q, got %q", p, m.moverEntries[1].path)
	}
}
```
(Constant names verified: `sourceKindPrimary` / `sourceKindFolder`.)

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestMover -v`
Expected: FAIL (`moverSource` undefined / `moverBoundingSource` undefined / sources-list not handled).

- [ ] **Step 3: Add the `moverSource` kind + `moverBoundingSource`**

In `mover.go`, add `moverSource` to the `moverEntryKind` const block (after `moverMoveThis`):
```go
	moverMoveThis
	moverSource // a library source root (destination target)
```
Add the helper:
```go
// moverBoundingSource returns the reachable library source whose root contains (or equals) dir.
func (m model) moverBoundingSource(dir string) (source, bool) {
	for _, s := range m.sources {
		if !s.reachable() {
			continue
		}
		r := s.root()
		if dir == r || withinRoot(dir, r) {
			return s, true
		}
	}
	return source{}, false
}
```

- [ ] **Step 4: Rewrite `moverReload` to support the sources list**

Replace the body of `moverReload` with:
```go
func (m *model) moverReload() {
	var rows []moverEntry
	if m.moverDestDir == "" {
		for _, s := range m.sources {
			if s.reachable() {
				rows = append(rows, moverEntry{name: s.Name, path: s.root(), kind: moverSource})
			}
		}
	} else {
		rows = append(rows, moverEntry{name: filepath.Base(m.moverDestDir), path: m.moverDestDir, kind: moverMoveHere})
		if src, ok := m.moverBoundingSource(m.moverDestDir); ok {
			up := filepath.Dir(m.moverDestDir)
			if m.moverDestDir == src.root() {
				up = "" // step up to the sources list
			}
			rows = append(rows, moverEntry{name: "..", path: up, kind: moverUp})
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
	}
	m.moverEntries = rows
	if m.moverSel >= len(rows) {
		m.moverSel = len(rows) - 1
	}
	if m.moverSel < 0 {
		m.moverSel = 0
	}
}
```

- [ ] **Step 5: Handle `moverSource` in the pick-dest `enter`**

In `updateMover`, the pick-dest `case "enter"` `switch e.kind`: add `moverSource` to the drill-in case (Task 3 will add the `m.moverError = ""` line here too; for now just navigation):
```go
			case moverUp, moverFolder, moverSource:
				m.moverDestDir = e.path
				m.moverSel = 0
				m.moverReload()
```

- [ ] **Step 6: Render the sources list + header + row in `moverView`**

In `moverView`'s pick-dest right pane:
- Row text: add a `moverSource` case before the `default`:
```go
				case moverSource:
					text = "◆ " + e.name
```
- Header: replace `framedPanel("TO "+filepath.Base(m.moverDestDir), …)` with a computed title:
```go
			toTitle := "TO · SOURCES"
			if m.moverDestDir != "" {
				toTitle = "TO · " + filepath.Base(m.moverDestDir)
				if src, ok := m.moverBoundingSource(m.moverDestDir); ok {
					if m.moverDestDir == src.root() {
						toTitle = "TO · " + src.Name
					} else {
						toTitle = "TO · " + src.Name + "/" + filepath.Base(m.moverDestDir)
					}
				}
			}
			rightPanel = framedPanel(toTitle, strings.Join(rows, "\n"), rightW, len(rows)+2, "")
```
- Footer: update the non-error footer string to `"↑↓ browse · enter drill/select · .. → sources · esc cancel"`.

- [ ] **Step 7: Run tests + build + vet**

```
/opt/homebrew/bin/gofmt -w mover.go mover_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: the four `TestMover*` tests PASS; full suite green.

- [ ] **Step 8: Commit**

```
git add mover.go mover_test.go
git commit -m "feat(mover): cross-source destinations via a Sources level"
```

---

## Task 3: Persistent move-error UX

**Files:**
- Modify: `main.go` (model field `moverError`), `mover.go` (`updateMover` confirm branch + clear-on-nav + `enterMover`/`enterMoverStandalone` reset + `moverView` sticky error), `styles.go` (`errColor`)
- Test: `mover_test.go`

**Interfaces:**
- Consumes: `applyMove`, `errColor`, `lipgloss.PlaceHorizontal`.
- Produces: `m.moverError string`; the mover stays open on a failed move and shows a sticky error.

- [ ] **Step 1: Write the failing test**

Add to `mover_test.go`:
```go
func TestMoverFailedMoveStaysOpenWithError(t *testing.T) {
	root := t.TempDir()
	// Source file + a colliding file at the destination so applyMove fails.
	srcDir := filepath.Join(root, "src")
	os.MkdirAll(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "ch.md"), []byte("x"), 0o644)
	dstDir := filepath.Join(root, "dst")
	os.MkdirAll(dstDir, 0o755)
	os.WriteFile(filepath.Join(dstDir, "ch.md"), []byte("y"), 0o644) // collision

	m := twoSourceModel(t, root, root)
	m.screen = screenMover
	m.moverPhase = moverPickDest
	m.moverSource = filepath.Join(srcDir, "ch.md")
	m.moverFromDir = srcDir
	m.moverIsDir = false
	m.moverDestDir = dstDir
	m.moverConfirm = true
	m.moverReturn = screenWriting

	nm, _ := m.updateMover(tea.KeyMsg{Type: tea.KeyEnter})
	got := nm.(model)
	if got.moverError == "" {
		t.Fatalf("a failed move should set moverError")
	}
	if got.screen != screenMover {
		t.Fatalf("the mover must stay open on failure, screen=%v", got.screen)
	}
	if got.moverConfirm {
		t.Fatalf("confirm should be dismissed after the failed attempt")
	}
}
```
(`tea` is already imported in `mover_test.go`; `twoSourceModel` from Task 2 is reused.)

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestMoverFailedMoveStaysOpen -v`
Expected: FAIL (`moverError` undefined, or the mover currently closes on failure).

- [ ] **Step 3: Add the model field + `errColor`**

In `main.go`, add beside the other `mover*` fields:
```go
	moverError string
```
In `styles.go`, add a semantic red (reuse the existing value):
```go
	errColor = lipgloss.Color("#ff5555") // red (Dracula)
```

- [ ] **Step 4: Stay open on a failed move (confirm branch)**

In `updateMover`, replace the `case "y", "enter":` body inside the `m.moverConfirm` block with:
```go
			case "y", "enter":
				if err := m.applyMove(); err != nil {
					m.moverError = "move failed: " + err.Error()
					m.moverConfirm = false
					m.moverReload()
					return m, nil
				}
				m.moverError = ""
				m.status = "moved " + filepath.Base(m.moverSource)
				m.moverConfirm = false
				m.files.SetDir(m.files.dir)
				m.screen = m.moverReturn
				return m, nil
```

- [ ] **Step 5: Clear the error on navigation + reset on entry**

- In the pick-dest `enter` drill case (from Task 2 Step 5), add `m.moverError = ""` as the first line:
```go
			case moverUp, moverFolder, moverSource:
				m.moverError = ""
				m.moverDestDir = e.path
				m.moverSel = 0
				m.moverReload()
```
- In the pick-source `enter` drill case (`case moverUp, moverFolder:`), add `m.moverError = ""` similarly.
- In `enterMover` and `enterMoverStandalone`, add `m.moverError = ""` near the other resets.

- [ ] **Step 6: Sticky error line in `moverView`**

In `moverView`, just before the existing `foot :=` footer (and after the `m.moverConfirm` block returns), insert:
```go
	if m.moverError != "" {
		errLine := lipgloss.NewStyle().Foreground(errColor).Render("⚠ " + m.moverError + "   (browse to dismiss · esc cancel)")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, errLine))
		return b.String()
	}
```

- [ ] **Step 7: Run tests + build + vet**

```
/opt/homebrew/bin/gofmt -w main.go mover.go styles.go mover_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: `TestMoverFailedMoveStaysOpenWithError` PASS; full suite green.

- [ ] **Step 8: Commit**

```
git add main.go mover.go styles.go mover_test.go
git commit -m "feat(mover): persistent move-error UX (stays open, sticky error)"
```

---

## Self-review notes
- **Spec coverage:** §1 (engine message) → Task 1; §2 (cross-source) → Task 2; §3 (persistent error) → Task 3. All covered.
- **Type consistency:** `moverSource` kind, `moverBoundingSource(string) (source, bool)`, `moverError string`, `errColor`, the `moverDestDir == ""` sentinel — used consistently across tasks. Task 2 Step 5 and Task 3 Step 5 both touch the pick-dest `enter` case (Task 3 adds the `m.moverError = ""` line); apply Task 2's version first, Task 3 augments it.
- **Invariants:** default destination stays `activeSourceRoot()`; `applyMove` unchanged; render stays windowed.
- **No placeholders:** every code step carries the actual code. If `sourceKind` constant names differ from `sourcePrimary`/`sourceFolder`, the implementer substitutes the real names (Task 2 Step 1 note).
