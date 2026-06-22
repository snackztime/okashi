# Typewriter Scrolling + Mouse File Pane — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add WordGrinder-style typewriter scrolling (caret pinned to vertical center, toggle with `ctrl+t`) and replace the keyboard-only filepicker with an owned `filelist` component supporting wheel/click/double-click.

**Architecture:** Vendor `bubbles/textarea` into `internal/textarea/` with a minimal additive `Typewriter` patch (top blank-row padding + center offset). Build a self-contained `filelist` in the `main` package and route `tea.MouseMsg` to it from the root model.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, vendored Bubbles textarea (cursor/key/runeutil/viewport subpackages stay external).

## Global Constraints

- Module path: `okashi` (import vendored editor as `okashi/internal/textarea`).
- Go toolchain: invoke as `/opt/homebrew/bin/go` (not on PATH for non-login shells).
- Vendored textarea is pinned to upstream **v0.20.0**; every vendored file gets a header comment recording the version and the local patch.
- Allowed writing extensions (filelist filter): `.md`, `.txt`, `.wg`, `.markdown`.
- `sidebarWidth = 32` (from `styles.go`); palette vars `accent`, `subtle` already exist.
- Run `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## File Structure

- **Create** `internal/textarea/textarea.go` — vendored copy of upstream textarea.go + Typewriter patch.
- **Create** `internal/textarea/memoization/memoization.go` — vendored memoization subpackage (stdlib-only).
- **Create** `internal/textarea/typewriter_test.go` — centering math tests.
- **Create** `filelist.go` — owned file-list component (state + methods + View).
- **Create** `filelist_test.go` — filelist unit tests.
- **Modify** `main.go` — editor type swap, `ctrl+t` toggle, replace filepicker with filelist, `tea.MouseMsg` handling.
- **Modify** `styles.go` — `selectedStyle` for the file-list highlight.
- **Modify** `README.md` — keys table + file-pane docs + roadmap.
- **Modify** `go.mod`/`go.sum` — drop `bubbles/filepicker` via `go mod tidy`.

---

## Task 1: Vendor the textarea package (no behavior change)

Pure vendoring refactor (TDD exception: copied code). Deliverable: okashi compiles against `internal/textarea` with all existing tests green.

**Files:**
- Create: `internal/textarea/textarea.go`, `internal/textarea/memoization/memoization.go`
- Modify: `main.go` (import + type references)

**Interfaces:**
- Produces: package `textarea` at `okashi/internal/textarea` with the same exported API as `bubbles/textarea` (`New`, `Model`, `SetValue`, `Value`, `Focus`, `Blur`, `SetWidth`, `SetHeight`, `Line`, `CursorUp`, `CursorDown`, `Blink`, `FocusedStyle`, `BlurredStyle`, etc.).

- [ ] **Step 1: Copy the package files from the module cache**

```bash
cd /Users/michael/dev/okashi
MOD=$(/opt/homebrew/bin/go env GOMODCACHE)
SRC="$MOD/github.com/charmbracelet/bubbles@v0.20.0/textarea"
mkdir -p internal/textarea/memoization
cp "$SRC/textarea.go" internal/textarea/textarea.go
cp "$SRC/memoization/memoization.go" internal/textarea/memoization/memoization.go
chmod u+w internal/textarea/textarea.go internal/textarea/memoization/memoization.go
```

- [ ] **Step 2: Rewrite the memoization import path inside textarea.go**

```bash
/usr/bin/sed -i '' \
  's#github.com/charmbracelet/bubbles/textarea/memoization#okashi/internal/textarea/memoization#' \
  internal/textarea/textarea.go
