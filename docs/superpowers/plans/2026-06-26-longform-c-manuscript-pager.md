# Long-form Plan C — Manuscript Pager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** a recurring generation bug emits a stray
> `court` token and/or drops the `antml:` namespace, silently no-op'ing tool calls.
> Mitigation: one tool call per message, as the FIRST element of the reply, explanation
> AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** A full-screen, read-through manuscript pager — all ordered sections concatenated into one scrollable view (raw prose, chapter dividers, dimmed markdown), a running "words-to-cursor / total" header, and Enter/click to jump into the editor at the exact line.

**Architecture:** A self-contained `pager.go` with a `pagerModel` that mirrors `outlineModel` — a manual scroller (not the bubbles viewport) owning the cursor line and an exact line→source map built ONCE on open. `main.go` adds `screenManuscript` and reaches it from the outline's `m`. One vendored-textarea addition (`MoveToLine`) lets jump-to-edit land on a specific line.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, `charmbracelet/x/ansi`.

**Design spec:** `docs/superpowers/specs/2026-06-26-manuscript-pager-plan-c-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt` (not on PATH).
- **Build once:** `pagerModel.load` reads each section file exactly once; scrolling and cursor movement do NO file I/O and never rebuild the lines. Render cost is O(visible height) — render only `lines[offset:offset+height]`. No glamour rendering.
- **Exact line→source map:** the build pre-wraps each source line to the measure width (`ansi.Wrap`), emitting one `pagerLine` per wrapped row, all mapping to the same `(file, src)`. The renderer never re-wraps; markdown dimming is colour-only (never changes a line's width).
- **Loose files excluded** from the pager (only `orderedSections`).
- Pager is reached from the outline via `m`; `o` → outline, `esc` → editor, `enter`/double-click → jump-to-edit. Only available inside a manuscript (entered from the outline).
- Reuse existing helpers; do not reimplement: `orderedSections`, `sectionTitle`, `projectTitle`, `wordCount`, `commafy`, `loadFile`, `enterOutline`, `selectedStyle`/`accent`/`subtle`, `sidebarRow`.
- Tests hermetic: `t.TempDir()` / `t.Setenv("OKASHI_DIR", …)`; the pure build takes the width in.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Editor line positioning (`internal/textarea`)

**Files:**
- Modify: `internal/textarea/textarea.go`
- Create: `internal/textarea/moveline_test.go`

**Interfaces:**
- Consumes: the package's `Model` (`m.value [][]rune`, `m.row`, `m.col`, `m.lastCharOffset`, the package-level `clamp`).
- Produces: `func (m *Model) MoveToLine(n int)` — moves the cursor to the start of line n (clamped to the buffer).

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/moveline_test.go`:

```go
package textarea

import "testing"

func TestMoveToLine(t *testing.T) {
	m := New()
	m.SetValue("a\nb\nc\nd\ne")
	m.MoveToLine(3)
	if m.Line() != 3 {
		t.Fatalf("MoveToLine(3) -> Line() = %d, want 3", m.Line())
	}
	m.MoveToLine(99)
	if m.Line() != 4 {
		t.Fatalf("MoveToLine(99) should clamp to the last line 4, got %d", m.Line())
	}
	m.MoveToLine(-1)
	if m.Line() != 0 {
		t.Fatalf("MoveToLine(-1) should clamp to 0, got %d", m.Line())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea -run TestMoveToLine 2>&1 | tail`
Expected: build error — `MoveToLine` undefined.

- [ ] **Step 3: Implement `MoveToLine`**

In `internal/textarea/textarea.go`, add this method next to `CursorStart` (which is right after `SetCursor`):

```go
// MoveToLine moves the cursor to the start of line n, clamped to the buffer.
func (m *Model) MoveToLine(n int) {
	if len(m.value) == 0 {
		return
	}
	m.row = clamp(n, 0, len(m.value)-1)
	m.col = 0
	m.lastCharOffset = 0
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test ./internal/textarea -run TestMoveToLine -v 2>&1 | tail
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/moveline_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea/textarea.go internal/textarea/moveline_test.go
git commit -m "textarea: MoveToLine — position the cursor on a given line"
```

---

## Task 2: Pager build — concatenate + line→source map (`pager.go`)

**Files:**
- Create: `pager.go`, `pager_test.go`

**Interfaces:**
- Consumes: `fileEntry`, `orderedSections`, `sectionTitle`, `wordCount` (existing); `ansi.Wrap`.
- Produces (package `main`):
  - `type pagerLine struct { text string; file string; src int; header bool; cumWords int }`
  - `type pagerModel struct { dir string; lines []pagerLine; total int; cursor int; offset int; width, height int }`
  - `func (p *pagerModel) load(dir string, width int)`

- [ ] **Step 1: Write the failing tests**

Create `pager_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagerLoadBuildsHeadersThenBody(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("four five"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose excluded"), 0o644) // loose
	var p pagerModel
	p.load(dir, 40)

	if len(p.lines) < 4 {
		t.Fatalf("expected at least 4 lines (2 headers + 2 body), got %d", len(p.lines))
	}
	if !p.lines[0].header || !strings.Contains(p.lines[0].text, "opening") {
		t.Fatalf("line 0 should be the 'opening' chapter header, got %+v", p.lines[0])
	}
	if p.lines[1].header || p.lines[1].file != "01-opening.md" || p.lines[1].src != 0 {
		t.Fatalf("line 1 should be opening's body line mapped to (01-opening.md,0), got %+v", p.lines[1])
	}
	// Loose file never appears.
	for _, l := range p.lines {
		if l.file == "notes.md" {
			t.Fatal("loose file must not appear in the pager")
		}
	}
	// Header lines carry their section file (so a header can jump to the section).
	if p.lines[0].file != "01-opening.md" || p.lines[0].src != -1 {
		t.Fatalf("header line should carry its file with src=-1, got %+v", p.lines[0])
	}
}

func TestPagerLoadWrapsLongLineKeepingMap(t *testing.T) {
	dir := t.TempDir()
	// One source line of 10 words; wrap to a small width so it spans several rows.
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("aa bb cc dd ee ff gg hh ii jj"), 0o644)
	var p pagerModel
	p.load(dir, 8)
	var bodyRows int
	for _, l := range p.lines {
		if l.header {
			continue
		}
		bodyRows++
		if l.file != "01-a.md" || l.src != 0 {
			t.Fatalf("every wrapped row of source line 0 must map to (01-a.md,0), got %+v", l)
		}
	}
	if bodyRows < 2 {
		t.Fatalf("a 10-word line at width 8 should wrap to >=2 rows, got %d", bodyRows)
	}
}

func TestPagerCumWordsAndTotal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644) // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)      // 2
	var p pagerModel
	p.load(dir, 40)
	if p.total != 5 {
		t.Fatalf("total = %d, want 5", p.total)
	}
	last := 0
	for _, l := range p.lines {
		if l.cumWords < last {
			t.Fatalf("cumWords must be monotonic non-decreasing, saw %d after %d", l.cumWords, last)
		}
		last = l.cumWords
	}
	if last != 5 {
		t.Fatalf("final cumWords = %d, want 5 (== total)", last)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestPagerLoad|TestPagerCumWords' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `pager.go`**

```go
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// pagerLine is one display row of the manuscript pager: text already wrapped to
// the measure width, plus the source it maps back to. Header (chapter-rule) lines
// carry their section file with src = -1.
type pagerLine struct {
	text     string
	file     string // section base name ("" only if there are no sections)
	src      int    // 0-based source line within file; -1 for a header line
	header   bool
	cumWords int // running word count from the manuscript start through this line
}

// pagerModel is the full-screen read-through pager: a manual scroller that owns
// the cursor line and the line→source map. Built once by load.
type pagerModel struct {
	dir    string
	lines  []pagerLine
	total  int
	cursor int
	offset int
	width  int
	height int
}

// load concatenates the dir's ordered sections (loose excluded) into wrapped,
// mapped lines. Each section contributes a "── Title ──" header line then its
// body, each source line wrapped to width. Reads each file exactly once.
func (p *pagerModel) load(dir string, width int) {
	if width < 1 {
		width = 1
	}
	entries := readEntries(dir) // markdown/text files, dirs excluded (from outline.go)
	sections, _ := orderedSections(entries)

	p.dir = dir
	p.lines = nil
	p.cursor = 0
	p.offset = 0

	running := 0
	for _, sec := range sections {
		p.lines = append(p.lines, pagerLine{
			text:     "── " + sectionTitle(sec.name) + " ──",
			file:     sec.name,
			src:      -1,
			header:   true,
			cumWords: running,
		})
		data, err := os.ReadFile(filepath.Join(dir, sec.name))
		if err != nil {
			continue
		}
		body := strings.TrimSuffix(string(data), "\n")
		for srcIdx, srcLine := range strings.Split(body, "\n") {
			for _, row := range strings.Split(ansi.Wrap(srcLine, width, ""), "\n") {
				running += wordCount(row)
				p.lines = append(p.lines, pagerLine{
					text:     row,
					file:     sec.name,
					src:      srcIdx,
					header:   false,
					cumWords: running,
				})
			}
		}
	}
	p.total = running
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPagerLoad|TestPagerCumWords' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w pager.go pager_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add pager.go pager_test.go
git commit -m "pager: build — concatenate ordered sections into a wrapped line→source map"
```

---

## Task 3: Pager render + navigation (`pager.go`)

**Files:**
- Modify: `pager.go`
- Test: `pager_test.go`

**Interfaces:**
- Consumes: `projectTitle`, `commafy`, `selectedStyle`, `accent`, `subtle`, `lipgloss`, `ansi` (existing); `pagerModel`/`pagerLine` (Task 2).
- Produces:
  - `const pagerHeaderHeight = 2`
  - `func (p *pagerModel) moveCursor(d int)`
  - `func (p *pagerModel) page(d int)`
  - `func (p pagerModel) jumpTarget() (file string, src int, ok bool)`
  - `func dimMarkdown(line string) string`
  - `func (p pagerModel) View() string`

- [ ] **Step 1: Write the failing tests**

Add to `pager_test.go` (`lipgloss` import needed):

```go
func TestPagerViewHeaderAndCursor(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 10
	view := p.View()
	if !strings.Contains(view, "/ 5w") {
		t.Fatalf("header should show the total '/ 5w':\n%s", view)
	}
	if !strings.Contains(view, "── a ──") {
		t.Fatalf("a chapter header rule should render:\n%s", view)
	}
}

func TestPagerMoveCursorClampsAndScrolls(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line\n")
	}
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte(sb.String()), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 5
	p.moveCursor(1000) // way past the end
	if p.cursor != len(p.lines)-1 {
		t.Fatalf("cursor should clamp to the last line, got %d/%d", p.cursor, len(p.lines))
	}
	if p.cursor < p.offset || p.cursor >= p.offset+p.height {
		t.Fatalf("cursor %d must be visible within [%d,%d)", p.cursor, p.offset, p.offset+p.height)
	}
	p.moveCursor(-1000)
	if p.cursor != 0 || p.offset != 0 {
		t.Fatalf("cursor/offset should return to 0, got cursor=%d offset=%d", p.cursor, p.offset)
	}
}

func TestPagerJumpTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("l0\nl1\nl2"), 0o644)
	var p pagerModel
	p.load(dir, 40)
	// lines: [0]=header(src -1), [1]=l0(src 0), [2]=l1(src 1), [3]=l2(src 2)
	p.cursor = 2
	file, src, ok := p.jumpTarget()
	if !ok || file != "01-a.md" || src != 1 {
		t.Fatalf("jumpTarget at a body line = (%q,%d,%v), want (01-a.md,1,true)", file, src, ok)
	}
	p.cursor = 0 // header line
	file, src, ok = p.jumpTarget()
	if !ok || file != "01-a.md" || src != 0 {
		t.Fatalf("jumpTarget at a header should open the section at line 0, got (%q,%d,%v)", file, src, ok)
	}
}

func TestDimMarkdownKeepsWidth(t *testing.T) {
	in := "# A *bold* idea"
	out := dimMarkdown(in)
	if lipgloss.Width(out) != lipgloss.Width(in) {
		t.Fatalf("dimMarkdown must not change the visible width: %d vs %d", lipgloss.Width(out), lipgloss.Width(in))
	}
}

func TestPagerScrollDoesNotReadFilesAfterBuild(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("some words here\n")
	}
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte(sb.String()), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 5
	// Remove the source file AFTER the build. If the pager re-read on scroll/render
	// it would now see no content; instead it must work entirely from p.lines.
	os.RemoveAll(filepath.Join(dir, "01-a.md"))
	before := len(p.lines)
	p.moveCursor(10)
	p.page(1)
	_ = p.View()
	if len(p.lines) != before {
		t.Fatal("scrolling/rendering must not rebuild lines or re-read files")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestPagerView|TestPagerMoveCursor|TestPagerJumpTarget|TestDimMarkdown|TestPagerScroll' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement (append to `pager.go`)**

Add `"fmt"` to `pager.go`'s imports (alongside `os`, `path/filepath`, `strings`, `ansi`), plus `"github.com/charmbracelet/lipgloss"`, then:

```go
const pagerHeaderHeight = 2 // the running-count header line + a blank spacer

// moveCursor moves the cursor by d lines (clamped) and scrolls to keep it visible.
func (p *pagerModel) moveCursor(d int) {
	if len(p.lines) == 0 {
		return
	}
	p.cursor += d
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.lines) {
		p.cursor = len(p.lines) - 1
	}
	p.ensureVisible()
}

// page moves the cursor by d full screens.
func (p *pagerModel) page(d int) { p.moveCursor(d * p.height) }

// ensureVisible scrolls offset so the cursor sits within the visible window.
func (p *pagerModel) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.height > 0 && p.cursor >= p.offset+p.height {
		p.offset = p.cursor - p.height + 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
}

// jumpTarget resolves the cursor line to the (file, source line) to open. A header
// line opens its section at line 0. ok is false when there's nothing to open.
func (p pagerModel) jumpTarget() (file string, src int, ok bool) {
	if p.cursor < 0 || p.cursor >= len(p.lines) {
		return "", 0, false
	}
	l := p.lines[p.cursor]
	if l.file == "" {
		return "", 0, false
	}
	if l.header {
		return l.file, 0, true
	}
	return l.file, l.src, true
}

// dimMarkdown colours markdown punctuation (#, *, _, `) subtle without changing the
// line's width, so the prose reads cleaner while the line→source map stays exact.
func dimMarkdown(line string) string {
	dim := lipgloss.NewStyle().Foreground(subtle)
	var b strings.Builder
	for _, r := range line {
		switch r {
		case '#', '*', '_', '`':
			b.WriteString(dim.Render(string(r)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// View renders the header line plus the visible window of lines (O(height)).
func (p pagerModel) View() string {
	cum := 0
	if p.cursor >= 0 && p.cursor < len(p.lines) {
		cum = p.lines[p.cursor].cumWords
	}
	head := fmt.Sprintf("%s · %s / %sw", projectTitle(filepath.Base(p.dir)), commafy(cum), commafy(p.total))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(head, p.width, "…")))
	b.WriteString("\n\n") // pagerHeaderHeight = 2

	end := p.offset + p.height
	if end > len(p.lines) {
		end = len(p.lines)
	}
	for i := p.offset; i < end; i++ {
		l := p.lines[i]
		var row string
		switch {
		case i == p.cursor:
			row = selectedStyle.Width(p.width).Render(ansi.Truncate(l.text, p.width, "…"))
		case l.header:
			row = lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(l.text, p.width, "…"))
		default:
			row = dimMarkdown(l.text)
		}
		b.WriteString(row)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPagerView|TestPagerMoveCursor|TestPagerJumpTarget|TestDimMarkdown|TestPagerScroll' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w pager.go pager_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add pager.go pager_test.go
git commit -m "pager: render (running header, dimmed markdown) + cursor/scroll navigation"
```

---

## Task 4: Screen wiring — enter, render, navigate, jump-to-edit (`main.go`)

**Files:**
- Modify: `main.go` (`screen` const, `model` struct, `Update`, `View`, the outline `m` case)
- Test: `pager_wiring_test.go` (create)

**Interfaces:**
- Consumes: `pagerModel` (`load`/`moveCursor`/`page`/`jumpTarget`/`View`), `pagerHeaderHeight` (Tasks 2–3); `loadFile`, `enterOutline`, `m.editor.MoveToLine` (Task 1), `m.colWidth`, `m.outline.dir`, `focusEditor`.
- Produces: `screenManuscript`; `model.pager pagerModel`; `func (m *model) enterManuscript()`; `func (m model) updateManuscript(msg tea.Msg) (tea.Model, tea.Cmd)`; `func (m model) pagerView() string`. The outline's `m` enters the pager.

- [ ] **Step 1: Write the failing tests**

Create `pager_wiring_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func manuscriptModel(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("alpha beta\ngamma"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("delta"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

func TestOutlineMEntersPager(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // pager
	m = nm.(model)
	if m.screen != screenManuscript {
		t.Fatalf("m in the outline should enter the pager, got screen %v", m.screen)
	}
	if len(m.pager.lines) == 0 {
		t.Fatal("the pager should be built on entry")
	}
}

func TestPagerEnterJumpsToEditAtLine(t *testing.T) {
	m, proj := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// Move the cursor to the "gamma" line (header, alpha, gamma -> index 2).
	m.pager.cursor = 2
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should jump into the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("Enter should open the mapped section, currentFile = %q", m.currentFile)
	}
	if m.editor.Line() != 1 {
		t.Fatalf("Enter should place the editor cursor on source line 1 (gamma), got %d", m.editor.Line())
	}
}

func TestPagerOGoesToOutlineEscToEditor(t *testing.T) {
	m, _ := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("o should return to the outline, got %v", m.screen)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}) // back to pager
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("esc should return to the editor, got %v", m.screen)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineMEntersPager|TestPagerEnter|TestPagerOGoes' 2>&1 | tail`
Expected: build error — `screenManuscript` undefined, etc.

- [ ] **Step 3: Add the screen, field, and entry helper**

In `main.go`, extend the `screen` const block (currently `screenHome`, `screenWriting`, `screenOutline`):

```go
const (
	screenHome screen = iota
	screenWriting
	screenOutline
	screenManuscript
)
```

Add to the `model` struct (after `outline outlineModel` and its companion fields):

```go
	pager pagerModel
```

Add the entry helper (place near `enterOutline`):

```go
// enterManuscript builds the read-through pager for the current outline's
// manuscript and shows it. Reached from the outline's `m`.
func (m *model) enterManuscript() {
	m.pager.width = m.colWidth
	m.pager.height = m.height - 1 - pagerHeaderHeight // status row + header
	if m.pager.height < 1 {
		m.pager.height = 1
	}
	m.pager.load(m.outline.dir, m.colWidth)
	m.screen = screenManuscript
	m.status = "manuscript · ↑↓ scroll · enter edit here · o outline · esc editor"
}
```

- [ ] **Step 4: Wire the outline `m`, screen dispatch, and `updateManuscript`/`pagerView`**

In `updateOutline`'s key switch, replace the stub:

```go
	case "m":
		m.status = "manuscript view — Plan C"
```

with:

```go
	case "m":
		m.enterManuscript()
```

In `Update`, add the dispatch right after the outline dispatch (`if m.screen == screenOutline { return m.updateOutline(msg) }`):

```go
	if m.screen == screenManuscript {
		return m.updateManuscript(msg)
	}
```

Add to `main.go`:

```go
// updateManuscript handles input on the pager: scroll, jump-to-edit, and exits.
func (m model) updateManuscript(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = sz.Width
		m.height = sz.Height
		m.pager.width = m.colWidth
		m.pager.height = sz.Height - 1 - pagerHeaderHeight
		if m.pager.height < 1 {
			m.pager.height = 1
		}
		m.pager.ensureVisible()
		m.layout()
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.pager.moveCursor(-1)
	case "down", "j":
		m.pager.moveCursor(1)
	case "pgup":
		m.pager.page(-1)
	case "pgdown":
		m.pager.page(1)
	case "enter":
		if file, src, ok := m.pager.jumpTarget(); ok {
			m.loadFile(filepath.Join(m.pager.dir, file))
			m.editor.MoveToLine(src)
			m.screen = screenWriting
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "o":
		m.enterOutline()
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
	}
	return m, nil
}

// pagerView renders the pager screen with the status bar.
func (m model) pagerView() string {
	body := m.pager.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}
```

In `View`, add the dispatch after the outline check (`if m.screen == screenOutline { return m.outlineView() }`):

```go
	if m.screen == screenManuscript {
		return m.pagerView()
	}
```

- [ ] **Step 5: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineMEntersPager|TestPagerEnter|TestPagerOGoes' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go pager_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go pager_wiring_test.go
git commit -m "pager: screen wiring — outline m enters, scroll, jump-to-edit, o/esc"
```

---

## Task 5: Mouse — click moves cursor, double-click jumps (`main.go`)

**Files:**
- Modify: `main.go` (`updateManuscript`)
- Test: `pager_wiring_test.go`

**Interfaces:**
- Consumes: `pagerHeaderHeight`, `m.pager` (`lines`/`offset`/`cursor`/`jumpTarget`), `sidebarRow`, `loadFile`, `m.editor.MoveToLine`, `m.lastClickRow`/`lastClickTime`.
- Produces: in the pager, a left-click sets the cursor from the hit-tested line; a second click on the same line within 400ms jumps to edit.

- [ ] **Step 1: Write the failing test**

Add to `pager_wiring_test.go` (add `"time"` is not needed; uses key/mouse msgs):

```go
func TestPagerClickThenDoubleClickJumps(t *testing.T) {
	m, proj := manuscriptModel(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = nm.(model)
	// Click the body line at offset 0 + (clickY - pagerHeaderHeight) = line 2 (gamma).
	clickY := pagerHeaderHeight + 2
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.pager.cursor != 2 {
		t.Fatalf("click should move the cursor to line 2, got %d", m.pager.cursor)
	}
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.screen != screenWriting || m.currentFile != filepath.Join(proj, "01-a.md") {
		t.Fatalf("double-click should jump into the editor, screen=%v file=%q", m.screen, m.currentFile)
	}
	if m.editor.Line() != 1 {
		t.Fatalf("double-click jump should land on source line 1 (gamma), got %d", m.editor.Line())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestPagerClick' 2>&1 | tail`
Expected: FAIL — mouse not handled in the pager.

- [ ] **Step 3: Handle mouse in `updateManuscript`**

In `updateManuscript`, add a mouse branch right before the `key, ok := msg.(tea.KeyMsg)` line:

```go
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Button != tea.MouseButtonLeft || mouse.Action != tea.MouseActionPress {
			return m, nil
		}
		row := sidebarRow(mouse.Y, pagerHeaderHeight, m.pager.height)
		if row < 0 {
			return m, nil
		}
		line := m.pager.offset + row
		if line >= len(m.pager.lines) {
			return m, nil
		}
		m.pager.cursor = line
		now := time.Now()
		if line == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if file, src, ok := m.pager.jumpTarget(); ok {
				m.loadFile(filepath.Join(m.pager.dir, file))
				m.editor.MoveToLine(src)
				m.screen = screenWriting
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{}
		} else {
			m.lastClickRow = line
			m.lastClickTime = now
		}
		return m, nil
	}
```

(Note: `sidebarRow(mouseY, bannerH, listHeight)` returns `mouseY - bannerH` when in range, else -1 — here `listHeight` is the visible body height `m.pager.height`, so clicks below the window return -1.)

- [ ] **Step 4: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPagerClick' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go pager_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go pager_wiring_test.go
git commit -m "pager: mouse — click moves the cursor, double-click jumps to edit"
```

---

## Task 6: Docs & status line

**Files:**
- Modify: `main.go` (outline status hint), `README.md`

**Interfaces:** none (docs + a status-hint string).

- [ ] **Step 1: Add the `m` (manuscript) hint to the outline status**

In `enterOutline`, the status line currently reads
`"outline · ↑↓ select · J/K reorder · enter open · n new · esc back"`.
Add the manuscript hint:

```go
	m.status = "outline · ↑↓ select · J/K reorder · enter open · n new · m read · esc back"
```

- [ ] **Step 2: Document the pager in `README.md`**

Read `README.md` first to match its heading style. Add to the outline/keys area:

```markdown
### Manuscript pager (read-through)

From the outline, press **m** for a full-screen read-through of the whole manuscript —
every chapter concatenated, with `── Title ──` dividers and a `words-so-far / total`
header so you can see where you are.

- `↑ ↓` / `j k` scroll · `pgup` / `pgdn` page · the cursor line is highlighted.
- **enter** (or double-click) on any line jumps into the editor at that exact line.
- `o` back to the outline · `esc` to the editor.
```

- [ ] **Step 3: Verify build + full suite; commit**

```bash
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go README.md
git commit -m "docs: manuscript pager keymap + outline m hint"
```

---

## Self-Review

**Spec coverage:**
- §1 Build once (concatenate ordered sections, header + wrapped body, line→source map, cumWords, loose excluded, each file read once) → Task 2.
- §2 Render O(height) (running header, window-only body, cursor highlight, chapter rule, dimmed markdown, `pagerHeaderHeight`) → Task 3.
- §3 Navigation & jump-to-edit (cursor move/page, jumpTarget incl. header→line 0, Enter/double-click → loadFile + MoveToLine, o/esc) → Task 3 (helpers), Task 4 (keys), Task 5 (mouse).
- §4 Editor line positioning (`MoveToLine`) → Task 1.
- §5 Wiring & keys (`screenManuscript`, outline `m` → pager, `o`/`esc`) → Task 4, Task 6 (hints).
- §6 Performance (build once, no disk after build, O(height) render) → Task 2 (file-read-once via the build) and Task 3's `TestPagerScrollDoesNotReadFilesAfterBuild` (removes the source file post-build and asserts scroll/render still work).

**Placeholder scan:** none — full code in every step.

**Type consistency:** `pagerLine{text,file,src,header,cumWords}` and `pagerModel{dir,lines,total,cursor,offset,width,height}` defined in Task 2, consumed by Tasks 3–5; `load`/`moveCursor`/`page`/`ensureVisible`/`jumpTarget`/`View`/`dimMarkdown`/`pagerHeaderHeight` consistent across Tasks 3–5; `model.pager`, `enterManuscript`, `updateManuscript`, `pagerView`, `screenManuscript` consistent across Tasks 4–6; `m.editor.MoveToLine` (Task 1) used by Tasks 4–5; `readEntries` reused from `outline.go` (markdown/text files, dirs excluded) so the pager and outline see the same sections.

**Deviation from the spec (noted):** spec §1 sketched header lines with `file:""`; this plan stores the **section file** on header lines (with `src:-1`) so `jumpTarget` can open a chapter from its header (spec §3 requires exactly that). `pagerLine.file` is only `""` in the degenerate no-sections case.

**Cross-cutting checks baked into tests:** every wrapped row of a source line maps to the same `(file,src)` (Task 2); `cumWords` monotonic and `== total` (Task 2); cursor clamps and stays visible (Task 3); the build is not re-read on scroll/render (Task 3); Enter and double-click both `loadFile` + land the editor on the mapped line (Tasks 4–5); `o`/`esc` route correctly (Task 4); the hit-test uses the same `pagerHeaderHeight` the render offsets by (Task 5).

**Note for the executor:** Task 4 adds the `updateManuscript` `WindowSizeMsg` branch from the start (unlike the earlier outline task, which needed a follow-up fix) — keep it. The `o` exit calls `enterOutline`, which rebuilds the outline from `m.files.dir`; this is correct because entering the pager never changed `m.files.dir` (it stays the manuscript folder).
