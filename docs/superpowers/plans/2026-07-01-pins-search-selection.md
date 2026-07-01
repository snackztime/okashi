# Pinning · Multi-source search · Mouse-selection docs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three independent QoL features — a "search everywhere" scope across all library sources, an F1 help line for native mouse selection, and a home PINNED strip for pinning projects/folders.

**Architecture:** Search adds a `scopeAll` + `searchAllSources` iterating `m.sources`. Selection is a one-line help-text addition. Pinning adds a `pins.json` store (mirrors `recent.go`), an `m.pinned` list toggled with `p` on home library items, and a `PINNED` strip region on home (mirrors the RECENT strip).

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `search.go`, `source.go` (`source.reachable()`/`.root()`/`.Name`), `recent.go` pattern, `home.go` (RECENT strip + regions), `atomicWrite`.

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`**, gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **Config stores take the path as a parameter** (`recent.go` pattern) — `loadPins(path)`/`togglePin(path,dir)`; only `pinsPath()` reads `os.UserConfigDir()`. Tests pass a temp path (hermetic).
- **`View()` stays O(visible):** the PINNED strip windows with `homeWindowOffset` like the RECENT strip.
- **Only projects/folders are pinnable** (`homeProject`/`homeFolder`) — not `◦ Notes`, not documents. Dead pins (paths gone from disk) are filtered at build.
- **`ctrl+a` (search-all) is intercepted in `updateSearch` BEFORE the input** (like `Tab`); `Tab` keeps cycling Document ↔ Project only.
- **No new dependency;** default build pure-Go.

---

### Task 1: Multi-source search — `scopeAll` + `searchAllSources` + `ctrl+a`

**Files:**
- Modify: `main.go` (`scopeAll` const); `search.go` (`searchAllSources`, `recomputeSearch`, `updateSearch` `ctrl+a`, `searchView` label/footer)
- Test: `search_test.go` (append)

**Interfaces:**
- Consumes: `searchProject(root, allowed, query, limit)` (search.go); `source.reachable()`/`.root()`/`.Name` (source.go); `m.sources`, `m.files.allowed`, `searchLimit`, `m.searchScope`.
- Produces: `const scopeAll`; `func searchAllSources(sources []source, allowed map[string]bool, query string, limit int) []searchHit`.

- [ ] **Step 1: Write the failing test**

Append to `search_test.go`:

```go
func TestSearchAllSourcesTagsAndSkips(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	gone := filepath.Join(t.TempDir(), "missing")
	os.WriteFile(filepath.Join(a, "one.md"), []byte("the windmill turned"), 0o644)
	os.WriteFile(filepath.Join(b, "two.md"), []byte("a wind rose"), 0o644)
	allowed := map[string]bool{".md": true}
	sources := []source{
		{ID: "primary", Name: "Primary", Kind: sourceKindPrimary, Path: a}, // root() = writingDir(); override below
		{ID: b, Name: "Dropbox", Kind: sourceKindFolder, Path: b},
		{ID: gone, Name: "Gone", Kind: sourceKindFolder, Path: gone},
	}
	// The primary's root() resolves to writingDir(); point it at `a` for this test.
	t.Setenv("OKASHI_DIR", a)

	hits := searchAllSources(sources, allowed, "wind", 50)
	if len(hits) < 2 {
		t.Fatalf("should find hits in both reachable sources, got %d: %+v", len(hits), hits)
	}
	var havePrimary, haveDropbox bool
	for _, h := range hits {
		if strings.HasPrefix(h.name, "Primary/") {
			havePrimary = true
		}
		if strings.HasPrefix(h.name, "Dropbox/") {
			haveDropbox = true
		}
		if strings.HasPrefix(h.name, "Gone/") {
			t.Fatalf("unreachable source must be skipped, got %q", h.name)
		}
	}
	if !havePrimary || !haveDropbox {
		t.Fatalf("hits should be tagged with their source name, got %+v", hits)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestSearchAllSources -v`
Expected: FAIL — `undefined: searchAllSources`.

- [ ] **Step 3: Add `scopeAll` + `searchAllSources` + wiring**

In `main.go`, add to the scope const block (after `scopeDocument`):

```go
	scopeAll
```

In `search.go`, add after `searchProject`:

```go
// searchAllSources searches every reachable library source's root, tagging each hit's display name
// with its source name so results read "Dropbox/notes.md:4". Capped at limit across all sources.
func searchAllSources(sources []source, allowed map[string]bool, query string, limit int) []searchHit {
	if strings.TrimSpace(query) == "" || limit <= 0 {
		return nil
	}
	var hits []searchHit
	for _, s := range sources {
		if !s.reachable() {
			continue
		}
		for _, h := range searchProject(s.root(), allowed, query, limit-len(hits)) {
			h.name = s.Name + "/" + h.name
			hits = append(hits, h)
		}
		if len(hits) >= limit {
			break
		}
	}
	return hits
}
```

In `recomputeSearch`, replace the `if scopeDocument … else …` with a scope switch:

```go
func (m *model) recomputeSearch() {
	q := m.searchInput.Value()
	switch m.searchScope {
	case scopeDocument:
		name := filepath.Base(m.currentFile)
		if name == "." || name == "" {
			name = "this document"
		}
		m.searchHits = searchText(name, m.currentFile, m.editor.Value(), q, searchLimit)
	case scopeAll:
		m.searchHits = searchAllSources(m.sources, m.files.allowed, q, searchLimit)
	default: // scopeProject
		m.searchHits = searchProject(m.files.root, m.files.allowed, q, searchLimit)
	}
	m.searchSel = 0
	m.searchOffset = 0
}
```

In `updateSearch`'s key switch (near the `case "tab":`), add:

```go
		case "ctrl+a":
			m.searchScope = scopeAll
			m.recomputeSearch()
			return m, nil
```

In `searchView`, extend the scope label (search for `scope := "Project"`):

```go
	scope := "Project"
	if m.searchScope == scopeDocument {
		scope = "This document"
	} else if m.searchScope == scopeAll {
		scope = "All sources"
	}
```

And append ` · ctrl+a all sources` to the footer hint string in `searchView` (the line with `Tab scope · esc back`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'SearchAll|Search' -v` then the full suite. vet + gofmt clean. Confirm `Tab` from `scopeAll` returns to Project (the existing `Tab` handler sets `scopeProject` when `searchScope != scopeProject`).

- [ ] **Step 5: Commit**

```bash
git add search.go main.go search_test.go
git commit -m "feat: multi-source search — ctrl+a searches all library sources"
```

---

### Task 2: Mouse-selection help line (F1)

**Files:**
- Modify: `main.go` (`helpText`, line ~42)

**Interfaces:** none (a help-string line).

- [ ] **Step 1: Add the line**

In `main.go`'s `helpText` const, add a line near the mouse/misc entries:

```
⌥/⇧+drag  select text (native) · ⌘C copy
```

(Match the existing `helpText` column formatting — the key on the left, the description aligned like the neighboring lines. Place it after the file-pane keys or in a "mouse" grouping if one exists.)

- [ ] **Step 2: Verify build + full suite unaffected**

Run: `/opt/homebrew/bin/go build ./...` and `/opt/homebrew/bin/go test ./...`. Both pass (a string change).

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "docs: F1 help line for native mouse text-selection (modifier-bypass)"
```

---

### Task 3: Pins store (`pins.json`)

**Files:**
- Create: `pins.go`
- Test: `pins_test.go` (create)

**Interfaces:**
- Consumes: `atomicWrite` (atomicwrite.go); `encoding/json`, `os`, `path/filepath`.
- Produces: `func pinsPath() string`, `func loadPins(path string) []string`, `func togglePin(path, dir string) []string`.

- [ ] **Step 1: Write the failing tests**

Create `pins_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinsToggleRoundTrip(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	if len(loadPins(store)) != 0 {
		t.Fatal("no store → empty")
	}
	p := togglePin(store, "/corpus/my-novel")
	if len(p) != 1 || p[0] != "/corpus/my-novel" {
		t.Fatalf("toggle should add, got %+v", p)
	}
	if got := loadPins(store); len(got) != 1 || got[0] != "/corpus/my-novel" {
		t.Fatalf("pin should persist, got %+v", got)
	}
	// Toggling the same path again removes it.
	if p := togglePin(store, "/corpus/my-novel"); len(p) != 0 {
		t.Fatalf("re-toggle should remove, got %+v", p)
	}
	if len(loadPins(store)) != 0 {
		t.Fatal("removal should persist")
	}
}

func TestPinsNoDuplicate(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	togglePin(store, "/a")
	togglePin(store, "/b")
	// re-adding /a via a fresh toggle removes it (toggle semantics); adding /c keeps order.
	if p := togglePin(store, "/c"); len(p) != 3 {
		t.Fatalf("want [/a /b /c], got %+v", p)
	}
}

func TestLoadPinsCorrupt(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	os.WriteFile(store, []byte("{bad"), 0o644)
	if len(loadPins(store)) != 0 {
		t.Fatal("corrupt store → empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'Pins|LoadPins' -v`
Expected: FAIL — `undefined: loadPins` etc.

- [ ] **Step 3: Create `pins.go`** (mirrors `recent.go`)

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type pinsFile struct {
	Pins []string `json:"pins"`
}

// pinsPath is the pinned-containers store, or "" if there is no usable config dir. Mirrors recentPath().
func pinsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "pins.json")
}

// loadPins reads the store; missing/corrupt/empty-path → nil.
func loadPins(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f pinsFile
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	return f.Pins
}

// togglePin adds dir if absent (appended) or removes it if present, persists atomically, and returns
// the new list. No-ops the write on an empty path (returns the computed list either way).
func togglePin(path, dir string) []string {
	pins := loadPins(path)
	out := make([]string, 0, len(pins)+1)
	found := false
	for _, p := range pins {
		if p == dir {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		out = append(out, dir)
	}
	if path != "" {
		if data, err := json.Marshal(pinsFile{Pins: out}); err == nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				_ = atomicWrite(path, data, 0o644)
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'Pins|LoadPins' -v`. PASS. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add pins.go pins_test.go
git commit -m "feat: pins.json store (load/toggle pinned containers)"
```

---

### Task 4: Pinning plumbing — `m.pinned`, the `p` toggle, `homePinned` items

**Files:**
- Modify: `main.go` (`m.pinned` field; init; the home `p` key); `home.go` (`homePinned` kind; `buildHomeItems` signature + call sites; `homeGroups` + `pinned()` accessor)
- Test: `home_test.go` (append)

**Interfaces:**
- Consumes: `loadPins`/`togglePin`/`pinsPath` (Task 3).
- Produces: model field `pinned []string`; `homePinned` kind; `buildHomeItems(recents, workspace, pinned)`; `func (m model) pinnedItems() []homeItem`.

- [ ] **Step 1: Write the failing test**

Append to `home_test.go`:

```go
func TestPinToggleOnHomeProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	t.Setenv("HOME", t.TempDir()) // isolate the pins config dir (TestMain also does this)
	proj := filepath.Join(root, "my-novel")
	createManuscript(proj, "My Novel", "Opening")

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = nm.(model)
	// select the project in LIBRARY and pin it with `p`.
	lib := m.library()
	for i, it := range lib {
		if it.label == "my-novel" {
			m.focusAt(regionLibrary, i)
		}
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = nm.(model)
	if len(m.pinnedItems()) != 1 || m.pinnedItems()[0].label != "★ my-novel" {
		t.Fatalf("p should pin the project, got %+v", m.pinnedItems())
	}
	// pinning ◦ Notes is a no-op.
	m.focusAt(regionLibrary, len(m.library())-1) // Notes is last
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = nm.(model)
	if len(m.pinnedItems()) != 1 {
		t.Fatalf("pinning ◦ Notes should be a no-op, got %+v", m.pinnedItems())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestPinToggle -v`
Expected: FAIL — `m.pinnedItems` / `homePinned` undefined.

- [ ] **Step 3: Implement the plumbing**

In `main.go`: add `pinned []string` to the model struct (near `sources`); in `initialModel`, `pinned: loadPins(pinsPath())`. In the home key handler (`updateHome`'s KeyMsg switch), add:

```go
		case "p":
			if m.homeRegion == regionLibrary {
				lib := m.library()
				if m.librarySelected >= 0 && m.librarySelected < len(lib) {
					it := lib[m.librarySelected]
					if it.kind == homeProject || it.kind == homeFolder {
						m.pinned = togglePin(pinsPath(), it.path)
						m.rebuildHome()
					}
				}
			}
```

In `home.go`: add `homePinned` to the `homeKind` const block (before `homeRecentFile`). Change `buildHomeItems` to accept pins and prepend live ones:

```go
func buildHomeItems(recents []string, workspace string, pinned []string) []homeItem {
	var items []homeItem
	for _, p := range pinned {
		if _, err := os.Stat(p); err == nil { // skip dead pins
			items = append(items, homeItem{kind: homePinned, label: "★ " + filepath.Base(p), path: p})
		}
	}
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}
	// ... rest unchanged (Notes, projects, folders, actions) ...
}
```

Update the call sites: `rebuildHome` and `initialModel`'s `homeItems:` literal and the `ctrl+o` refresh — all pass `m.pinned`. In `homeGroups`, route `homePinned` into a new leading return `pinned`:

```go
func homeGroups(items []homeItem) (pinned, recents, projects, folders, other, actions []homeItem) {
	for _, it := range items {
		switch it.kind {
		case homePinned:
			pinned = append(pinned, it)
		case homeRecentFile:
			recents = append(recents, it)
		// ... existing cases ...
		}
	}
	return
}
```

Update EVERY `homeGroups(...)` destructuring call site (add the leading `pinned` slot; use `_` where unused): `recents()`, `actions()`, `library()`, `libraryColumn`, and add:

```go
func (m model) pinnedItems() []homeItem { p, _, _, _, _, _ := homeGroups(m.homeItems); return p }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'PinToggle|Home|BuildHomeItems' -v` then the full suite. Update any existing test that calls `buildHomeItems(recents, ws)` (now 3 args — pass `nil` for pins) or destructures `homeGroups` (now 6 returns). vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add main.go home.go home_test.go
git commit -m "feat: pin/unpin home projects & folders (p key, homePinned items)"
```

---

### Task 5: PINNED strip — render, nav, hit-test

**Files:**
- Modify: `home.go` (`regionPinned`; `pinnedStrip`; `homeContent` renders it above RECENT; `visibleRegions`/`visibleCols`/`regionCount`; `homeMove`/`homeItemAt`/`openHomeSelection`)
- Test: `home_test.go` (append)

**Interfaces:**
- Consumes: `pinnedItems()` (Task 4); `homeWindowOffset`, `framedPanel`, the existing `recentStrip` pattern.
- Produces: `const regionPinned`; `func (m model) pinnedStrip(contentW int) ([]string, []innerCell)`.

**Context:** The PINNED strip is a second full-width horizontal strip ABOVE the RECENT strip, structurally identical to `recentStrip`/`regionRecent`. Rendered only when there are pins.

- [ ] **Step 1: Write the failing test**

Append to `home_test.go`:

```go
func TestPinnedStripRendersAndHitTests(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	t.Setenv("HOME", t.TempDir())
	createManuscript(filepath.Join(root, "my-novel"), "My Novel", "Opening")
	os.MkdirAll(filepath.Join(root, "research"), 0o755)

	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = nm.(model)
	m.pinned = []string{filepath.Join(root, "my-novel"), filepath.Join(root, "research")}
	m.rebuildHome()
	m.resetHomeSelection()

	out := ansiStrip(m.homeView())
	if !strings.Contains(out, "PINNED") || !strings.Contains(out, "★ my-novel") {
		t.Fatalf("home should render a PINNED strip with the pins:\n%s", out)
	}
	// render == hit-test: each pinned cell round-trips.
	_, cells, _ := m.homeContent()
	var pinnedCells int
	for _, c := range cells {
		if c.region == regionPinned {
			pinnedCells++
			x, y := homeCellXY(m, c.region, c.index)
			r, idx, ok := m.homeItemAt(x, y)
			if !ok || r != c.region || idx != c.index {
				t.Fatalf("pinned cell (%d) failed hit-test → (%d,%d,%v)", c.index, r, idx, ok)
			}
		}
	}
	if pinnedCells != 2 {
		t.Fatalf("want 2 pinned cells, got %d", pinnedCells)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestPinnedStrip -v`
Expected: FAIL — `regionPinned` undefined / no PINNED strip.

- [ ] **Step 3: Implement the strip + region**

Add `regionPinned` to the `homeRegion` const block (before `regionRecent`). Add `pinnedStrip` (copy `recentStrip`, but source `m.pinnedItems()`, empty → return `nil,nil` so an empty strip is not shown, and use the item labels which already include `★ `):

```go
func (m model) pinnedStrip(contentW int) ([]string, []innerCell) {
	pins := m.pinnedItems()
	if len(pins) == 0 {
		return nil, nil
	}
	const sep = "   "
	label := func(i int) string { return pins[i].label } // already "★ name"
	active := 0
	if m.homeRegion == regionPinned {
		active = clampIdx(m.homeIndex, len(pins))
	}
	start := active
	used := lipgloss.Width(label(active))
	for start > 0 {
		w := lipgloss.Width(sep) + lipgloss.Width(label(start-1))
		if used+w > contentW {
			break
		}
		used += w
		start--
	}
	var b strings.Builder
	var cells []innerCell
	col := 0
	for i := start; i < len(pins); i++ {
		lbl := label(i)
		if i == start {
			lbl = ansi.Truncate(lbl, contentW, "…")
		} else {
			if col+lipgloss.Width(sep)+lipgloss.Width(lbl) > contentW {
				break
			}
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		sel := m.homeRegion == regionPinned && m.homeIndex == i
		b.WriteString(homeLabel(lbl, sel))
		cells = append(cells, innerCell{regionPinned, i, 0, col, col + lipgloss.Width(lbl)})
		col += lipgloss.Width(lbl)
	}
	return []string{b.String()}, cells
}
```

In `regionCount`, add `case regionPinned: return len(m.pinnedItems())`. In `homeContent`, render the PINNED strip BEFORE the RECENT strip (only when `len(m.pinnedItems()) > 0`), mirroring the RECENT-strip block (frame `framedPanel("PINNED", …, blockW, 3, "")`, translate cells with `stripTop+1` / `2+x`). In `visibleRegions`, prepend `regionPinned` when `regionCount(regionPinned) > 0`:

```go
func (m model) visibleRegions() []homeRegion {
	if m.width <= 0 {
		return []homeRegion{regionPinned, regionRecent, regionLibrary, regionFiles, regionActions}
	}
	cols, _, _ := m.homeColumns()
	var regs []homeRegion
	if m.regionCount(regionPinned) > 0 {
		regs = append(regs, regionPinned)
	}
	regs = append(regs, regionRecent)
	regs = append(regs, cols...)
	return append(regs, regionActions)
}
```

`visibleCols` already excludes `regionRecent`/`regionActions`; add `regionPinned` to the exclusion (it is a strip, not a browse column).

In `homeMove`, handle `regionPinned` like `regionRecent` but as the TOP strip: `←→` move within; `↓` enters the RECENT strip (or the first non-empty column / actions); `↑` does nothing. In the `regionRecent` `↑` case, if `regionCount(regionPinned) > 0`, `↑` from RECENT enters the PINNED strip.

In `openHomeSelection`, add a `regionPinned` case: select the pinned container = `SetDir(pinnedItems()[m.homeIndex].path)` + focus the sidebar (mirror the `regionLibrary` `homeProject`/`homeFolder` open).

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'PinnedStrip|Home|Pin' -v` then the full suite. Update any existing home nav/hit-test test whose expectations shift when a PINNED strip is present (only tests that set `m.pinned` will see it; the default has no pins, so most are unaffected). vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add home.go home_test.go
git commit -m "feat: home PINNED strip (region, render, nav, hit-test)"
```

---

## Self-Review

**Spec coverage (against `2026-07-01-pins-search-selection-design.md`):**
- §1 multi-source search: `scopeAll` + `searchAllSources` (tag by source, skip unreachable, cap) + `ctrl+a` + label → Task 1. ✅
- §2 mouse-selection help line → Task 2. ✅
- §3 pinning: store → Task 3; `m.pinned` + `p` toggle (projects/folders only) + `homePinned` → Task 4; PINNED strip render/nav/hit-test + dead-pin filter → Tasks 4 (filter in buildHomeItems) + 5. ✅
- Out of scope (unbuilt): document/Notes pinning, pin reorder, README rewrite, mouse-capture changes.

**Placeholder scan:** every code step is complete. `buildHomeItems`/`homeGroups` signature changes explicitly name "update every call site" with the mechanical change (add a `nil`/`_` slot) — a splice instruction with the exact edit, not a placeholder.

**Type consistency:** `scopeAll` (Task 1). `pinsPath`/`loadPins`/`togglePin(path,dir)` (Task 3) consumed by Task 4. `buildHomeItems(recents, workspace, pinned)` (Task 4) — every caller updated. `homeGroups` gains a leading `pinned` return (Task 4); `pinnedItems()` reads it; `regionPinned`/`pinnedStrip` (Task 5) consume `pinnedItems()`. `homePinned` kind routed consistently. `clampIdx`/`homeWindowOffset`/`ansi.Truncate`/`framedPanel`/`homeLabel` are existing.

**Open follow-through for the executor:** the `homeGroups` 5→6 return change and `buildHomeItems` 2→3 arg change ripple to several call sites and a few existing tests (`TestBuildHomeItems*`, any `homeGroups(` destructure) — grep for both and update mechanically (add the pinned slot / a `nil` pins arg) before running the suite. The PINNED strip adds a nav layer; verify `TestHomeVerticalFlow`-style flows still hold with pins absent (default) and present.
