# okashi Library — Step 3a: Multi-Source Home Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the sources model into the home screen so the library reflects an active source you switch with `s`, shows the source name in the LIBRARY title, exposes the source root's loose docs as a leading `◦ Loose` entry, and lets you add/remove folder sources inline — WITHOUT yet changing the column layout (that is step 3b).

**Architecture:** Add `sources`/`activeSource` to the model with an `activeSourceRoot()` helper; route the home library build through it. Add `s` cycling, a dynamic LIBRARY title, a synthetic `◦ Loose` library item, and an inline add-source prompt + `d` remove. All logic-testable; the visible layout stays the current RECENT│LIBRARY│FILES Miller until step 3b.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `source.go` (Task from step 2), `home.go`, `main.go`. Reuses `loadSources`/`saveSources`/`sourcesPath`/`addSource`/`removeSource`/`newFolderSource` and the `nameInput` prompt.

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`** and gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **The active source drives the library.** The home workspace is `m.activeSourceRoot()` (= `m.sources[m.activeSource].root()`), NOT a hardcoded `writingDir()`. With only the primary source present (`activeSource==0`), behavior is IDENTICAL to today — existing home tests must stay green.
- **The primary source is index 0, always present, non-removable.** `s` never removes it; `d` on it is a no-op with a status.
- **Unreachable sources are skipped** by `s` cycling (never switch INTO an unreachable source); the primary is always reachable enough to fall back to.
- **Persist on change:** adding/removing a source calls `saveSources(sourcesPath(), m.sources)`.
- **No layout change in this step** — the RECENT│LIBRARY│FILES columns, nav, and hit-testing stay as-is. Only the LIBRARY title text, the `◦ Loose` row, and the `+ add source…` row change within the existing structure. (Recent-on-top + inline `+` on LIBRARY/FILES + Browse-only actions = step 3b.)
- **Default build stays pure-Go;** no new dependencies.

---

### Task 1: Sources in the model + `activeSourceRoot()`

**Files:**
- Modify: `main.go` — model struct (after line 174), `initialModel` (line 280 area), the home refresh (line 1085)
- Test: `source_test.go` (add a model-level test) OR `home_test.go` — use whichever already imports the model; prefer `home_test.go` if it constructs `initialModel()`.

**Interfaces:**
- Consumes: `loadSources`, `sourcesPath`, `source.root()`, `primarySource` (step 2 `source.go`); `writingDir()`.
- Produces: model fields `sources []source`, `activeSource int`; `func (m model) activeSourceRoot() string`; `func (m *model) rebuildHome()`.

- [ ] **Step 1: Write the failing test**

Add to `home_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestActiveSourceRoot -v`
Expected: FAIL — `m.sources undefined`.

- [ ] **Step 3: Add the fields, helper, and wiring**

In `main.go`, add to the model struct after `librarySelected int` (line 174):

```go
	sources         []source // library sources; [0] is always the primary (writingDir())
	activeSource    int      // index into sources driving the home library
```

In `initialModel`'s `model{...}` literal, add:

```go
		sources:        loadSources(sourcesPath()),
		activeSource:   0,
```

Add these methods (near the other `model` home methods in `home.go`):

```go
// activeSourceRoot is the filesystem root of the active library source. Falls back to
// writingDir() if the index is somehow out of range (defensive; activeSource is clamped).
func (m model) activeSourceRoot() string {
	if m.activeSource < 0 || m.activeSource >= len(m.sources) {
		return writingDir()
	}
	return m.sources[m.activeSource].root()
}

// rebuildHome rebuilds the launch list from recents + the active source's library, then
// refreshes the FILES column. Call after anything that changes the active source.
func (m *model) rebuildHome() {
	m.homeItems = buildHomeItems(loadRecents(recentPath()), m.activeSourceRoot())
	m.recomputeHomeFiles()
}
```

At `main.go:1085` (the home refresh), replace:

```go
			m.homeItems = buildHomeItems(loadRecents(recentPath()), writingDir())
```

with:

```go
			m.homeItems = buildHomeItems(loadRecents(recentPath()), m.activeSourceRoot())