```

- [ ] **Step 3: Add a provenance header to both vendored files**

Prepend to `internal/textarea/textarea.go` (above `package textarea`):

```go
// Vendored from github.com/charmbracelet/bubbles/textarea @ v0.20.0.
// Local patch: adds the exported `Typewriter` field, `ViewportYOffset()`,
// and centered-scroll rendering (see comments marked "okashi:typewriter").
// Re-sync deliberately when bumping Bubbles.
```

Prepend to `internal/textarea/memoization/memoization.go`:

```go
// Vendored from github.com/charmbracelet/bubbles/textarea/memoization @ v0.20.0.
// Unmodified.
```

- [ ] **Step 4: Point okashi at the vendored package**

In `main.go`, change the import:

```go
// remove: "github.com/charmbracelet/bubbles/textarea"
"okashi/internal/textarea"
```

The type reference `textarea.Model` and call `textarea.New()` stay identical (same package name). No other code changes.

- [ ] **Step 5: Tidy, build, and run the existing suite**

```bash
/opt/homebrew/bin/go mod tidy
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
```
Expected: build ok; `ok  okashi` and `ok  okashi/internal/textarea` (the vendored test file `textarea_test.go` was NOT copied, so only okashi tests run) — all green, no behavior change.

- [ ] **Step 6: Commit**

```bash
git add internal/textarea main.go go.mod go.sum
git commit -m "Vendor bubbles/textarea into internal/textarea (no behavior change)"
```

---

## Task 2: Typewriter patch in the vendored textarea

**Files:**
- Modify: `internal/textarea/textarea.go` (field, `repositionView`, two render sites, getter)
- Test: `internal/textarea/typewriter_test.go`

**Interfaces:**
- Produces: exported field `Typewriter bool` on `textarea.Model`; method `func (m Model) ViewportYOffset() int`. When `Typewriter` is true, after `View()` the caret's wrapped row is centered: `ViewportYOffset()` equals `cursorLineNumber()` (clamped ≥ 0).

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/typewriter_test.go`:

```go
package textarea

import (
	"fmt"
	"strings"
	"testing"
)

// build a 40-line, narrow-content textarea so each logical line is one row
// (cursorLineNumber == Line()).
func newTA() Model {
	ta := New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(40)
	ta.SetHeight(10) // viewport height 10, center row = 5
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	ta.SetValue(strings.Join(lines, "\n"))
	return ta
}

func TestTypewriterCentersCaret(t *testing.T) {
	ta := newTA()
	ta.Typewriter = true

	// Move caret to the top, then to a few known rows, and assert the offset
	// centers that row (offset == row, since lines don't wrap).
	for i := 0; i < 40; i++ {
		ta.CursorUp()
	}
	for _, want := range []int{0, 10, 20} {
		for ta.Line() < want {
			ta.CursorDown()
		}
		_ = ta.View() // centering is applied during render
		if got := ta.ViewportYOffset(); got != want {
			t.Fatalf("caret on line %d: YOffset = %d, want %d", ta.Line(), got, want)
		}
	}
}

func TestTypewriterOffDoesNotCenter(t *testing.T) {
	ta := newTA()
	ta.Typewriter = false
	for i := 0; i < 40; i++ {
		ta.CursorUp()
	}
	_ = ta.View()
	if got := ta.ViewportYOffset(); got != 0 {
		t.Fatalf("typewriter off, caret at top: YOffset = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run TestTypewriter -v 2>&1 | tail -8
```
Expected: build error — `ta.Typewriter undefined` and `ta.ViewportYOffset undefined`.

- [ ] **Step 3: Add the field and getter**

In `internal/textarea/textarea.go`, add to the `Model` struct (near the other exported fields like `Cursor`):

```go
	// Typewriter, when true, pins the caret's row to the vertical center of
	// the viewport instead of edge-anchoring it. okashi:typewriter
	Typewriter bool
```

Add a getter near `func (m Model) Line()`:

```go
// ViewportYOffset returns the current vertical scroll offset of the internal
// viewport. okashi:typewriter
func (m Model) ViewportYOffset() int {
	return m.viewport.YOffset
}
```

- [ ] **Step 4: Skip edge-anchoring when typewriter is on**

In `repositionView` (starts `func (m *Model) repositionView() {`), insert as the first line of the body:

```go
	if m.Typewriter {
		return // centering is applied in renderViewport during View. okashi:typewriter
	}
```

- [ ] **Step 5: Add the centering render helper and call it at both render sites**

Add this method (place it just below `repositionView`):

```go
// renderViewport sets the viewport content and returns the rendered view. When
// Typewriter is on it prepends Height/2 blank rows so the caret's wrapped row
// can sit at screen-center (the buffer already pads Height end-of-buffer rows
// below, covering the bottom). okashi:typewriter
func (m Model) renderViewport(content string) string {
	if m.Typewriter && m.height > 0 {
		pad := m.height / 2
		m.viewport.SetContent(strings.Repeat("\n", pad) + content)
		m.viewport.SetYOffset(m.cursorLineNumber())
	} else {
		m.viewport.SetContent(content)
	}
	return m.style.Base.Render(m.viewport.View())
}
```

