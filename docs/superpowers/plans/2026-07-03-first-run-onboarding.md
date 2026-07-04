# First-run onboarding — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On first launch, seed the `demo/the-lighthouse` sample manuscript into an empty writing dir so a new user immediately sees a real project to open/read/export; and make the empty-home state actionable.

**Architecture:** Embed the sample with `//go:embed` (the pattern fonts.go/spell.go already use); a marker-gated, empty-dir-only seed runs once at the top of `initialModel()`. Path-parameterized helpers for testability.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`.
- Seeding is BEST-EFFORT + SAFE: only when the marker is absent AND the writing dir has no projects/`.md`; it never overwrites; it always writes the marker so it runs at most once.
- Writes go through `atomicWrite` (already exists). Config marker lives under `os.UserConfigDir()/okashi/` like the other stores.
- `//go:embed demo/the-lighthouse` embeds the 4 sample files (manifest + 3 chapters). The demo dir exists at build time.
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.
- `initialModel()` is in main.go; it resolves `writingDir()` and builds the home. Tests use `initialModel()` (confirmed test-safe). `TestMain` sets HOME/XDG to a temp dir, so real `UserConfigDir()` writes land in temp during tests.

---

## Task 1: Embed + seed the sample on first run

**Files:** Create `onboarding.go`. Modify `main.go` (one line in `initialModel`). Test: `onboarding_test.go`.

**Interfaces:** Produces `var sampleFS embed.FS`; `func seedMarkerPath() string`; `func maybeSeedSample(writingDir, marker string)`; `func dirIsEmptyish(dir string) bool`; `func writeMarker(marker string) error`.

- [ ] **Step 1: Write the failing tests**

Create `onboarding_test.go`:
```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsEmptyish(t *testing.T) {
	empty := t.TempDir()
	if !dirIsEmptyish(empty) {
		t.Fatal("fresh temp dir should be empty-ish")
	}
	withDoc := t.TempDir()
	os.WriteFile(filepath.Join(withDoc, "a.md"), []byte("x"), 0o644)
	if dirIsEmptyish(withDoc) {
		t.Fatal("a .md present → not empty-ish")
	}
	withDir := t.TempDir()
	os.MkdirAll(filepath.Join(withDir, "proj"), 0o755)
	if dirIsEmptyish(withDir) {
		t.Fatal("a subdir present → not empty-ish")
	}
	dotOnly := t.TempDir()
	os.WriteFile(filepath.Join(dotOnly, ".hidden"), []byte("x"), 0o644)
	if !dirIsEmptyish(dotOnly) {
		t.Fatal("only dotfiles → empty-ish")
	}
}

func TestMaybeSeedSample(t *testing.T) {
	wd := t.TempDir()
	marker := filepath.Join(t.TempDir(), "cfg", ".seeded")
	maybeSeedSample(wd, marker)
	// Sample seeded.
	man := filepath.Join(wd, "the-lighthouse", "manifest.json")
	if _, err := os.Stat(man); err != nil {
		t.Fatalf("manifest should be seeded: %v", err)
	}
	for _, ch := range []string{"01-the-keeper.md", "02-the-fog.md", "03-the-light.md"} {
		if _, err := os.Stat(filepath.Join(wd, "the-lighthouse", ch)); err != nil {
			t.Fatalf("chapter %s should be seeded: %v", ch, err)
		}
	}
	// Marker written.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker should exist after seeding: %v", err)
	}
	// A second call is a no-op (doesn't error / re-seed differently).
	maybeSeedSample(wd, marker)

	// Non-empty dir + fresh marker → no seed, but marker written.
	wd2 := t.TempDir()
	os.WriteFile(filepath.Join(wd2, "existing.md"), []byte("x"), 0o644)
	marker2 := filepath.Join(t.TempDir(), "cfg", ".seeded")
	maybeSeedSample(wd2, marker2)
	if _, err := os.Stat(filepath.Join(wd2, "the-lighthouse")); err == nil {
		t.Fatal("must NOT seed into a non-empty dir")
	}
	if _, err := os.Stat(marker2); err != nil {
		t.Fatal("marker should still be written for a non-empty dir")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run 'TestDirIsEmptyish|TestMaybeSeedSample' -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Create `onboarding.go`**

```go
package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed demo/the-lighthouse
var sampleFS embed.FS

