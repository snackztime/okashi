# Atomic Writes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Replace every in-place `os.WriteFile` in okashi with an atomic temp-file + fsync + rename helper, so a crash or concurrent reader never sees a truncated file (the `CLAUDE.md` "never write in place" invariant).

**Architecture:** One helper `atomicWrite(path, data, perm)` writes to a dot-prefixed temp file in the target's own directory, fsyncs, chmods, and renames over the target; all six `os.WriteFile` call sites switch to it. The existing two-phase `os.Rename` renumber is already atomic and is left alone.

**Tech Stack:** Go stdlib (`os`, `path/filepath`).

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt` (not on PATH).
- The temp file is created in the **same directory** as the target (so `os.Rename` is a same-volume atomic op) and is **dot-prefixed** (`.<base>.tmp-*`) so the file pane never lists it and it's never mistaken for a section/loose file; it is removed on every error path.
- fsync the temp before rename. Preserve the intended mode (`0o644`).
- Leave the existing `os.Rename` sites (`applyRenames`, the rename feature) unchanged — already atomic.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: The `atomicWrite` helper (`atomicwrite.go`)

**Files:**
- Create: `atomicwrite.go`, `atomicwrite_test.go`

**Interfaces:**
- Produces (package `main`): `func atomicWrite(path string, data []byte, perm os.FileMode) error`.

- [ ] **Step 1: Write the failing tests**

Create `atomicwrite_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteCreatesFileWithMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	if err := atomicWrite(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hello" {
		t.Fatalf("content = %q, err %v", b, err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", fi.Mode().Perm())
	}
}

func TestAtomicWriteOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	os.WriteFile(p, []byte("old longer contents"), 0o644)
	if err := atomicWrite(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "new" {
		t.Fatalf("overwrite content = %q, want new", b)
	}
}

func TestAtomicWriteLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	if err := atomicWrite(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "f.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only f.md, got %v (temp left behind?)", names)
	}
}

func TestAtomicWriteErrorsOnMissingDir(t *testing.T) {
	if err := atomicWrite(filepath.Join(t.TempDir(), "nope", "f.md"), []byte("x"), 0o644); err == nil {
		t.Fatal("expected an error writing into a non-existent directory")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestAtomicWrite 2>&1 | tail`
Expected: build error — `atomicWrite` undefined.

- [ ] **Step 3: Implement `atomicwrite.go`**

```go
package main

import (
	"os"
	"path/filepath"
)

// atomicWrite writes data to path atomically: it writes to a temp file in the SAME
// directory, fsyncs it, chmods it, then renames it over path. A crash or a concurrent
// reader never sees a truncated file. The temp is dot-prefixed (hidden from the file pane)
// and removed on any error path.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // harmless no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
```

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run TestAtomicWrite -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w atomicwrite.go atomicwrite_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add atomicwrite.go atomicwrite_test.go
git commit -m "atomicwrite: temp-file + fsync + rename helper"
```

---

## Task 2: Route every write through `atomicWrite`

**Files:**
- Modify: `main.go` (`save`), `export.go` (×2), `backup.go`, `outline.go` (`commitInsert`), `recent.go`

**Interfaces:**
- Consumes: `atomicWrite` (Task 1).

- [ ] **Step 1: Replace the six `os.WriteFile` call sites**

Each is a one-line swap (`os.WriteFile(` → `atomicWrite(`), arguments unchanged:

`main.go` — `save()` (the editor save):
```go
	if err := atomicWrite(m.currentFile, []byte(m.editor.Value()), 0o644); err != nil {
```

`export.go` — the RTF and PDF writes:
```go
	if err := atomicWrite(rtfPath, writeRTF(doc, st, meta), 0o644); err != nil {
```
```go
	if err := atomicWrite(pdfPath, pdfBytes, 0o644); err != nil {
```

`backup.go` — the snapshot copies:
```go
		if err := atomicWrite(filepath.Join(dest, filepath.Base(p)), data, 0o644); err != nil {
```

`outline.go` — `commitInsert` (the new empty section file):
```go
	if err := atomicWrite(filepath.Join(dir, newName), nil, 0o644); err != nil {
```

`recent.go` — the recents store:
```go
	_ = atomicWrite(path, data, 0o644)
```

- [ ] **Step 2: Fix any now-unused imports**

After the swap, run `/opt/homebrew/bin/go vet ./... 2>&1 | tail`. If any file no longer uses `os` (none should — `backup.go` still calls `os.ReadFile`/`os.MkdirAll`, `export.go` `os.MkdirAll`/`os.ReadFile`, `outline.go` `os.Rename`/`os.Stat`/`os.ReadDir`, `main.go` and `recent.go` use `os` elsewhere), remove the unused import. Expected: nothing to remove.

- [ ] **Step 3: Run the full suite**

Run: `/opt/homebrew/bin/go test ./... 2>&1 | tail -4`
Expected: PASS — the existing save/export/backup/insert tests still pass because `atomicWrite` produces the identical final file. (`TestExportSingleDocFromEditor`, `TestExportWholeManuscriptFromOutline`, the outline insert tests, and the backup tests all read the resulting files and are the regression net.)

- [ ] **Step 4: gofmt; build; commit**

```bash
/opt/homebrew/bin/gofmt -w main.go export.go backup.go outline.go recent.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go export.go backup.go outline.go recent.go
git commit -m "atomicwrite: route save, export, backup, new-section, and recents through atomicWrite"
```

---

## Self-Review

**Spec coverage:** the helper (temp in same dir, dot-prefixed, fsync, chmod, rename, remove-on-error) → Task 1; all five spec call sites (save, export ×2, backup, commitInsert, recents) → Task 2; the `os.Rename` renumber left as-is (not in either task) → matches "out of scope: leave existing renames."

**Placeholder scan:** none — full code in both tasks.

**Type consistency:** `atomicWrite(path string, data []byte, perm os.FileMode) error` defined in Task 1, called with exactly that signature at all six sites in Task 2.

**Regression net:** Task 2 Step 3 leans on the existing export/insert/backup tests (which read the produced files) to confirm `atomicWrite` is a behavior-preserving swap; Task 1's own tests prove the atomicity/overwrite/no-temp/error properties. No `os.WriteFile` should remain in non-test code after Task 2 (the implementer can confirm with `grep -rn "os.WriteFile" *.go | grep -v _test`).
