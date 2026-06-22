# File Pane: Breadcrumb + Confinement — Implementation Plan (Plan B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Confine the file pane to the okashi workspace root and add a root-relative breadcrumb header.

**Architecture:** `filelist` gains a `root`; `SetDir` clamps to it and `..` only appears below it. The breadcrumb is a `filelist` method, rendered by `main.go`'s sidebar composition (keeping `filelist.View()` as just the list). The header takes one row, so the mouse click→row offset becomes 1.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- Workspace root = `writingDir()`. When `filelist.root != ""`, navigation cannot go above it; when `root == ""` (default), behavior is unchanged (so existing filelist tests that use temp dirs still pass).
- Breadcrumb is relative to root: `filepath.Base(root)` at the top (e.g. `okashi`), each subdir joined by `" / "`.
- The breadcrumb header occupies one row; the file-list mouse offset is therefore `1` (not 0), reflected in BOTH `layout` (list height) and the click mapping.
- Launch screen's `homeOpenOther` label becomes **"Browse all files"** (still roots at `writingDir()`).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Confine navigation to the workspace root

**Files:**
- Modify: `filelist.go` (`root` field, `SetDir` clamp, `..` gating, `withinRoot` helper), `main.go` (`initialModel` sets root), `home.go` ("Browse all files" label)
- Test: `filelist_test.go`

