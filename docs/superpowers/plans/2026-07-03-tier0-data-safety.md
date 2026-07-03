# Tier-0 data safety — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close okashi's silent-data-loss paths: save on quit, save on chapter switch, a per-session backup snapshot, and an external-change (iCloud/other-device) guard that never overwrites a changed file.

**Architecture:** All four fixes are localized to `main.go`'s `save()`, `loadFile()`, and one global `ctrl+c` intercept in `Update()`. No new screens; `atomicWrite` and `saveGoals` already exist.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`.
- Autosave flushes only after 2 s idle (`autosaveDue`, main.go:462) — that's the window these fixes close.
- Every write goes through `atomicWrite` (temp+rename). Backups + conflict copies use it too. Backups are BEST-EFFORT — a backup failure must never block or fail the actual save.
- Backups live in a `.okashi-bak/` sibling dir (dot-prefixed → already excluded from the pane and manuscript detection). Conflict copies are `<name>.conflict-<ts><ext>` siblings.
- Facts: `save()` and `loadFile()` are `*model` methods in main.go; `atomicWrite(path, data, perm)` in atomicwrite.go; `saveGoals(goalsPath(), m.goalsAll)`; `m.dirty`/`m.currentFile`/`m.editor.Value()`; `strings` and `time` are already imported in main.go.
- After every task: `/opt/homebrew/bin/gofmt -w main.go`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.

---

## Task 1: Save on quit (global ctrl+c intercept)

**Files:** Modify `main.go` (add `saveIfDirty`; add a global `ctrl+c` intercept before the screen dispatch). Test: `datasafety_test.go` (new).

**Interfaces:** Produces `func (m *model) saveIfDirty()`.

- [ ] **Step 1: Write the failing test**

Create `datasafety_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// editorModelAt builds a model whose editor holds `content` for `path`, marked dirty.
func dirtyModel(t *testing.T, path, content string) model {
	t.Helper()
	m := initialModel()
	m.loadFile(path) // sets currentFile + loadedMtime
	m.editor.SetValue(content)
	m.dirty = true
	return m
}

func TestSaveIfDirtyFlushes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("old"), 0o644)
	m := dirtyModel(t, p, "new content")
	m.saveIfDirty()
	got, _ := os.ReadFile(p)
	if string(got) != "new content" {
		t.Fatalf("saveIfDirty should flush the buffer, disk = %q", got)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestSaveIfDirtyFlushes -v`
Expected: FAIL (`saveIfDirty` undefined).

- [ ] **Step 3: Add `saveIfDirty` + the global ctrl+c intercept**

Add the helper (near `save()`):
```go
// saveIfDirty flushes unsaved work before the app exits.
func (m *model) saveIfDirty() {
	if m.dirty && m.currentFile != "" {
		m.save()
	}
	if m.goalsAll != nil {
		saveGoals(goalsPath(), m.goalsAll)
	}
}
```
In `Update`, insert a global intercept immediately BEFORE the `// Global help overlay` block (so ctrl+c always flushes+quits, from any screen/state, even with help showing):
```go
	// Global quit: flush unsaved work first, from any screen.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.saveIfDirty()
		return m, tea.Quit
	}
```
The per-site `case "ctrl+c": return m, tea.Quit` handlers are now unreachable — leave them (harmless).

