# okashi Library — Step 2: Sources Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give okashi a multi-source library model — a primary source (`writingDir()`) plus user-added folder sources persisted in `sources.json` — mirroring the companion app's `SourceStore`, as the pure foundation the home source-picker UI will consume.

**Architecture:** A new `source.go` with a `source`/`sourceKind` value type (resolved `root()`, `reachable()`), a synthesized always-present primary source, and a `sources.json` store (`os.UserConfigDir()/okashi/sources.json`) that persists ONLY user-added folder sources — the primary is synthesized at load. No UI in this step; the picker + `◦ Loose` + home layout land in step 3.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), `encoding/json`, existing `atomicWrite` (`atomicwrite.go`), existing `writingDir()` (`main.go:321`). Mirrors the `recent.go` persistence pattern verbatim.

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`** and gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **`sources.json` persists user-added folder sources ONLY.** The primary source is synthesized at load and NEVER written to the file (mirrors the companion app's `SourceStore`; spec §1).
- **The primary source is always present and non-removable.** Its resolved root is `writingDir()` (which honors `OKASHI_DIR`); its stored `Path` is `""`.
- **All writes atomic** via `atomicWrite`; create the config dir with `os.MkdirAll` first (as `recent.go` does).
- **Graceful degradation:** a missing/corrupt `sources.json`, or an unusable config dir, yields just `[primary]` — never an error to the caller (mirrors `loadRecents`).
- **Per-source isolation:** an unreachable folder source is reported via `reachable()==false` and must never block loading the others (spec §1). This step provides the predicate; the UI skips on it in step 3.
- **Default build stays pure-Go.** No new dependencies.
- **Shape alignment:** keep the JSON field names (`id`,`name`,`kind`,`path`) conceptually aligned with the companion app's `Source` so the two apps stay parallel (separate files, same fields; spec §1). This is NOT the manifest schema and carries no HARD-GATE, but keep the field set stable.

---

### Task 1: Source value type (`source`, `sourceKind`, primary, `root`, `reachable`)

**Files:**
- Create: `source.go`
- Test: `source_test.go`

**Interfaces:**
- Consumes: `writingDir()` (`main.go:321`); `filepath`, `os` (stdlib).
- Produces:
  - `type sourceKind int` with `sourceKindPrimary`, `sourceKindFolder`
  - `type source struct { ID, Name string; Kind sourceKind; Path string }` (JSON tags `id`,`name`,`kind`,`path`)
  - `const primarySourceID = "primary"`
  - `func primarySource() source`
  - `func newFolderSource(path string) source`
  - `func (s source) root() string`
  - `func (s source) reachable() bool`

- [ ] **Step 1: Write the failing tests**

Create `source_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrimarySourceResolvesToWritingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)
	p := primarySource()
	if p.ID != primarySourceID {
		t.Fatalf("primary ID = %q, want %q", p.ID, primarySourceID)
	}
	if p.Kind != sourceKindPrimary {
		t.Fatalf("primary Kind = %v, want primary", p.Kind)
	}
	if p.Path != "" {
		t.Fatalf("primary stored Path must be empty (resolved at runtime), got %q", p.Path)
	}
	if p.root() != dir {
		t.Fatalf("primary root() = %q, want writingDir() %q", p.root(), dir)
	}
}

func TestNewFolderSource(t *testing.T) {
	s := newFolderSource("/tmp/writing/Dropbox Novels")
	if s.Kind != sourceKindFolder {
		t.Fatalf("Kind = %v, want folder", s.Kind)
	}
	if s.Path != "/tmp/writing/Dropbox Novels" || s.root() != "/tmp/writing/Dropbox Novels" {
		t.Fatalf("Path/root = %q/%q", s.Path, s.root())
	}
	if s.Name != "Dropbox Novels" {
		t.Fatalf("Name = %q, want the folder base name", s.Name)
	}
	if s.ID == "" || s.ID == primarySourceID {
		t.Fatalf("folder source needs a stable non-primary ID, got %q", s.ID)
	}
}

