# Structural Editing — Chunk 1: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the shared structural-editing engine — pure manifest transforms (`manifestInsert`/`manifestRemove`/`manifestReorder`) plus the file-system moves (`safeMove`, `moveDocument`, `moveFolder`) — that structure mode (chunk 2) and the file mover (chunk 3) will both call.

**Architecture:** Pure `items`-slice transforms live in `manifest.go` beside the existing reader/writer. The file-system moves live in a new `move.go`; they compose the transforms with `safeMove` + the existing atomic `writeManifest` (read-modify-write per manifest). No UI and no user-reachable behavior change in this chunk — it is dormant capability the next two chunks wire up.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), existing `manifest.go` (`manifest`, `manifestItem`, `readManifest`, `writeManifest`, `hasManifest`), `sectionTitle` (`project.go`), `withinRoot` (`filelist.go`), `atomicWrite` (`atomicwrite.go`).

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`** and gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **Manifest schema is EXACTLY v1** — these ops only reorder/insert/remove `items`; no field/shape change.
- **All manifest writes go through the existing `writeManifest`** (atomic, sorted-keys, no HTML escaping, no trailing newline — byte-shape parity with the companion app). Never marshal a manifest by hand.
- **Read-modify-write:** any op that edits an *existing* manifest re-reads it immediately before writing.
- **Pure transforms never mutate their argument** — they return a new `manifest` value with a fresh `Items` slice.
- **Refuse, don't guess:** a move refuses a name collision in the destination and refuses to touch a manuscript whose manifest is present-but-unreadable (never auto-rename, never infer structure).
- **No new third-party dependency;** default build stays pure-Go.
- **No user-facing change in this chunk.** The shared-contract flip ("structural edits planned → shipped, confirm-gated") rides with chunk 2 (structure mode), the first user-reachable structural edit — do NOT edit `CLAUDE.md` or the companion app's repo here.

---

### Task 1: Pure manifest transforms (`manifestInsert` / `manifestRemove` / `manifestReorder`)

**Files:**
- Modify: `manifest.go` (append after `renameChapterTitle`, line ~135)
- Test: `manifest_writers_test.go` (append)

**Interfaces:**
- Consumes: `manifest`, `manifestItem` (`manifest.go`).
- Produces:
  - `func manifestInsert(m manifest, file, title string, at int) manifest`
  - `func manifestRemove(m manifest, file string) manifest`
  - `func manifestReorder(m manifest, file string, to int) manifest`

- [ ] **Step 1: Write the failing tests**

Append to `manifest_writers_test.go`:

```go
func mf(files ...string) manifest {
	m := manifest{SchemaVersion: 1, Title: "T"}
	for _, f := range files {
		m.Items = append(m.Items, manifestItem{File: f, Title: f})
	}
	return m
}

func files(m manifest) []string {
	var out []string
	for _, it := range m.Items {
		out = append(out, it.File)
	}
	return out
}

func eqStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestManifestInsert(t *testing.T) {
	orig := mf("a", "b", "c")
	got := manifestInsert(orig, "x", "X", 1)
	if !eqStr(files(got), []string{"a", "x", "b", "c"}) {
		t.Fatalf("insert at 1: %v", files(got))
	}
	if got.Items[1].Title != "X" {
		t.Fatalf("inserted title = %q", got.Items[1].Title)
	}
	// clamp
	if !eqStr(files(manifestInsert(orig, "x", "X", 99)), []string{"a", "b", "c", "x"}) {
		t.Fatal("insert past end should append")
	}
	if !eqStr(files(manifestInsert(orig, "x", "X", -5)), []string{"x", "a", "b", "c"}) {
		t.Fatal("insert before start should prepend")
	}
	// no mutation of the argument
	if !eqStr(files(orig), []string{"a", "b", "c"}) {
		t.Fatalf("insert mutated its argument: %v", files(orig))
	}
}

func TestManifestRemove(t *testing.T) {
	orig := mf("a", "b", "c")
	if !eqStr(files(manifestRemove(orig, "b")), []string{"a", "c"}) {
		t.Fatal("remove b")
	}
	if !eqStr(files(manifestRemove(orig, "zzz")), []string{"a", "b", "c"}) {
		t.Fatal("removing an absent file should be a no-op")
	}
	if !eqStr(files(orig), []string{"a", "b", "c"}) {
		t.Fatal("remove mutated its argument")
	}
}