Replace BOTH occurrences of these two lines:

```go
	m.viewport.SetContent(s.String())
	return m.style.Base.Render(m.viewport.View())
```

with:

```go
	return m.renderViewport(s.String())
```

(There are exactly two — around `textarea.go:1196` and `:1293`.)

- [ ] **Step 6: Run the tests to verify they pass**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run TestTypewriter -v 2>&1 | tail -8
```
Expected: both PASS.

- [ ] **Step 7: gofmt, full build/test, commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/typewriter_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea
git commit -m "textarea: add Typewriter centered-scroll patch + tests"
```

---

## Task 3: Wire the typewriter toggle into okashi (ctrl+t, default on)

**Files:**
- Modify: `main.go` (model field, initialModel, ctrl+t case, status hint)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `textarea.Model.Typewriter` (Task 2).
- Produces: `model.typewriter bool`; `ctrl+t` flips it and mirrors it onto `m.editor.Typewriter`.

- [ ] **Step 1: Write the failing test**

Append to `smoke_test.go`:

```go
func TestTypewriterToggle(t *testing.T) {
	m := initialModel()
	if !m.typewriter || !m.editor.Typewriter {
		t.Fatal("typewriter should default on")
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = nm.(model)
	if m.typewriter || m.editor.Typewriter {
		t.Fatal("ctrl+t should turn typewriter off")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	m = nm.(model)
	if !m.typewriter || !m.editor.Typewriter {
		t.Fatal("ctrl+t should turn typewriter back on")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
/opt/homebrew/bin/go test . -run TestTypewriterToggle -v 2>&1 | tail -6
```
Expected: build error — `m.typewriter undefined`.

- [ ] **Step 3: Add the model field**

In the `model` struct, next to `previewing bool`:

```go
	typewriter bool
```

- [ ] **Step 4: Default it on in initialModel**

In `initialModel`, after `ta.FocusedStyle.CursorLine = lipgloss.NewStyle()`:

```go
	ta.Typewriter = true // typewriter scrolling on by default; ctrl+t toggles
```

And add `typewriter: true,` to the returned `model{...}` literal. Update the status hint string to include `ctrl+t typewriter`:

```go
		status:         "ctrl+b sidebar · tab switch · ctrl+n new · ctrl+p preview · ctrl+t typewriter · ctrl+s save · ctrl+c quit",
```

- [ ] **Step 5: Add the ctrl+t case**

In `Update`'s `tea.KeyMsg` switch, after the `ctrl+p` case:

```go
		case "ctrl+t":
			m.typewriter = !m.typewriter
			m.editor.Typewriter = m.typewriter
			if m.typewriter {
				m.status = "typewriter on"
			} else {
				m.status = "typewriter off"
			}
			return m, nil
```