```

(Leave the `initialModel` `homeItems:` literal as `writingDir()` — identical to `activeSourceRoot()` at init since `activeSource==0` is the primary; a comment noting this is welcome.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'ActiveSourceRoot|Home' -v` then the full suite `/opt/homebrew/bin/go test ./...`.
Expected: PASS (existing home tests unaffected — primary root == writingDir()). vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add main.go home.go home_test.go
git commit -m "feat: active source drives the home library (sources in the model)"
```

---

### Task 2: `s` cycles the active source + dynamic LIBRARY title

**Files:**
- Modify: `home.go` — `updateHome` KeyMsg switch; `homeColumns` (LIBRARY title)
- Test: `home_test.go`

**Interfaces:**
- Consumes: `m.sources`, `m.activeSource`, `m.activeSourceRoot`, `rebuildHome`, `source.reachable`, `source.Name` (Task 1 + step 2).
- Produces: `func (m *model) cycleSource(dir int)`; `s` key handling; LIBRARY title `"LIBRARY · <name> ▾"`.

- [ ] **Step 1: Write the failing test**

Add to `home_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestCycleSource -v`
Expected: FAIL — `m.cycleSource undefined`.

- [ ] **Step 3: Implement `cycleSource` + the `s` key + the title**

Add to `home.go`:

```go
// cycleSource advances the active source by dir (wrapping), skipping unreachable sources, then
// rebuilds the library. Stays put if no other reachable source exists.
func (m *model) cycleSource(dir int) {
	n := len(m.sources)
	if n <= 1 {
		return
	}
	for i := 1; i <= n; i++ {
		nxt := ((m.activeSource+dir*i)%n + n) % n
		if nxt == m.activeSource {
			break
		}
		if m.sources[nxt].reachable() {
			m.activeSource = nxt
			m.rebuildHome()
			m.librarySelected = 0
			m.recomputeHomeFiles()
			m.resetHomeSelection()
			m.status = "source: " + m.sources[nxt].Name
			return
		}
	}
}
```

In `updateHome`'s `KeyMsg` switch (after the existing `"shift+tab"` case, before `"enter"`), add:

```go
		case "s":
			m.cycleSource(1)
```

In `homeColumns`, replace the static LIBRARY title. Change:

```go
	allTitles := []string{"RECENT", "LIBRARY", "FILES"}
```

to:

```go
	libTitle := "LIBRARY"
	if len(m.sources) > 1 && m.activeSource >= 0 && m.activeSource < len(m.sources) {
		libTitle = "LIBRARY · " + m.sources[m.activeSource].Name + " ▾"
	}
	allTitles := []string{"RECENT", libTitle, "FILES"}
```

(When only the primary source exists, the title stays plain `"LIBRARY"` — no `▾` clutter for single-source users.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'CycleSource|Home' -v`, then full suite. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add home.go home_test.go
git commit -m "feat: 's' cycles the active source; LIBRARY title shows it"
```

---

### Task 3: `◦ Loose` leading LIBRARY entry (the source root's unfiled docs)

**Files:**
- Modify: `home.go` — `homeKind` (add `homeLoose`), `buildHomeItems`, `homeGroups`/`library`, `libraryColumn`, `openHomeSelection`
- Test: `home_test.go`

**Interfaces:**
- Consumes: `m.homeFilesFor` (returns a dir's docs — for a non-manuscript root this is its loose docs), `activeSourceRoot`.
- Produces: a `homeLoose` item at the head of the library; selecting it drives FILES with the root's loose docs.

**Context:** `homeFilesFor(root)` already returns the root's loose documents when `root` is a plain (non-manuscript) directory — `resolveManuscript` classifies the source root as a category, so its `loose` docs come back. The `◦ Loose` item's `path` is the source root; selecting it reuses `homeFilesFor`.

- [ ] **Step 1: Write the failing test**

Add to `home_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestLooseEntry -v`
Expected: FAIL — `homeLoose` undefined / first item is not Loose.

- [ ] **Step 3: Implement the `◦ Loose` item**

In `home.go`, add `homeLoose` to the `homeKind` const block (after `homeFolder`):

```go
	homeLoose
```

In `buildHomeItems`, prepend the Loose item before projects (its path is the source root):

```go
func buildHomeItems(recents []string, workspace string) []homeItem {
	var items []homeItem
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}
	items = append(items, homeItem{kind: homeLoose, label: "◦ Loose", path: workspace})
	projects, folders := classifyLibrary(workspace)
	items = append(items, projects...)
	items = append(items, folders...)
	items = append(items,
		homeItem{kind: homeNewDocument, label: "New document"},
		homeItem{kind: homeNewProject, label: "New project"},
		homeItem{kind: homeOpenOther, label: "Browse all files"},
	)
	return items
}
```

In `homeGroups`, route `homeLoose` into the library group (so `library()` includes it first). Change the switch:

```go
	for _, it := range items {
		switch it.kind {
		case homeRecentFile:
			recents = append(recents, it)
		case homeLoose, homeProject:
			projects = append(projects, it)
		case homeFolder:
			folders = append(folders, it)
		default:
			actions = append(actions, it)
		}
	}