**Interfaces:**
- Produces: `filelist.root string`; `func withinRoot(dir, root string) bool`; `SetDir` clamps to `root` when set; `..` present only when `dir != root`.

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go`:

```go
func TestFilelistConfinedToRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "novel")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f := newFilelist()
	f.root = root

	// At root: no ".." entry.
	f.SetDir(root)
	if f.has("..") {
		t.Fatal("root should not show a .. entry")
	}
	// In a subdir: ".." present.
	f.SetDir(sub)
	if !f.has("..") {
		t.Fatal("subdir should show a .. entry")
	}
	// Trying to go above root clamps back to root.
	f.SetDir(filepath.Dir(root))
	if f.dir != root {
		t.Fatalf("navigating above root should clamp to root, got %q", f.dir)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestFilelistConfinedToRoot -v 2>&1 | tail`
Expected: FAIL — `f.root` unknown / clamp not implemented.

- [ ] **Step 3: Add the `root` field and `withinRoot`**

In `filelist.go`, add to the `filelist` struct:

```go
	root string
```

Add the helper:

```go
// withinRoot reports whether dir is root or a descendant of it.
func withinRoot(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
```

- [ ] **Step 4: Clamp `SetDir` and gate `..`**

In `filelist.SetDir`, at the very top (before `f.dir = dir`):

```go
	if f.root != "" && !withinRoot(dir, f.root) {
		dir = f.root
	}
```

Replace the existing `..` block:

```go
	if parent := filepath.Dir(dir); parent != dir {
		f.entries = append(f.entries, fileEntry{name: "..", isDir: true})
	}
```

with:

```go
	showParent := filepath.Dir(dir) != dir // not at filesystem root
	if f.root != "" {
		showParent = dir != f.root // confined: only below the workspace root
	}
	if showParent {
		f.entries = append(f.entries, fileEntry{name: "..", isDir: true})
	}
```

- [ ] **Step 5: Set the root in `initialModel`; rename the launch action**

In `main.go` `initialModel`, after `fl := newFilelist()` and before `fl.SetDir(writingDir())`:

```go
	fl.root = writingDir()
```

In `home.go` `buildHomeItems`, change the final item label:

```go
	items = append(items, homeItem{kind: homeOpenOther, label: "Browse all files"})
```

- [ ] **Step 6: Run the test, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFilelistConfinedToRoot|TestBuildHomeItems' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w filelist.go main.go home.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go main.go home.go filelist_test.go
git commit -m "Confine file pane to the okashi workspace root"
```

Note: `TestBuildHomeItems` asserts the open-other item — if it checks the label, update that assertion to "Browse all files" in the same commit.

---

## Task 2: Breadcrumb header

**Files:**
- Modify: `filelist.go` (`breadcrumb` method), `styles.go` (`breadcrumbStyle`), `main.go` (sidebar composition in `View`, `layout` height, mouse offset)
- Test: `filelist_test.go`, `smoke_test.go`

**Interfaces:**
- Produces: `func (f filelist) breadcrumb() string`; `breadcrumbStyle` (styles.go); the sidebar renders the breadcrumb above the list; mouse offset is `1`.

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go`:

```go
func TestBreadcrumb(t *testing.T) {
	root := "/home/me/okashi"
	cases := []struct {
		dir  string
		want string
	}{
		{"/home/me/okashi", "okashi"},
		{"/home/me/okashi/Book Name", "okashi / Book Name"},
		{"/home/me/okashi/Essays/Drafts", "okashi / Essays / Drafts"},
	}
	for _, c := range cases {
		f := filelist{root: root, dir: c.dir}
		if got := f.breadcrumb(); got != c.want {
			t.Fatalf("breadcrumb(%q) = %q, want %q", c.dir, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestBreadcrumb -v 2>&1 | tail`
Expected: build error — `f.breadcrumb undefined`.

- [ ] **Step 3: Implement `breadcrumb` + style**

In `filelist.go`:

```go
// breadcrumb is the current path relative to the workspace root, e.g.
// "okashi" at the root or "okashi / Book Name" inside a project.
func (f filelist) breadcrumb() string {
	base := filepath.Base(f.root)
	rel, err := filepath.Rel(f.root, f.dir)
	if err != nil || rel == "." || rel == "" {
		return base
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return base + " / " + strings.Join(parts, " / ")
}
```

In `styles.go`, after `selectedStyle`:

```go
var breadcrumbStyle = lipgloss.NewStyle().
	Foreground(accent).
	Bold(true)
```

- [ ] **Step 4: Render the header and adjust layout + mouse offset**

In `main.go` `View`, the sidebar block currently is:

```go
		side := sidebarStyle.
			Width(sidebarWidth - 2).
			Height(bodyH - 2).
			Render(m.files.View())
```

Replace with (header above the list):

```go
		sideInner := lipgloss.JoinVertical(
			lipgloss.Left,
			breadcrumbStyle.Render(ansi.Truncate(m.files.breadcrumb(), sidebarWidth-2, "…")),
			m.files.View(),
		)
		side := sidebarStyle.
			Width(sidebarWidth - 2).
			Height(bodyH - 2).
			Render(sideInner)
```

(Add `"github.com/charmbracelet/x/ansi"` to `main.go` imports if not present.)

In `layout`, where it sets the file-list height for the sidebar, reduce by one for the header row:

```go
		m.files.height = bodyH - 3 // sidebar content height (bodyH-2) minus the breadcrumb row
```

(It currently reads `m.files.height = bodyH - 2`.)

In the `tea.MouseMsg` click branch, change the offset from 0 to 1 (the breadcrumb row sits above the list):

```go
		// Breadcrumb header occupies row 0; the file list starts at row 1.
		row := sidebarRow(msg.Y, 1, m.files.height)
```

(It currently reads `sidebarRow(msg.Y, 0, m.files.height)`.)

- [ ] **Step 5: Add a mouse-offset regression test**

Add to `smoke_test.go`:

```go
func TestSidebarClickRowAccountsForBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("hi words"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.root = dir
	m.files.SetDir(dir) // entries: ["draft.md"] (no ".." at root)

	// Row 0 of the list is at screen Y=1 (breadcrumb is Y=0). Click it.
	click := tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	nm, _ = m.Update(click)
	m = nm.(model)
	if m.files.entries[m.files.selected].name != "draft.md" {
		t.Fatalf("click at Y=1 should select the first list row, got %q", m.files.entries[m.files.selected].name)
	}
}
```

- [ ] **Step 6: Run the tests, full suite, build, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestBreadcrumb|TestSidebarClickRow' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w filelist.go styles.go main.go filelist_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add filelist.go styles.go main.go filelist_test.go smoke_test.go
git commit -m "File pane breadcrumb header (mouse offset accounts for it)"
```

---

## Task 3: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the docs**

In `README.md`, update the file-pane / launch text to mention the breadcrumb + workspace confinement. After the existing sidebar paragraph, add:

```markdown
The file pane is confined to your okashi workspace folder — a breadcrumb at the
top of the pane (`okashi / Book Name`) shows where you are, and you can't browse
above the workspace. "Browse all files" on the launch screen returns to the
workspace root.
```

- [ ] **Step 2: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Docs: file-pane breadcrumb + workspace confinement"
```

- [ ] **Step 4: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` → open a project; confirm the breadcrumb reads `okashi / <project>`, navigating up stops at the workspace root (no `..` at the top), clicking a file still selects the right row (the breadcrumb offset is correct), and "Browse all files" returns to the root.

---

## Self-Review

**Spec coverage (Plan B scope — spec Section 3):**
- Confinement (root clamp, `..` only below root, "Browse all files") → Task 1.
- Breadcrumb header relative to root → Task 2.
- Mouse offset accounts for the header (both `layout` height and click mapping) → Task 2 (with a regression test).
- Docs → Task 3.

**Placeholder scan:** none — every code step shows full code.

**Type consistency:** `filelist.root`, `withinRoot`, `breadcrumb()`, `breadcrumbStyle` used consistently. The mouse offset change (0→1) and the `layout` height change (`bodyH-2`→`bodyH-3`) are both in Task 2, matching the one-row header — no split dependency. `root == ""` preserves prior behavior, so only the new confinement test exercises clamping.

**Cross-cutting check:** the breadcrumb adds one row; Task 2 updates the list height AND the click offset together, so clicks stay aligned (a `TestSidebarClickRowAccountsForBreadcrumb` regression locks it).