- [ ] **Step 6: Run the test, gofmt, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestTypewriterToggle -v 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Add ctrl+t typewriter toggle (default on)"
```

---

## Task 4: filelist — directory read, filter, sort, render

**Files:**
- Create: `filelist.go`, `filelist_test.go`
- Modify: `styles.go` (`selectedStyle`)

**Interfaces:**
- Produces:
  - `type fileEntry struct { name string; isDir bool }`
  - `type filelist struct { dir string; entries []fileEntry; selected, offset, width, height int; allowed map[string]bool }`
  - `func newFilelist() filelist`
  - `func (f *filelist) SetDir(dir string)`
  - `func (f filelist) View() string`

- [ ] **Step 1: Write the failing test**

Create `filelist_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilelistReadsFiltersSorts(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "b.md"), "x")
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	mustWrite(t, filepath.Join(dir, "skip.png"), "x") // wrong extension
	if err := os.Mkdir(filepath.Join(dir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}

	f := newFilelist()
	f.SetDir(dir)

	var names []string
	for _, e := range f.entries {
		names = append(names, e.name)
	}
	// ".." first (temp dir has a parent), then dir, then files alpha; .png skipped.
	want := []string{"..", "chapters", "a.txt", "b.md"}
	if len(names) != len(want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entries = %v, want %v", names, want)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
/opt/homebrew/bin/go test . -run TestFilelistReadsFiltersSorts -v 2>&1 | tail -6
```
Expected: build error — `newFilelist undefined`.

- [ ] **Step 3: Implement filelist.go**

Create `filelist.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type fileEntry struct {
	name  string
	isDir bool
}

// filelist is a minimal, mouse-friendly file browser we fully own.
type filelist struct {
	dir      string
	entries  []fileEntry
	selected int
	offset   int // index of the top visible row
	width    int
	height   int
	allowed  map[string]bool
}

func newFilelist() filelist {
	return filelist{
		width:  sidebarWidth - 2,
		height: 1,
		allowed: map[string]bool{
			".md": true, ".txt": true, ".wg": true, ".markdown": true,
		},
	}
}

// SetDir loads dir's entries (filtered, sorted dirs-first) and resets the cursor.
func (f *filelist) SetDir(dir string) {
	f.dir = dir
	f.entries = nil
	f.selected = 0
	f.offset = 0

	if parent := filepath.Dir(dir); parent != dir {
		f.entries = append(f.entries, fileEntry{name: "..", isDir: true})
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var dirs, files []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue // hidden
		}
		if it.IsDir() {
			dirs = append(dirs, fileEntry{name: name, isDir: true})
			continue
		}
		if f.allowed[strings.ToLower(filepath.Ext(name))] {
			files = append(files, fileEntry{name: name})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	f.entries = append(f.entries, dirs...)
	f.entries = append(f.entries, files...)
}

// View renders the visible window of entries, highlighting the selection.
func (f filelist) View() string {
	if len(f.entries) == 0 {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty)")
	}
	end := f.offset + f.height
	if end > len(f.entries) {
		end = len(f.entries)
	}
	var b strings.Builder
	for i := f.offset; i < end; i++ {
		e := f.entries[i]
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		label = ansi.Truncate(label, f.width, "…")
		if i == f.selected {
			b.WriteString(selectedStyle.Width(f.width).Render(label))
		} else {
			b.WriteString(label)
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Add the highlight style**

In `styles.go`, after `statusStyle`:

```go
var selectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(accent)
```

- [ ] **Step 5: Run the test to verify it passes**

```bash
/opt/homebrew/bin/go test . -run TestFilelistReadsFiltersSorts -v 2>&1 | tail -4
```
Expected: PASS.

- [ ] **Step 6: gofmt, build, commit**

```bash
/opt/homebrew/bin/go mod tidy   # pulls x/ansi to a direct dep
/opt/homebrew/bin/gofmt -w filelist.go filelist_test.go styles.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go filelist_test.go styles.go go.mod go.sum
git commit -m "filelist: directory read/filter/sort + render"
```

---

## Task 5: filelist — navigation, scroll window, activate

**Files:**
- Modify: `filelist.go`
- Test: `filelist_test.go`

**Interfaces:**
- Produces:
  - `func (f *filelist) moveBy(n int)` — moves selection, clamped, scrolls into view.
  - `func (f *filelist) selectRow(visibleRow int)` — sets selection from a visible-row index.
  - `func (f *filelist) activate() (path string, ok bool)` — dir/`..` navigate (ok=false); file returns its absolute path (ok=true).

- [ ] **Step 1: Write the failing tests**

Append to `filelist_test.go`:

```go
func TestFilelistNavigationAndScroll(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}

	f.moveBy(-1) // clamp at 0
	if f.selected != 0 {
		t.Fatalf("selected = %d, want 0", f.selected)
	}
	f.moveBy(4) // to last; window should scroll
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4", f.selected)
	}
	if f.offset != 2 { // height 3 → window [2,3,4]
		t.Fatalf("offset = %d, want 2", f.offset)
	}
	f.moveBy(10) // clamp at last
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4 (clamped)", f.selected)
	}
}

func TestFilelistSelectRow(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.offset = 2
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}
	f.selectRow(1) // offset 2 + row 1 = index 3
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3", f.selected)
	}
	f.selectRow(99) // out of range: ignored
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3 (unchanged)", f.selected)
	}
}

func TestFilelistActivate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "x")
	f := newFilelist()
	f.SetDir(dir)

	// Select the file (entries: "..", "note.md") and activate it.
	f.selected = 1
	path, ok := f.activate()
	if !ok || path != filepath.Join(dir, "note.md") {
		t.Fatalf("activate file = (%q, %v), want (%q, true)", path, ok, filepath.Join(dir, "note.md"))
	}

	// Activating ".." navigates up and opens nothing.
	f.SetDir(dir)
	f.selected = 0
	if _, ok := f.activate(); ok {
		t.Fatal("activating .. should not open a file")
	}
	if f.dir != filepath.Dir(dir) {
		t.Fatalf("after .. dir = %q, want %q", f.dir, filepath.Dir(dir))
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
/opt/homebrew/bin/go test . -run 'TestFilelistNavigation|TestFilelistSelectRow|TestFilelistActivate' -v 2>&1 | tail -8
```
Expected: build error — `f.moveBy undefined` (etc.).

- [ ] **Step 3: Implement the methods**

Append to `filelist.go`:

```go
func (f *filelist) moveBy(n int) {
	if len(f.entries) == 0 {
		return
	}
	f.selected += n
	if f.selected < 0 {
		f.selected = 0
	}
	if f.selected >= len(f.entries) {
		f.selected = len(f.entries) - 1
	}
	f.scrollIntoView()
}

func (f *filelist) scrollIntoView() {
	if f.selected < f.offset {
		f.offset = f.selected
	} else if f.height > 0 && f.selected >= f.offset+f.height {
		f.offset = f.selected - f.height + 1
	}
	if f.offset < 0 {
		f.offset = 0
	}
}

// selectRow sets the selection from a row index within the visible window.
func (f *filelist) selectRow(visibleRow int) {
	if visibleRow < 0 {
		return
	}
	idx := f.offset + visibleRow
	if idx >= len(f.entries) {
		return
	}
	f.selected = idx
}

// activate acts on the selected entry: directories (and "..") navigate and
// return ok=false; a file returns its absolute path with ok=true.
func (f *filelist) activate() (string, bool) {
	if len(f.entries) == 0 {
		return "", false
	}
	e := f.entries[f.selected]
	if e.isDir {
		if e.name == ".." {
			f.SetDir(filepath.Dir(f.dir))
		} else {
			f.SetDir(filepath.Join(f.dir, e.name))
		}
		return "", false
	}
	return filepath.Join(f.dir, e.name), true
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
/opt/homebrew/bin/go test . -run 'TestFilelistNavigation|TestFilelistSelectRow|TestFilelistActivate' -v 2>&1 | tail -8
```
Expected: all PASS.

- [ ] **Step 5: gofmt, full suite, commit**

```bash
/opt/homebrew/bin/gofmt -w filelist.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go filelist_test.go
git commit -m "filelist: navigation, scroll window, activate"
```

---

## Task 6: Replace the filepicker with filelist (keyboard parity)

**Files:**
- Modify: `main.go` (field, initialModel, Update routing, View, layout), `go.mod`/`go.sum`
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: all of `filelist` (Tasks 4–5).
- Produces: `model.files filelist`; keyboard nav routes to it; opening a file calls `loadFile`.

- [ ] **Step 1: Write the failing test**

Append to `smoke_test.go`:

```go
func TestFilelistOpensFileFromSidebar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("hello world words"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.SetDir(dir)

	// Select the file (".." then "draft.md") and press enter.
	m.files.selected = 1
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)

	if m.currentFile != path {
		t.Fatalf("currentFile = %q, want %q", m.currentFile, path)
	}
	if m.focus != focusEditor {
		t.Fatal("opening a file should move focus to the editor")
	}
}
```

This test requires the file pane to have focus. `initialModel` already sets `focus: focusSidebar`, so it holds.

- [ ] **Step 2: Run to verify it fails**

```bash
/opt/homebrew/bin/go test . -run TestFilelistOpensFileFromSidebar -v 2>&1 | tail -6
```
Expected: build error — `m.files undefined`.

- [ ] **Step 3: Swap the struct field**

In `main.go`, in the `model` struct, replace:

```go
	filepicker filepicker.Model
```

with:

```go
	files filelist
```

Remove the `"github.com/charmbracelet/bubbles/filepicker"` import.

- [ ] **Step 4: Update initialModel**

Replace the filepicker setup block:

```go
	fp := filepicker.New()
	fp.CurrentDirectory = writingDir()
	fp.DirAllowed = true
	fp.FileAllowed = true
	fp.ShowHidden = false
	fp.AutoHeight = false
	fp.AllowedTypes = []string{".md", ".txt", ".wg", ".markdown"}
```

with:

```go
	fl := newFilelist()
	fl.SetDir(writingDir())
```

In the returned `model{...}`, replace `filepicker: fp,` with `files: fl,`.

- [ ] **Step 5: Replace Init**

`Init` currently returns `m.filepicker.Init()`. The filelist has no init command:

```go
func (m model) Init() tea.Cmd {
	return nil
}
```

- [ ] **Step 6: Replace the sidebar routing in Update**

Replace this block:

```go
	} else if m.focus == focusSidebar && m.sidebarVisible {
		m.filepicker, cmd = m.filepicker.Update(msg)
		cmds = append(cmds, cmd)
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			m.loadFile(path)
			m.focus = focusEditor
			m.editor.Focus()
		}
	} else {
```

with:

```go
	} else if m.focus == focusSidebar && m.sidebarVisible {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "up", "k":
				m.files.moveBy(-1)
			case "down", "j":
				m.files.moveBy(1)
			case "enter", "right", "l":
				if path, ok := m.files.activate(); ok {
					m.loadFile(path)
					m.focus = focusEditor
					m.editor.Focus()
				}
			case "left", "h", "backspace":
				m.files.SetDir(filepath.Dir(m.files.dir))
			}
		}
	} else {
```

- [ ] **Step 7: Update View and layout**

In `View`, replace the filepicker render:

```go
			Render(m.filepicker.View())
```

with:

```go
			Render(m.files.View())
```

In `layout`, replace `m.filepicker.Height = bodyH - 4` with:

```go
		m.files.height = bodyH - 2
		m.files.width = sidebarWidth - 2
```

- [ ] **Step 8: Tidy, build, run target test + full suite**

```bash
/opt/homebrew/bin/go mod tidy   # drops bubbles/filepicker
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go test . -run TestFilelistOpensFileFromSidebar -v 2>&1 | tail -4
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
```
Expected: target test PASS; full suite green; build ok.

- [ ] **Step 9: Commit**

```bash
git add main.go go.mod go.sum
git commit -m "Replace filepicker with owned filelist (keyboard parity)"
```

---

## Task 7: Mouse handling — wheel, click-select+focus, double-click open

**Files:**
- Modify: `main.go` (mouse fields, coord helper, `tea.MouseMsg` branch, imports)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `filelist.moveBy/selectRow/activate`.
- Produces: `func sidebarRow(mouseY, bannerH, listHeight int) int`; model fields `lastClickRow int`, `lastClickTime time.Time`; a `tea.MouseMsg` branch in `Update`.

- [ ] **Step 1: Write the failing tests**

Append to `smoke_test.go` (add `"time"` to the test file's imports):

```go
func TestSidebarRowMapping(t *testing.T) {
	// banner 4 rows tall, list 10 rows. Y=4 -> row 0; Y=13 -> row 9; Y=3 -> -1.
	if got := sidebarRow(4, 4, 10); got != 0 {
		t.Fatalf("sidebarRow(4,4,10) = %d, want 0", got)
	}
	if got := sidebarRow(13, 4, 10); got != 9 {
		t.Fatalf("sidebarRow(13,4,10) = %d, want 9", got)
	}
	if got := sidebarRow(3, 4, 10); got != -1 {
		t.Fatalf("sidebarRow above list = %d, want -1", got)
	}
	if got := sidebarRow(14, 4, 10); got != -1 {
		t.Fatalf("sidebarRow below list = %d, want -1", got)
	}
}

func TestMouseWheelScrollsFileList(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.files.entries = []fileEntry{{name: "a"}, {name: "b"}, {name: "c"}}
	m.files.selected = 0

	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.files.selected != 1 {
		t.Fatalf("wheel down: selected = %d, want 1", m.files.selected)
	}
	if m.focus != focusSidebar {
		t.Fatal("wheel over the pane should focus it")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebarRowMapping|TestMouseWheel' -v 2>&1 | tail -8
```
Expected: build error — `sidebarRow undefined`.

- [ ] **Step 3: Add imports and model fields**

In `main.go`, add `"time"` to the import block. Add to the `model` struct (near `status`):

```go
	lastClickRow  int
	lastClickTime time.Time
```

- [ ] **Step 4: Add the coordinate helper**

Add near `writingDir` in `main.go`:

```go
// sidebarRow maps an absolute mouse Y to a row index within the file list, or
// -1 if the click is outside the list. The list starts just below the banner.
func sidebarRow(mouseY, bannerH, listHeight int) int {
	row := mouseY - bannerH
	if row < 0 || row >= listHeight {
		return -1
	}
	return row
}
```

- [ ] **Step 5: Add the MouseMsg branch**

In `Update`, add a case to the top-level `switch msg := msg.(type)` (alongside `tea.WindowSizeMsg` and `tea.KeyMsg`):

```go
	case tea.MouseMsg:
		if !m.sidebarVisible || msg.X >= sidebarWidth {
			return m, nil // editor-pane mouse is out of scope
		}
		bannerH := lipgloss.Height(bannerView(m.width))
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.focus = focusSidebar
			m.editor.Blur()
			m.files.moveBy(-1)
		case tea.MouseButtonWheelDown:
			m.focus = focusSidebar
			m.editor.Blur()
			m.files.moveBy(1)
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			row := sidebarRow(msg.Y, bannerH, m.files.height)
			if row < 0 {
				return m, nil
			}
			m.focus = focusSidebar
			m.editor.Blur()
			m.files.selectRow(row)
			now := time.Now()
			if row == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
				if path, ok := m.files.activate(); ok {
					m.loadFile(path)
					m.focus = focusEditor
					m.editor.Focus()
				}
				m.lastClickTime = time.Time{} // consume the double-click
			} else {
				m.lastClickRow = row
				m.lastClickTime = now
			}
		}
		return m, nil
```

- [ ] **Step 6: Run target tests, gofmt, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebarRowMapping|TestMouseWheel' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "Mouse support on the file pane: wheel, click-select, double-click open"
```

---

## Task 8: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the keys table**

In `README.md`, add a `ctrl+t` row to the keys table:

```
| `ctrl+t` | Toggle typewriter scrolling (centered caret) |
```

- [ ] **Step 2: Document the mouse + replace the navigation note**

Replace the sidebar nav sentence:

```
Inside the sidebar, arrow keys + Enter navigate folders and open a file.
```

with:

```
Inside the sidebar, arrow keys + Enter navigate folders and open a file; the
mouse works too — wheel scrolls, a single click selects (and focuses the pane),
a double-click opens a file or enters a folder.
```

- [ ] **Step 3: Mark roadmap items 1 and 4 done**

Edit the roadmap list:

```
1. ~~**Typewriter scroll**~~ ✅ Done — caret pinned to center; `ctrl+t` toggles.
...
4. ~~**Chapter list**~~ ✅ Done — owned `filelist` replaces the filepicker
   (and adds mouse support).
```

- [ ] **Step 4: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```
Expected: no gofmt output; all tests pass; build ok.

- [ ] **Step 5: Commit and push**

```bash
git add README.md
git commit -m "Docs: typewriter + mouse file pane; mark roadmap #1, #4 done"
git push
```

- [ ] **Step 6: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` then verify: caret stays vertically centered while typing; `ctrl+t` toggles it off/on; mouse wheel scrolls the file list; single-click selects + focuses; double-click opens a file / enters a folder. Note: typewriter centering and mouse coordinate mapping are only fully confirmed here (tests cover the math, not the live render).

---

## Self-Review

**Spec coverage:**
- Typewriter vendored-textarea patch + symmetric padding → Tasks 1–2. (Bottom padding is provided by textarea's existing `m.height` end-of-buffer rows; only top padding is added — noted in Task 2 Step 5.)
- `ctrl+t` toggle, default on → Task 3.
- filelist read/filter/sort, nav, scroll, activate → Tasks 4–5.
- Replace filepicker, drop dependency, keyboard parity → Task 6.
- Mouse: wheel / click-select+focus / double-click open, coord mapping, 400ms double-click → Task 7.
- Out-of-scope items (editor mouse, focus dimming) respected. Docs/roadmap → Task 8.

**Placeholder scan:** none — every code step shows full code; every command shows expected output.

**Type consistency:** `filelist`, `fileEntry`, `newFilelist`, `SetDir`, `View`, `moveBy`, `scrollIntoView`, `selectRow`, `activate`, `sidebarRow`, `model.files`, `model.typewriter`, `textarea.Model.Typewriter`, `ViewportYOffset` are used consistently across tasks. `activate()` returns `(string, bool)` everywhere. Mouse uses Bubble Tea v1 API (`tea.MouseMsg.Action/Button`, `tea.MouseButtonWheelUp/Down/Left`, `tea.MouseActionPress`).

**Note on padding correctness:** with `Height/2` top padding and the pre-existing `Height` end-of-buffer rows below, `SetYOffset(cursorLineNumber())` centers every line including line 0 (top blanks) and EOF (bottom EOB rows). Verified by the Task 2 test at rows 0/10/20.