- [ ] **Step 4: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w main.go datasafety_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add main.go datasafety_test.go
git commit -m "fix(data-safety): flush unsaved work on ctrl+c quit"
```

---

## Task 2: Save on switch (loadFile guard)

**Files:** Modify `main.go` (`loadFile`). Test: `datasafety_test.go`.

**Interfaces:** Consumes `save()`. No new symbols.

- [ ] **Step 1: Write the failing test**

Append to `datasafety_test.go`:
```go
func TestLoadFileFlushesOutgoingBuffer(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	os.WriteFile(a, []byte("a-orig"), 0o644)
	os.WriteFile(b, []byte("b-orig"), 0o644)
	m := initialModel()
	m.loadFile(a)
	m.editor.SetValue("a-edited")
	m.dirty = true
	m.loadFile(b) // switching away must flush a first
	if got, _ := os.ReadFile(a); string(got) != "a-edited" {
		t.Fatalf("switching chapters must save the outgoing buffer, a = %q", got)
	}
	if m.editor.Value() != "b-orig" {
		t.Fatalf("after switch the editor should show b, got %q", m.editor.Value())
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestLoadFileFlushesOutgoingBuffer -v`
Expected: FAIL (a.md still "a-orig" — the edit was dropped).

- [ ] **Step 3: Add the guard at the top of `loadFile`**

```go
func (m *model) loadFile(path string) {
	if m.dirty && m.currentFile != "" && m.currentFile != path {
		m.save() // flush the outgoing buffer before clobbering it
	}
	data, err := os.ReadFile(path)
	// … rest unchanged …
}
```

- [ ] **Step 4: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w main.go datasafety_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add main.go datasafety_test.go
git commit -m "fix(data-safety): flush the outgoing buffer when switching files"
```

---

## Task 3: Backup-before-save (timestamped ring, once per file per session)

**Files:** Modify `main.go` (`save()`; add `snapshotBackup`/`pruneBackups`; model field `backedUp`; const `backupKeep`). Test: `datasafety_test.go`.

**Interfaces:** Produces `func snapshotBackup(path string)`, `func pruneBackups(dir, base string, keep int)`; model field `backedUp map[string]bool`; const `backupKeep = 10`.

- [ ] **Step 1: Write the failing test**

Append to `datasafety_test.go`:
```go
func TestBackupSnapshotOncePerSession(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("v1"), 0o644)
	m := initialModel()
	m.loadFile(p)
	m.editor.SetValue("v2")
	m.dirty = true
	m.save() // first save: snapshots the pre-edit "v1"
	bakDir := filepath.Join(dir, ".okashi-bak")
	entries, _ := os.ReadDir(bakDir)
	if len(entries) != 1 {
		t.Fatalf("first save should create exactly one snapshot, got %d", len(entries))
	}
	if b, _ := os.ReadFile(filepath.Join(bakDir, entries[0].Name())); string(b) != "v1" {
		t.Fatalf("snapshot should hold the pre-edit content, got %q", b)
	}
	m.editor.SetValue("v3")
	m.dirty = true
	m.save() // second save this session: no new snapshot
	entries2, _ := os.ReadDir(bakDir)
	if len(entries2) != 1 {
		t.Fatalf("second same-session save should not add a snapshot, got %d", len(entries2))
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestBackupSnapshotOncePerSession -v`
Expected: FAIL (no `.okashi-bak`).

- [ ] **Step 3: Add the backup helpers + field, wire into `save()`**

Add:
```go
const backupKeep = 10 // timestamped snapshots retained per file

// snapshotBackup copies the current on-disk file into <dir>/.okashi-bak/<base>.<YYYYMMDD-HHMMSS>,
// then keeps only the newest backupKeep snapshots for that base. Best-effort — never blocks a save.
func snapshotBackup(path string) {
	data, err := os.ReadFile(path)
	if err != nil { // new/unreadable file → nothing to back up
		return
	}
	bakDir := filepath.Join(filepath.Dir(path), ".okashi-bak")
	if os.MkdirAll(bakDir, 0o755) != nil {
		return
	}
	base := filepath.Base(path)
	_ = atomicWrite(filepath.Join(bakDir, base+"."+time.Now().Format("20060102-150405")), data, 0o644)
	pruneBackups(bakDir, base, backupKeep)
}

// pruneBackups removes all but the newest `keep` snapshots named "<base>.*" in dir.
func pruneBackups(dir, base string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var snaps []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), base+".") {
			snaps = append(snaps, e.Name())
		}
	}
	if len(snaps) <= keep {
		return
	}
	sort.Strings(snaps) // timestamp suffix sorts chronologically; oldest first
	for _, name := range snaps[:len(snaps)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
```
Add the model field near the other transient maps:
```go
	backedUp map[string]bool // files snapshotted this session
```
In `save()`, immediately before the `atomicWrite(m.currentFile, …)` call:
```go
	if m.backedUp == nil {
		m.backedUp = map[string]bool{}
	}
	if !m.backedUp[m.currentFile] {
		snapshotBackup(m.currentFile)
		m.backedUp[m.currentFile] = true
	}
```
(Confirm `sort` is imported in main.go; add it if not.)

- [ ] **Step 4: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w main.go datasafety_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```
git add main.go datasafety_test.go
git commit -m "feat(data-safety): per-session timestamped backup snapshot before save"
```

---

## Task 4: External-change guard (never overwrite a file changed on disk)

**Files:** Modify `main.go` (`loadFile` records mtime; `save()` checks it and diverts on conflict; model field `loadedMtime`). Test: `datasafety_test.go`.

**Interfaces:** Model field `loadedMtime map[string]time.Time`.

- [ ] **Step 1: Write the failing test**

Append to `datasafety_test.go`:
```go
func TestExternalChangeDivertsToConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("orig"), 0o644)
	m := initialModel()
	m.loadFile(p) // records loadedMtime
	m.editor.SetValue("my edits")
	m.dirty = true
	// Simulate an external change: rewrite p with a strictly newer mtime.
	os.WriteFile(p, []byte("external version"), 0o644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(p, future, future)
	m.save()
	// The original on disk must NOT be overwritten.
	if got, _ := os.ReadFile(p); string(got) != "external version" {
		t.Fatalf("save must not clobber the externally-changed file, got %q", got)
	}
	// A conflict copy must hold our edits, and currentFile must repoint to it.
	matches, _ := filepath.Glob(filepath.Join(dir, "ch.conflict-*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected one conflict copy, got %d", len(matches))
	}
	if b, _ := os.ReadFile(matches[0]); string(b) != "my edits" {
		t.Fatalf("conflict copy should hold our edits, got %q", b)
	}
	if m.currentFile != matches[0] {
		t.Fatalf("currentFile should repoint to the conflict copy, got %q", m.currentFile)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestExternalChangeDivertsToConflict -v`
Expected: FAIL (original clobbered / no conflict copy).

- [ ] **Step 3: Record mtime in `loadFile`**

Add the model field:
```go
	loadedMtime map[string]time.Time // file → mtime at load, for the external-change guard
```
In `loadFile`, after `m.currentFile = path`:
```go
	if m.loadedMtime == nil {
		m.loadedMtime = map[string]time.Time{}
	}
	if fi, err := os.Stat(path); err == nil {
		m.loadedMtime[path] = fi.ModTime()
	}
```

- [ ] **Step 4: Guard + divert in `save()`**

At the START of `save()`'s write logic (after the `currentFile == ""` guard, BEFORE the backup snapshot from Task 3):
```go
	// External-change guard: never overwrite a file that changed on disk since we loaded it.
	if fi, err := os.Stat(m.currentFile); err == nil {
		if loaded, ok := m.loadedMtime[m.currentFile]; ok && fi.ModTime().After(loaded) {
			ext := filepath.Ext(m.currentFile)
			confl := strings.TrimSuffix(m.currentFile, ext) + ".conflict-" + time.Now().Format("20060102-150405") + ext
			if werr := atomicWrite(confl, []byte(m.editor.Value()), 0o644); werr != nil {
				m.status = "save failed (conflict): " + werr.Error()
				return
			}
			m.status = "⚠ " + filepath.Base(m.currentFile) + " changed on disk — your edits saved to " + filepath.Base(confl)
			m.currentFile = confl
			if m.loadedMtime == nil {
				m.loadedMtime = map[string]time.Time{}
			}
			if cfi, err := os.Stat(confl); err == nil {
				m.loadedMtime[confl] = cfi.ModTime()
			}
			m.dirty = false
			addRecent(recentPath(), confl)
			return
		}
	}
```
After a normal (non-conflict) successful write — i.e. after `m.dirty = false` in `save()` — refresh the recorded mtime so our own write doesn't look like an external change next time:
```go
	if m.loadedMtime == nil {
		m.loadedMtime = map[string]time.Time{}
	}
	if fi, err := os.Stat(m.currentFile); err == nil {
		m.loadedMtime[m.currentFile] = fi.ModTime()
	}
```

- [ ] **Step 5: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w main.go datasafety_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: all four data-safety tests PASS; full suite green.

- [ ] **Step 6: Commit**

```
git add main.go datasafety_test.go
git commit -m "feat(data-safety): guard against overwriting externally-changed files"
```

---

## Self-review notes
- **Spec coverage:** save-on-quit → T1; save-on-switch → T2; backup ring → T3; external-change guard → T4. All covered.
- **Type consistency:** `saveIfDirty()`, `snapshotBackup(string)`, `pruneBackups(string,string,int)`, `backupKeep`, model fields `backedUp map[string]bool` + `loadedMtime map[string]time.Time` — used consistently.
- **save() ordering (T3+T4 both edit save()):** mtime-guard FIRST (return on conflict, don't touch the original) → backup snapshot → atomicWrite → dirty=false → refresh loadedMtime. Apply T3's backup block first; T4 inserts its guard ABOVE it and the refresh BELOW the existing `m.dirty = false`.
- **Best-effort backups:** `snapshotBackup`/`pruneBackups` swallow all errors — a backup problem never fails or blocks the real save.
- **Test model construction:** the tests call `initialModel()` then `loadFile`. If `initialModel()` proves unsuitable in a test (it resolves a writing dir + detects theme), fall back to whatever the existing tests use to get an editor-bearing model — verify against the current test suite before relying on it (Task 1 implementer: confirm this first).