```

(The `homeLoose` item now sorts to the front of `projects`, hence the front of `library()` — `buildHomeItems` already placed it first, and `homeGroups` preserves order.)

In `libraryColumn`, render the Loose row before the PROJECTS header. At the top of the row-building (before the `if len(projects) > 0` block), split the Loose item out of `projects`:

```go
	_, projects, folders, _ := homeGroups(m.homeItems)
	type lrow struct {
		header bool
		text   string
		libIdx int
	}
	var rows []lrow
	idx := 0
	// The leading ◦ Loose entry (if present) renders above the PROJECTS header.
	if len(projects) > 0 && projects[0].kind == homeLoose {
		rows = append(rows, lrow{text: projects[0].label, libIdx: idx})
		idx++
		projects = projects[1:]
	}
	if len(projects) > 0 {
		rows = append(rows, lrow{header: true, text: "PROJECTS"})
		for _, p := range projects {
			rows = append(rows, lrow{text: "› " + p.label, libIdx: idx})
			idx++
		}
	}
	// ... (FOLDERS block unchanged below)
```

In `openHomeSelection`'s `regionLibrary` case, `SetDir` to the item's path (for Loose that is the source root, landing you among its loose docs) — the existing code already does `m.files.SetDir(lib[m.librarySelected].path)`, which is correct for Loose since its `path` is the root. No change needed there, but confirm it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'Loose|Home|Library' -v`, then full suite. vet + gofmt clean. Existing home/library tests must still pass (the Loose item shifts library indices by one — check any test asserting `librarySelected`/library ordering and update expectations if the plan's Task-1/2 tests referenced positions; the new Loose-first ordering is intended).

- [ ] **Step 5: Commit**

```bash
git add home.go home_test.go
git commit -m "feat: '◦ Loose' library entry for the source root's unfiled docs"
```

---

### Task 4: Add / remove a folder source inline

**Files:**
- Modify: `home.go` — a `+ add source…` action row + `d` key; `main.go` — a small add-source prompt mode reusing `nameInput`
- Test: `home_test.go`

**Interfaces:**
- Consumes: `addSource`, `removeSource`, `newFolderSource`, `saveSources`, `sourcesPath`, `source.reachable` (step 2); `nameInput`.
- Produces: `func (m *model) confirmAddSource(path string)`; `func (m *model) removeActiveSource()`; a `d` key on home; an add-source prompt entry point.

**Context:** Keep it lean (spec §4, resolved: cycle + inline add/remove). Adding a source: a prompt where the user types/pastes an absolute folder path. Removing: `d` on the home screen removes the ACTIVE source if it is a folder source. This task adds the model operations + key handling + a minimal prompt; it does NOT need the step-3b layout. If wiring a brand-new input mode is heavy, gate the prompt behind the existing `nameInput` with a boolean `m.addingSource` and render it in the home view's status line.

- [ ] **Step 1: Write the failing test** (model operations — no UI needed to test these)

Add to `home_test.go`:

```go
func TestConfirmAddSourcePersistsAndSwitches(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run 'AddSource|RemoveActiveSource' -v`
Expected: FAIL — `confirmAddSource` / `removeActiveSource` undefined.

- [ ] **Step 3: Implement the model operations**

Add to `home.go`:

```go
// confirmAddSource adds path as a folder source (if reachable), persists, switches to it, and
// rebuilds the library. Ignores an unreachable/duplicate path (addSource dedups by ID).
func (m *model) confirmAddSource(path string) {
	s := newFolderSource(path)
	if !s.reachable() {
		m.status = "not a folder: " + path
		return
	}
	before := len(m.sources)
	m.sources = addSource(m.sources, s)
	if len(m.sources) == before { // dedup: already present → just switch to it
		for i, e := range m.sources {
			if e.ID == s.ID {
				m.activeSource = i
			}
		}
	} else {
		m.activeSource = len(m.sources) - 1
	}
	_ = saveSources(sourcesPath(), m.sources)
	m.rebuildHome()
	m.librarySelected = 0
	m.recomputeHomeFiles()
	m.resetHomeSelection()
	m.status = "source added: " + s.Name
}

// removeActiveSource removes the active source if it is a folder source, persists, and returns
// to the primary. A no-op (with a status) when the active source is the primary.
func (m *model) removeActiveSource() {
	if m.activeSource < 0 || m.activeSource >= len(m.sources) {
		return
	}
	s := m.sources[m.activeSource]
	if s.Kind == sourceKindPrimary {
		m.status = "the primary source can't be removed"
		return
	}
	m.sources = removeSource(m.sources, s.ID)
	m.activeSource = 0
	_ = saveSources(sourcesPath(), m.sources)
	m.rebuildHome()
	m.librarySelected = 0
	m.recomputeHomeFiles()
	m.resetHomeSelection()
	m.status = "source removed: " + s.Name
}
```

- [ ] **Step 4: Wire the keys/prompt (minimal)**

In `updateHome`'s `KeyMsg` switch, add a `d` key that removes the active source:

```go
		case "d":
			m.removeActiveSource()
```

For adding: add a `case "a":` that opens a path prompt. Reuse `nameInput` with a new `m.addingSource bool` flag (add the field to the model struct near `creatingFile`). On `a`: `m.addingSource = true; m.nameInput.SetValue(""); m.nameInput.Placeholder = "/path/to/folder"; m.nameInput.Focus()`. Route input while `m.addingSource` is true (in `updateHome` or the top-level `Update`, mirroring how `creatingFile` is captured — see `main.go:711` "While naming a new file, the prompt captures all input") to `nameInput`; on `enter` call `m.confirmAddSource(strings.TrimSpace(m.nameInput.Value()))` and clear the flag; on `esc` clear it. Render the prompt on the home screen's status/footer line while active.

**If the prompt wiring is non-trivial**, ship Task 4 with the model ops + the `d` remove + the `a` handler calling `confirmAddSource` on the current `nameInput` value, and note the prompt-rendering polish for step 3b (which reworks the home input surface anyway). The DONE bar for this task is the three model-operation tests passing + `d`/`a` wired; the prompt's visual polish can defer.

- [ ] **Step 5: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'AddSource|RemoveActiveSource|Home' -v`, then full suite. vet + gofmt clean.

- [ ] **Step 6: Commit**

```bash
git add home.go main.go home_test.go
git commit -m "feat: add/remove folder sources inline (a/d on home)"
```

---

## Self-Review

**Spec coverage (against `2026-06-30-okashi-library-sources-manifest-design.md` §1, §4 source-picker resolution):**
- Active source drives the library (workspace = `activeSourceRoot()`) → Task 1. ✅
- `s` cycles sources, skipping unreachable; LIBRARY title shows the active source → Task 2. ✅ (spec §4 resolved)
- `◦ Loose` leading entry = source root's unfiled docs → Task 3. ✅
- Inline add (`a` → path prompt, `addSource`+persist+switch) / remove (`d`, `removeSource`+persist; primary protected) → Task 4. ✅ (spec §4 resolved: cycle + inline add/remove)
- Switching repopulates LIBRARY + FILES → `rebuildHome`/`recomputeHomeFiles`/`resetHomeSelection` in Tasks 2 & 4. ✅
- NOT in this step (step 3b): Recent-on-top strip, inline `+` on LIBRARY/FILES with trailing-slash trigger, Browse-only actions, the add-source prompt's visual polish.

**Type consistency:** `activeSourceRoot()`/`rebuildHome()` (Task 1) consumed by Tasks 2-4; `cycleSource` (Task 2); `confirmAddSource`/`removeActiveSource` (Task 4). `homeLoose` (Task 3) routed through `homeGroups`→`library()`. All operate on the `[]source` from step 2.

**Placeholder scan:** the one soft edge is Task 4's add-source PROMPT rendering (a new input surface). It is explicitly scoped: the testable model operations (`confirmAddSource`/`removeActiveSource`) are fully specified with tests; the prompt's visual wiring is bounded with a fallback (defer polish to 3b, which reworks the home input surface regardless). The `d`/`a` key handlers and model ops are the task's hard DONE bar.

**Index-shift caution (Task 3):** inserting `◦ Loose` at the front of the library shifts `librarySelected` indices by one. Any existing test asserting a specific library index or `openHomeSelection` on a library position must be re-checked; the Loose-first ordering is intended, so update expectations rather than preserve old positions.