// seedMarkerPath is the once-only first-run marker (UserConfigDir/okashi/.seeded).
func seedMarkerPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", ".seeded")
}

// maybeSeedSample seeds the sample manuscript into writingDir on the very first run — only when the
// dir has no existing projects/documents — then writes the marker so it never runs again. Best-effort.
func maybeSeedSample(writingDir, marker string) {
	if marker == "" {
		return
	}
	if _, err := os.Stat(marker); err == nil {
		return // already ran once
	}
	if !dirIsEmptyish(writingDir) {
		_ = writeMarker(marker) // existing corpus — never seed, but don't re-check every launch
		return
	}
	dst := filepath.Join(writingDir, "the-lighthouse")
	_ = fs.WalkDir(sampleFS, "demo/the-lighthouse", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := sampleFS.ReadFile(p)
		if rerr != nil {
			return nil
		}
		_ = os.MkdirAll(dst, 0o755)
		_ = atomicWrite(filepath.Join(dst, filepath.Base(p)), data, 0o644)
		return nil
	})
	_ = writeMarker(marker)
}

// dirIsEmptyish reports whether dir has no non-dotfile .md files and no subdirectories.
func dirIsEmptyish(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() || strings.HasSuffix(name, ".md") {
			return false
		}
	}
	return true
}

// writeMarker creates the marker file (and its dir). Best-effort.
func writeMarker(marker string) error {
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	return atomicWrite(marker, []byte("1"), 0o644)
}
```

- [ ] **Step 4: Wire into `initialModel`**

At the very top of `initialModel()` (before it resolves the writing dir into the file pane / home), add:
```go
	maybeSeedSample(writingDir(), seedMarkerPath())
```
(Place it as the first statement so the seeded project is present when the home is built. If `initialModel` already calls `writingDir()` once and stores it, reuse that value.)

- [ ] **Step 5: Run + build + vet**

```
/opt/homebrew/bin/gofmt -w onboarding.go main.go onboarding_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: both new tests PASS; full suite green.

- [ ] **Step 6: Commit**

```
git add onboarding.go main.go onboarding_test.go
git commit -m "feat(onboarding): seed the sample manuscript on first run"
```

---

## Task 2: Actionable empty-home affordance

**Files:** Modify `home.go` (the two `(empty)` renders). Test: none (a display string).

- [ ] **Step 1: Replace the two `(empty)` strings**

In `home.go`, the LIBRARY-empty (line ~758) and FILES-empty (line ~787) each `return []string{homeDim("(empty)")}, nil`. Confirm which is which (grep the surrounding function/context), then:
- LIBRARY empty → `homeDim("no projects — + to create")`
- FILES empty → `homeDim("no files — ctrl+n for a doc")`

- [ ] **Step 2: Build + eyeball**

```
/opt/homebrew/bin/gofmt -w home.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
(Optional: `OKASHI_DIR=$(mktemp -d) go run .` to see the affordance — but note first-run will now seed the sample, so test the empty state by deleting the seeded project or pointing at a dir with the marker already set.)

- [ ] **Step 3: Commit**

```
git add home.go
git commit -m "feat(onboarding): actionable empty-home affordance"
```

---

## Self-review notes
- **Spec coverage:** embed+seed → Task 1; empty-home affordance → Task 2. Covered.
- **Safety:** seed is marker-gated + empty-dir-only + never overwrites + best-effort (all errors swallowed). A failure never blocks launch.
- **Type consistency:** `maybeSeedSample(string,string)`, `dirIsEmptyish(string) bool`, `seedMarkerPath() string`, `writeMarker(string) error`, `sampleFS embed.FS` — consistent.
- **Test isolation:** `TestMain` points HOME/XDG at temp, so `seedMarkerPath()` and any real-config writes stay in temp; the tests here pass explicit temp paths regardless.