func TestSourceReachable(t *testing.T) {
	dir := t.TempDir()
	if !newFolderSource(dir).reachable() {
		t.Fatal("an existing dir must be reachable")
	}
	if newFolderSource(filepath.Join(dir, "gone")).reachable() {
		t.Fatal("a missing dir must be unreachable")
	}
	// A file (not a dir) is not a reachable source root.
	f := filepath.Join(dir, "a.md")
	os.WriteFile(f, []byte("x"), 0o644)
	if newFolderSource(f).reachable() {
		t.Fatal("a plain file is not a reachable source root")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'PrimarySource|NewFolderSource|SourceReachable' -v`
Expected: FAIL — `undefined: primarySource` etc.

- [ ] **Step 3: Write `source.go`**

```go
package main

import (
	"os"
	"path/filepath"
)

// sourceKind distinguishes the always-present primary source from user-added folders.
type sourceKind int

const (
	sourceKindPrimary sourceKind = iota
	sourceKindFolder
)

const primarySourceID = "primary"

// source is one library root okashi browses. It mirrors the companion app's Source (id/name/kind/path)
// so the two apps stay conceptually parallel, though each persists its own file. The primary's
// Path is empty and resolves to writingDir() at runtime; folder sources carry an absolute path.
type source struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Kind sourceKind `json:"kind"`
	Path string     `json:"path"`
}

// primarySource is the always-present, non-removable default root (= writingDir()).
func primarySource() source {
	name := filepath.Base(writingDir())
	if name == "." || name == "" || name == string(filepath.Separator) {
		name = "okashi"
	}
	return source{ID: primarySourceID, Name: name, Kind: sourceKindPrimary}
}

// newFolderSource builds a user folder source from an absolute path. The path is its own stable
// ID (dedup key); the display Name defaults to the folder's base name (user-editable later).
func newFolderSource(path string) source {
	name := filepath.Base(path)
	if name == "." || name == "" {
		name = path
	}
	return source{ID: path, Name: name, Kind: sourceKindFolder, Path: path}
}

// root is the resolved filesystem root: writingDir() for the primary, else the stored Path.
func (s source) root() string {
	if s.Kind == sourceKindPrimary {
		return writingDir()
	}
	return s.Path
}

// reachable reports whether the source root exists and is a directory. An unreachable folder
// source (deleted/offline) is skipped by the UI, never blocking the others (spec §1).
func (s source) reachable() bool {
	info, err := os.Stat(s.root())
	return err == nil && info.IsDir()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'PrimarySource|NewFolderSource|SourceReachable' -v`
Expected: PASS. Then `/opt/homebrew/bin/go vet ./...` clean and `/opt/homebrew/bin/gofmt -l source.go source_test.go` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add source.go source_test.go
git commit -m "feat: source value type (primary + folder, root/reachable)"
```

---

### Task 2: Sources store (`sources.json` load / save / add / remove)

**Files:**
- Modify: `source.go` (append the store functions)
- Test: `source_test.go` (append store tests)

**Interfaces:**
- Consumes: `source`, `primarySource`, `sourceKindPrimary`, `sourceKindFolder`, `newFolderSource` (Task 1); `atomicWrite` (`atomicwrite.go`); `os.UserConfigDir`, `encoding/json`, `filepath` (add imports `encoding/json` to `source.go`).
- Produces:
  - `func sourcesPath() string` — the default store path (production; `""` if no config dir)
  - `func loadSources(path string) []source` — `[primary]` + persisted folder sources
  - `func saveSources(path string, all []source) error` — persists ONLY folder sources
  - `func addSource(all []source, s source) []source` — dedup by ID, returns the new slice
  - `func removeSource(all []source, id string) []source` — no-op for the primary ID

**Context:** Mirror `recent.go` EXACTLY: the load/save functions take the store **path as a parameter** (so tests pass a temp path — `recent.go`'s `loadRecents(store)`/`addRecent(store,…)` do this; `recentPath()` only supplies the production default). Production callers use `loadSources(sourcesPath())`. The store file holds a `{"sources": [...]}` object of user folder sources; the primary is synthesized in `loadSources`, never serialized.

- [ ] **Step 1: Write the failing tests**

Append to `source_test.go`:

```go
func TestLoadSourcesNoFileIsPrimaryOnly(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	got := loadSources(store)
	if len(got) != 1 || got[0].Kind != sourceKindPrimary {
		t.Fatalf("no store should yield exactly [primary], got %+v", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	folder := newFolderSource(t.TempDir())
	all := []source{primarySource(), folder}
	if err := saveSources(store, all); err != nil {
		t.Fatalf("saveSources: %v", err)
	}
	got := loadSources(store)
	if len(got) != 2 {
		t.Fatalf("want [primary, folder], got %+v", got)
	}
	if got[0].Kind != sourceKindPrimary {
		t.Fatalf("primary must be first, got %+v", got[0])
	}
	if got[1].ID != folder.ID || got[1].Kind != sourceKindFolder {
		t.Fatalf("folder source not restored: %+v", got[1])
	}
}

func TestSaveSourcesDoesNotPersistPrimary(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	if err := saveSources(store, []source{primarySource()}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("store should exist: %v", err)
	}
	if strings.Contains(string(data), `"primary"`) {
		t.Fatalf("primary must not be serialized:\n%s", data)
	}
}

func TestAddSourceDedupsByID(t *testing.T) {
	base := []source{primarySource()}
	f := newFolderSource("/tmp/x")
	one := addSource(base, f)
	two := addSource(one, newFolderSource("/tmp/x")) // same path → same ID
	if len(two) != 2 {
		t.Fatalf("adding the same path twice must dedup, got %d sources", len(two))
	}
}

func TestRemoveSourceKeepsPrimary(t *testing.T) {
	all := []source{primarySource(), newFolderSource("/tmp/x")}
	all = removeSource(all, "/tmp/x")
	if len(all) != 1 || all[0].Kind != sourceKindPrimary {
		t.Fatalf("removing a folder should leave [primary], got %+v", all)
	}
	all = removeSource(all, primarySourceID) // must be a no-op
	if len(all) != 1 {
		t.Fatalf("the primary source must not be removable, got %+v", all)
	}
}

func TestLoadSourcesCorruptFileIsPrimaryOnly(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	os.WriteFile(store, []byte("{not json"), 0o644)
	got := loadSources(store)
	if len(got) != 1 || got[0].Kind != sourceKindPrimary {
		t.Fatalf("corrupt store should degrade to [primary], got %+v", got)
	}
}
```

Add `"strings"` to the `source_test.go` imports (for `TestSaveSourcesDoesNotPersistPrimary`). The load/save tests pass an explicit temp `store` path — mirroring `recent_test.go` — so they never touch the real config dir; no env-var juggling.

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'LoadSources|SaveThenLoad|SaveSources|AddSource|RemoveSource' -v`
Expected: FAIL — `undefined: loadSources` etc.

- [ ] **Step 3: Append the store to `source.go`**

Add `"encoding/json"` to the imports, then:

```go
// sourcesFile is the on-disk shape of sources.json: user-added folder sources only.
type sourcesFile struct {
	Sources []source `json:"sources"`
}

// sourcesPath returns the sources store path, or "" if there is no usable user config dir
// (sources then silently reduce to the primary). Mirrors recentPath().
func sourcesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "sources.json")
}

// loadSources returns the primary source (always first) followed by the folder sources
// persisted at path. A missing/corrupt store, or an empty path, yields just [primary].
// Production callers pass sourcesPath(); tests pass a temp path (mirrors loadRecents).
func loadSources(path string) []source {
	out := []source{primarySource()}
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var f sourcesFile
	if json.Unmarshal(data, &f) != nil {
		return out
	}
	for _, s := range f.Sources {
		if s.Kind == sourceKindPrimary { // never trust a stored primary; it is synthesized
			continue
		}
		out = append(out, s)
	}
	return out
}

// saveSources persists ONLY the folder sources from all to path (the primary is synthesized at
// load, never stored). No-ops on an empty path. Production callers pass sourcesPath().
func saveSources(path string, all []source) error {
	if path == "" {
		return nil
	}
	user := make([]source, 0, len(all))
	for _, s := range all {
		if s.Kind == sourceKindPrimary {
			continue
		}
		user = append(user, s)
	}
	data, err := json.MarshalIndent(sourcesFile{Sources: user}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, data, 0o644)
}

// addSource appends s unless a source with the same ID is already present (dedup).
func addSource(all []source, s source) []source {
	for _, e := range all {
		if e.ID == s.ID {
			return all
		}
	}
	return append(all, s)
}

// removeSource drops the source with the given ID. The primary source is never removable.
func removeSource(all []source, id string) []source {
	if id == primarySourceID {
		return all
	}
	out := make([]source, 0, len(all))
	for _, s := range all {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'LoadSources|SaveThenLoad|SaveSources|AddSource|RemoveSource' -v`
Expected: PASS. Full suite `/opt/homebrew/bin/go test ./...` passes, vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add source.go source_test.go
git commit -m "feat: sources.json store (load/save/add/remove; primary synthesized)"
```

---

## Self-Review

**Spec coverage (against `2026-06-30-okashi-library-sources-manifest-design.md` §1):**
- `sourceKind` (primary/folder) + `source` struct (ID/Name/Kind/Path) → Task 1. ✅
- `sources.json` in `os.UserConfigDir()/okashi` persists user-added only; primary synthesized → Task 2. ✅
- Primary always present + non-removable → `removeSource` no-ops on primary; `loadSources` always prepends → Tasks 1-2. ✅
- Per-source isolation (unreachable skipped) → `reachable()` predicate (Task 1); UI skip is step 3. ✅
- Standalone use (folder source / OKASHI_DIR, standalone) → primary honors `writingDir()`/`OKASHI_DIR`; folder sources are plain paths. ✅
- Field-shape alignment with the companion app's Source → JSON tags `id/name/kind/path`. ✅
- NOT in this step (correctly deferred to step 3): the LIBRARY source picker, `◦ Loose`, switching repopulating LIBRARY/FILES, the home layout.

**Type consistency:** `source`/`sourceKind`/`primarySourceID`/`newFolderSource` defined in Task 1, consumed by Task 2's store. `loadSources`/`saveSources` operate on `[]source`; `addSource`/`removeSource` take and return `[]source`. All signatures consistent.

**Placeholder scan:** no TBD/vague steps; every function body is complete. Config-dir isolation is handled the codebase's way — the store functions take the path as a parameter (`recent.go` pattern), so tests pass a temp path and never touch the real `os.UserConfigDir()`; only `sourcesPath()` reads the config dir, and production wires `loadSources(sourcesPath())`.