func TestManifestReorder(t *testing.T) {
	orig := mf("a", "b", "c", "d")
	// move c (index 2) up one → index 1
	if !eqStr(files(manifestReorder(orig, "c", 1)), []string{"a", "c", "b", "d"}) {
		t.Fatalf("reorder c up: %v", files(manifestReorder(orig, "c", 1)))
	}
	// move c down one → index 3 (in the post-removal list)
	if !eqStr(files(manifestReorder(orig, "c", 3)), []string{"a", "b", "d", "c"}) {
		t.Fatalf("reorder c down: %v", files(manifestReorder(orig, "c", 3)))
	}
	// absent → no-op
	if !eqStr(files(manifestReorder(orig, "zzz", 0)), []string{"a", "b", "c", "d"}) {
		t.Fatal("reorder absent should be a no-op")
	}
	if !eqStr(files(orig), []string{"a", "b", "c", "d"}) {
		t.Fatal("reorder mutated its argument")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'ManifestInsert|ManifestRemove|ManifestReorder' -v`
Expected: FAIL — `undefined: manifestInsert` etc.

- [ ] **Step 3: Implement the transforms**

Append to `manifest.go`:

```go
// manifestInsert returns a copy of m with a new {file,title} item inserted at index at (clamped to
// [0,len]). Callers ensure file is not already listed. The argument is not mutated.
func manifestInsert(m manifest, file, title string, at int) manifest {
	if at < 0 {
		at = 0
	}
	if at > len(m.Items) {
		at = len(m.Items)
	}
	items := make([]manifestItem, 0, len(m.Items)+1)
	items = append(items, m.Items[:at]...)
	items = append(items, manifestItem{File: file, Title: title})
	items = append(items, m.Items[at:]...)
	m.Items = items
	return m
}

// manifestRemove returns a copy of m without the item whose File == file (no-op if absent). The
// argument is not mutated.
func manifestRemove(m manifest, file string) manifest {
	items := make([]manifestItem, 0, len(m.Items))
	for _, it := range m.Items {
		if it.File != file {
			items = append(items, it)
		}
	}
	m.Items = items
	return m
}

// manifestReorder returns a copy of m with the item File==file moved to index to (clamped) in the
// list AFTER the item is removed. No-op if file isn't listed. The argument is not mutated.
func manifestReorder(m manifest, file string, to int) manifest {
	from := -1
	for i, it := range m.Items {
		if it.File == file {
			from = i
			break
		}
	}
	if from < 0 {
		return m
	}
	moved := m.Items[from]
	rest := make([]manifestItem, 0, len(m.Items)-1)
	for i, it := range m.Items {
		if i != from {
			rest = append(rest, it)
		}
	}
	if to < 0 {
		to = 0
	}
	if to > len(rest) {
		to = len(rest)
	}
	out := make([]manifestItem, 0, len(m.Items))
	out = append(out, rest[:to]...)
	out = append(out, moved)
	out = append(out, rest[to:]...)
	m.Items = out
	return m
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'ManifestInsert|ManifestRemove|ManifestReorder' -v`
Expected: PASS. Then `/opt/homebrew/bin/go vet ./...` clean and `/opt/homebrew/bin/gofmt -l manifest.go manifest_writers_test.go` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add manifest.go manifest_writers_test.go
git commit -m "feat: pure manifest transforms (insert/remove/reorder)"
```

---

### Task 2: `safeMove` (rename with cross-volume fallback)

**Files:**
- Create: `move.go`
- Test: `move_test.go` (create)

**Interfaces:**
- Consumes: `atomicWrite` (`atomicwrite.go`); stdlib `os`, `errors`, `syscall`.
- Produces: `func safeMove(src, dst string) error`.

- [ ] **Step 1: Write the failing test**

Create `move_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeMoveSameVolume(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	dst := filepath.Join(dir, "sub", "a.md")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safeMove(src, dst); err != nil {
		t.Fatalf("safeMove: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be gone after a move")
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "hello" {
		t.Fatalf("dest content = %q err=%v", b, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestSafeMove -v`
Expected: FAIL — `undefined: safeMove`.

- [ ] **Step 3: Implement `safeMove`**

Create `move.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// safeMove moves a file from src to dst. It tries os.Rename (fast, same-volume) and, only on a
// cross-device error, falls back to copy-then-remove. dst's parent directory must already exist.
func safeMove(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// Cross-volume: copy the bytes atomically to dst, then remove the source.
	data, rerr := os.ReadFile(src)
	if rerr != nil {
		return rerr
	}
	if werr := atomicWrite(dst, data, 0o644); werr != nil {
		return werr
	}
	return os.Remove(src)
}
```

(The `fmt` and `filepath` imports are used by Task 3/4, which append to this file; if the linter complains about unused imports in this task's intermediate state, add `fmt`/`filepath` when Task 3 lands. To keep Task 2 self-contained and building, import ONLY `errors`, `os`, `syscall` now, and add `fmt`/`filepath` in Task 3.)

Corrected imports for THIS task (Task 2 only):

```go
import (
	"errors"
	"os"
	"syscall"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `/opt/homebrew/bin/go test ./... -run TestSafeMove -v`
Expected: PASS. `/opt/homebrew/bin/go vet ./...` clean; `/opt/homebrew/bin/gofmt -l move.go move_test.go` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add move.go move_test.go
git commit -m "feat: safeMove (rename + cross-volume copy fallback)"
```

---

### Task 3: `moveDocument` (move a file + manifest edits on both sides)

**Files:**
- Modify: `move.go` (add `moveDocument`; add `fmt`, `path/filepath` to the imports)
- Test: `move_test.go` (append)

**Interfaces:**
- Consumes: `safeMove` (Task 2); `readManifest`, `writeManifest`, `hasManifest`, `manifestInsert`, `manifestRemove` (Tasks 1 + existing); `sectionTitle` (`project.go`).
- Produces: `func moveDocument(srcDir, file, dstDir string, asChapter bool) error`.

**Context:** "Was it a chapter?" = its filename appears in `srcDir`'s manifest `items`. "Is the dest a manuscript?" = `hasManifest(dstDir)`. The file moves FIRST; then the manifests are edited (re-read immediately before each write).

- [ ] **Step 1: Write the failing tests**

Append to `move_test.go`:

```go
func TestMoveDocumentLooseToCategory(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "note.md"), []byte("x"), 0o644)
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)
	if err := moveDocument(root, "note.md", cat, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cat, "note.md")); err != nil {
		t.Fatalf("file should have moved: %v", err)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsChapter(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "deleted-scene.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled") // one chapter: 01-untitled.md
	if err := moveDocument(root, "deleted-scene.md", proj, true); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	last := m.Items[len(m.Items)-1]
	if last.File != "deleted-scene.md" {
		t.Fatalf("moved file should be appended as a chapter, items=%+v", m.Items)
	}
	if last.Title != "deleted scene" { // sectionTitle de-slugs the filename
		t.Fatalf("chapter title = %q, want de-slugged 'deleted scene'", last.Title)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsResource(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "res.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled")
	before, _, _ := readManifest(proj)
	if err := moveDocument(root, "res.md", proj, false); err != nil {
		t.Fatal(err)
	}
	after, _, _ := readManifest(proj)
	if len(after.Items) != len(before.Items) {
		t.Fatalf("resource move must NOT change items: before=%d after=%d", len(before.Items), len(after.Items))
	}
	if _, err := os.Stat(filepath.Join(proj, "res.md")); err != nil {
		t.Fatalf("file should be in the folder as a resource: %v", err)
	}
}

func TestMoveDocumentChapterOutRemovesFromManifest(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	first, _ := createManuscript(proj, "Novel", "Untitled") // first == 01-untitled.md
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)
	if err := moveDocument(proj, first, cat, false); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	for _, it := range m.Items {
		if it.File == first {
			t.Fatal("chapter should have been removed from the source manifest")
		}
	}
	if _, err := os.Stat(filepath.Join(cat, first)); err != nil {
		t.Fatalf("file should have moved to the category: %v", err)
	}
}

func TestMoveDocumentRefusesCollisionAndNoop(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	os.MkdirAll(a, 0o755)
	os.MkdirAll(b, 0o755)
	os.WriteFile(filepath.Join(a, "x.md"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(b, "x.md"), []byte("2"), 0o644)
	if err := moveDocument(a, "x.md", b, false); err == nil {
		t.Fatal("a name collision in the destination must be refused")
	}
	if err := moveDocument(a, "x.md", a, false); err == nil {
		t.Fatal("moving into the same folder must be refused")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run TestMoveDocument -v`
Expected: FAIL — `undefined: moveDocument`.

- [ ] **Step 3: Implement `moveDocument`**

First, extend `move.go`'s imports to:

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)
```

Then add:

```go
// moveDocument relocates document `file` from srcDir to dstDir. It moves the file, removes it from
// the source manifest if it was a chapter there, and — if dstDir is a manuscript and asChapter is
// true — appends it to the destination manifest; otherwise it lands as a loose Resource. Refuses a
// no-op (same folder), a destination name collision, and an unreadable manifest on either side.
func moveDocument(srcDir, file, dstDir string, asChapter bool) error {
	if srcDir == dstDir {
		return fmt.Errorf("source and destination are the same folder")
	}
	// Refuse to touch a manuscript whose manifest is present-but-unreadable (don't guess).
	if _, present, err := readManifest(srcDir); present && err != nil {
		return fmt.Errorf("source manifest unreadable: %w", err)
	}
	if _, present, err := readManifest(dstDir); present && err != nil {
		return fmt.Errorf("destination manifest unreadable: %w", err)
	}
	dst := filepath.Join(dstDir, file)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", file)
	}

	// Was it a listed chapter of the source manuscript?
	wasChapter := false
	if sm, present, err := readManifest(srcDir); err == nil && present {
		for _, it := range sm.Items {
			if it.File == file {
				wasChapter = true
				break
			}
		}
	}

	// Move the file first, so a failed move never leaves dangling manifest edits.
	if err := safeMove(filepath.Join(srcDir, file), dst); err != nil {
		return err
	}

	// Source manifest: drop the chapter (read-modify-write).
	if wasChapter {
		if sm, present, err := readManifest(srcDir); err == nil && present {
			if err := writeManifest(srcDir, manifestRemove(sm, file)); err != nil {
				return err
			}
		}
	}

	// Destination manifest: append as a chapter when requested and the dest is a manuscript.
	if asChapter && hasManifest(dstDir) {
		dm, present, err := readManifest(dstDir)
		if err != nil {
			return err
		}
		if present {
			if err := writeManifest(dstDir, manifestInsert(dm, file, sectionTitle(file), len(dm.Items))); err != nil {
				return err
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestMoveDocument -v` then the full suite `/opt/homebrew/bin/go test ./...`. vet + gofmt clean on `move.go`/`move_test.go`.

- [ ] **Step 5: Commit**

```bash
git add move.go move_test.go
git commit -m "feat: moveDocument (file move + manifest insert/remove on boundary crossings)"
```

---

### Task 4: `moveFolder` (move a directory, manifest rides along)

**Files:**
- Modify: `move.go` (add `moveFolder`)
- Test: `move_test.go` (append)

**Interfaces:**
- Consumes: `withinRoot` (`filelist.go`); stdlib `os`, `path/filepath`, `fmt` (already imported after Task 3).
- Produces: `func moveFolder(srcDir, dstParent string) error`.

**Context:** `moveFolder` moves the whole directory `srcDir` INTO `dstParent` (becoming `dstParent/base(srcDir)`), a plain `os.Rename` — a manuscript's `manifest.json` travels inside the folder untouched. `withinRoot(dstParent, srcDir)` reports whether `dstParent` is `srcDir` or a descendant (an illegal move-into-itself).

- [ ] **Step 1: Write the failing tests**

Append to `move_test.go`:

```go
func TestMoveFolderManuscriptRidesAlong(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled")
	trilogy := filepath.Join(root, "trilogy")
	os.MkdirAll(trilogy, 0o755)

	if err := moveFolder(proj, trilogy); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(trilogy, "novel")
	if !hasManifest(moved) {
		t.Fatal("the manuscript's manifest should ride along into the new location")
	}
	m, _, err := readManifest(moved)
	if err != nil || m.Title != "Novel" {
		t.Fatalf("manifest should read back intact: title=%q err=%v", m.Title, err)
	}
	if _, err := os.Stat(proj); !os.IsNotExist(err) {
		t.Fatal("the old folder location should be gone")
	}
}

func TestMoveFolderRefusesCollisionAndSelf(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	os.MkdirAll(filepath.Join(a, "sub"), 0o755)
	dst := filepath.Join(root, "dst")
	os.MkdirAll(filepath.Join(dst, "a"), 0o755) // collision: dst/a already exists

	if err := moveFolder(a, dst); err == nil {
		t.Fatal("a name collision must be refused")
	}
	if err := moveFolder(a, filepath.Join(a, "sub")); err == nil {
		t.Fatal("moving a folder into its own descendant must be refused")
	}
	if err := moveFolder(a, filepath.Dir(a)); err == nil {
		t.Fatal("moving a folder into the parent it already lives in must be refused")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run TestMoveFolder -v`
Expected: FAIL — `undefined: moveFolder`.

- [ ] **Step 3: Implement `moveFolder`**

Add to `move.go`:

```go
// moveFolder moves the directory srcDir INTO dstParent (as dstParent/base(srcDir)) via os.Rename.
// A manuscript's manifest.json rides along inside the folder. Refuses a name collision, moving a
// folder into itself or a descendant, and a no-op (it already lives in dstParent).
func moveFolder(srcDir, dstParent string) error {
	base := filepath.Base(srcDir)
	dst := filepath.Join(dstParent, base)
	if srcDir == dst {
		return fmt.Errorf("%s already lives there", base)
	}
	if withinRoot(dstParent, srcDir) {
		return fmt.Errorf("can't move a folder into itself")
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", base)
	}
	return os.Rename(srcDir, dst)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run TestMoveFolder -v` then the full suite `/opt/homebrew/bin/go test ./...`. vet + gofmt clean.

- [ ] **Step 5: Commit**

```bash
git add move.go move_test.go
git commit -m "feat: moveFolder (os.Rename with self/collision guards)"
```

---

## Self-Review

**Spec coverage (against `2026-07-01-structural-editing-file-mover-design.md` §1):**
- `manifestInsert` / `manifestRemove` / `manifestReorder` (pure, no-mutation, clamped) → Task 1. ✅
- `moveDocument` — file move + src-remove-if-chapter + dst-insert-if-manuscript-and-asChapter; refuse no-op / collision / unreadable manifest; read-modify-write → Task 3. ✅
- `moveFolder` — os.Rename, manifest rides along, refuse collision / self-descendant → Task 4. ✅
- `safeMove` — rename + cross-volume copy-then-remove fallback → Task 2. ✅
- NOT in this chunk (correctly): structure mode (chunk 2), the mover (chunk 3), and the shared-contract flip (rides with chunk 2 — this chunk is dormant capability).

**Placeholder scan:** every step has complete code. The one hazard is Task 2's imports (a bare `safeMove` uses only `errors`/`os`/`syscall`, while `fmt`/`filepath` arrive with Task 3) — called out explicitly so the implementer imports the minimal set in Task 2 and extends in Task 3, keeping every intermediate commit compiling.

**Type consistency:** `manifestInsert(m, file, title, at)`, `manifestRemove(m, file)`, `manifestReorder(m, file, to)` defined in Task 1 and consumed by `moveDocument` (Task 3). `safeMove(src, dst)` (Task 2) consumed by `moveDocument`. `moveDocument(srcDir, file, dstDir, asChapter)` / `moveFolder(srcDir, dstParent)` signatures match the spec §1 and the chunk-2/3 plans will consume them. `sectionTitle(file)` returns the de-slugged title used in the append; `withinRoot(dstParent, srcDir)` is the existing helper (dir-or-descendant).
