# Inspector Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Add a toggleable right-side **inspector** panel (three-column layout) with a tab framework and its first tab, **Words** (live doc + project reading stats).

**Architecture:** A new `inspector.go` holds the read-only `inspectorModel` (tab bar + Words body) plus pure stat helpers (`computeDocStats`, `computeProjStats`). `main.go` adds the panel to the writing-screen `View()`/`layout()` via a shared `effectivePanels()` helper (so sizing and composition never diverge), a `ctrl+i` toggle, and the auto-hide-sidebar-when-narrow rule.

**Tech Stack:** Go, lipgloss, existing `wordCount`/`commafy`/`wordCountCache`/`resolveManuscript`.

**Design spec:** `docs/superpowers/specs/2026-06-28-inspector-foundation-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt`.
- `inspectorWidth = 32`; `minEditorMeasure = 50`. Inspector shows on the **writing screen only**, toggled by `ctrl+i`.
- Effective-panel rule (computed, never mutates `m.sidebarVisible`): if inspector open AND `width - sidebarWidth - inspectorWidth < minEditorMeasure`, suppress the sidebar for that render.
- Inspector is read-only / non-focusable this cycle; the tab-switch key is deferred (only one tab).
- Reuse existing: `wordCount`, `commafy` (main.go), `wordCountCache.count`, `m.files.view` (resolved `manuscriptView` with `chapters []chapterRef{file,title}` + `loose []fileEntry`), `m.files.wc`, `m.files.dir`.
- Base palette unchanged; `inspectorStyle` mirrors `sidebarStyle` (border on the LEFT).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Inspector component + stat helpers (`inspector.go`, `styles.go`)

**Files:**
- Create: `inspector.go`, `inspector_test.go`
- Modify: `styles.go` (add `inspectorStyle`)

**Interfaces:**
- Produces: `type inspectorTab int`, `const tabWords inspectorTab = 0`; `type inspectorModel struct { visible bool; tab inspectorTab }`; `type docStats struct{ words, chars, paragraphs int }`; `type projStats struct{ words, chapters int; manuscript bool }`; `func computeDocStats(text string) docStats`; `func computeProjStats(dir string, v manuscriptView, wc *wordCountCache) projStats`; `func (in inspectorModel) View(width int, doc docStats, proj projStats) string`.

- [ ] **Step 1: Write the failing tests**

Create `inspector_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeDocStats(t *testing.T) {
	ds := computeDocStats("Hello world.\n\nSecond paragraph here.\n")
	if ds.words != 5 {
		t.Fatalf("words = %d, want 5", ds.words)
	}
	if ds.paragraphs != 2 {
		t.Fatalf("paragraphs = %d, want 2", ds.paragraphs)
	}
	if ds.chars == 0 {
		t.Fatal("chars should be non-zero")
	}
	// Trailing blank lines must not inflate the paragraph count.
	if got := computeDocStats("One.\n\n\n\nTwo.\n\n").paragraphs; got != 2 {
		t.Fatalf("paragraphs with extra blank lines = %d, want 2", got)
	}
	// Empty buffer → all zero.
	if z := computeDocStats(""); z.words != 0 || z.chars != 0 || z.paragraphs != 0 {
		t.Fatalf("empty stats = %+v, want zero", z)
	}
}

func TestComputeProjStatsManuscript(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644)   // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)        // 2
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("loose note words"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if !ps.manuscript {
		t.Fatal("expected manuscript = true (numbered chapters present)")
	}
	if ps.chapters != 2 {
		t.Fatalf("chapters = %d, want 2", ps.chapters)
	}
	if ps.words != 5 {
		t.Fatalf("project words = %d, want 5 (chapters only, loose excluded)", ps.words)
	}
}

func TestComputeProjStatsPlainFolder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("three"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	ps := computeProjStats(dir, v, newWordCountCache())
	if ps.manuscript {
		t.Fatal("plain folder must not be a manuscript")
	}
	if ps.words != 3 {
		t.Fatalf("folder words = %d, want 3 (sum of loose docs)", ps.words)
	}
}

func TestInspectorViewRendersWords(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{words: 1204, chars: 6830, paragraphs: 38}, projStats{words: 47032, chapters: 12, manuscript: true})
	for _, want := range []string{"Words", "Document", "Project", "1,204", "47,032", "Chapters", "12"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspector view missing %q:\n%s", want, out)
		}
	}
	// Non-manuscript omits the Chapters line.
	plain := in.View(28, docStats{words: 10}, projStats{words: 10, manuscript: false})
	if strings.Contains(plain, "Chapters") {
		t.Fatal("non-manuscript inspector should omit 'Chapters'")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestComputeDocStats|TestComputeProjStats|TestInspectorView' 2>&1 | tail`
Expected: build errors — undefined `computeDocStats`/`computeProjStats`/`inspectorModel`.

- [ ] **Step 3: Create `inspector.go`**

```go
package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

type inspectorTab int

const tabWords inspectorTab = 0 // more tabs (Goals, Analysis, Outline) added later

// inspectorModel is the read-only right-side panel: a tab bar + the active tab.
type inspectorModel struct {
	visible bool
	tab     inspectorTab
}

type docStats struct{ words, chars, paragraphs int }

type projStats struct {
	words, chapters int
	manuscript      bool
}

var blankLineRe = regexp.MustCompile(`\n[ \t]*\n`)

// computeDocStats derives word/char/paragraph counts from the open buffer.
func computeDocStats(text string) docStats {
	if strings.TrimSpace(text) == "" {
		return docStats{}
	}
	ds := docStats{
		words: wordCount(text),
		chars: utf8.RuneCountInString(text),
	}
	for _, block := range blankLineRe.Split(text, -1) {
		if strings.TrimSpace(block) != "" {
			ds.paragraphs++
		}
	}
	return ds
}

// computeProjStats sums the resolved manuscript's chapter word counts (or, for a
// plain folder, its loose docs) using the existing word-count cache.
func computeProjStats(dir string, v manuscriptView, wc *wordCountCache) projStats {
	if wc == nil {
		return projStats{}
	}
	if len(v.chapters) > 0 {
		ps := projStats{manuscript: true, chapters: len(v.chapters)}
		for _, ch := range v.chapters {
			ps.words += wc.count(filepath.Join(dir, ch.file))
		}
		return ps
	}
	ps := projStats{}
	for _, e := range v.loose {
		ps.words += wc.count(filepath.Join(dir, e.name))
	}
	return ps
}

// kvRow renders "  label" left, a subtle right-aligned number, fit to width.
func kvRow(label string, n, width int) string {
	lbl := "  " + label
	val := commafy(n)
	gap := width - lipgloss.Width(lbl) - lipgloss.Width(val)
	if gap < 1 {
		gap = 1
	}
	return lbl + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(subtle).Render(val)
}

// View renders the tab bar + the active tab's body, fit to the given inner width.
func (in inspectorModel) View(width int, doc docStats, proj projStats) string {
	tabs := []string{"Words"} // future: Goals, Analysis, Outline
	var bar strings.Builder
	for i, t := range tabs {
		chip := " " + t + " "
		if inspectorTab(i) == in.tab {
			bar.WriteString(selectedStyle.Render(chip))
		} else {
			bar.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(chip))
		}
	}

	var b strings.Builder
	b.WriteString(bar.String())
	b.WriteString("\n\n")
	b.WriteString(breadcrumbStyle.Render("Document") + "\n")
	b.WriteString(kvRow("Words", doc.words, width) + "\n")
	b.WriteString(kvRow("Characters", doc.chars, width) + "\n")
	b.WriteString(kvRow("Paragraphs", doc.paragraphs, width) + "\n\n")
	b.WriteString(breadcrumbStyle.Render("Project") + "\n")
	b.WriteString(kvRow("Words", proj.words, width))
	if proj.manuscript {
		b.WriteString("\n" + kvRow("Chapters", proj.chapters, width))
	}
	return b.String()
}
```

- [ ] **Step 4: Add `inspectorStyle` to `styles.go`**

```go
// inspectorStyle is the right-side info panel — mirrors sidebarStyle with the
// border on the LEFT edge.
var inspectorStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder(), false, false, false, true).
	BorderForeground(subtle).
	Padding(0, 1)
```

- [ ] **Step 5: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestComputeDocStats|TestComputeProjStats|TestInspectorView' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w inspector.go inspector_test.go styles.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add inspector.go inspector_test.go styles.go
git commit -m "inspector: component shell (tab bar + Words tab) + doc/project stat helpers"
```

---

## Task 2: Wire the inspector into the writing screen (`main.go`)

**Files:**
- Modify: `main.go` (model field + init, `effectivePanels`, `layout`, `View`, `ctrl+i`, status hint)
- Test: `main_test.go` or `smoke_test.go`

**Interfaces:**
- Consumes: `inspectorModel`, `computeDocStats`, `computeProjStats`, `inspectorStyle`, `inspectorWidth`, `minEditorMeasure` (Task 1).
- Produces: `func (m model) effectivePanels() (showSidebar, showInspector bool, editorArea int)`.

- [ ] **Step 1: Write the failing tests**

Add to `smoke_test.go`:

```go
func TestEffectivePanels(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	m.sidebarVisible = true
	m.inspector.visible = true

	// Wide: all three columns; editor area is the remainder.
	m.width = 140
	ss, si, area := m.effectivePanels()
	if !ss || !si {
		t.Fatalf("wide: showSidebar=%v showInspector=%v, want both true", ss, si)
	}
	if area != 140-sidebarWidth-inspectorWidth {
		t.Fatalf("wide editorArea = %d, want %d", area, 140-sidebarWidth-inspectorWidth)
	}

	// Narrow: inspector open squeezes the editor → sidebar suppressed, editor keeps >= min.
	m.width = 90 // 90-32-32 = 26 < 50
	ss, si, area = m.effectivePanels()
	if ss {
		t.Fatal("narrow: sidebar should be suppressed while the inspector is open")
	}
	if !si {
		t.Fatal("narrow: inspector should stay visible")
	}
	if area < minEditorMeasure {
		t.Fatalf("narrow editorArea = %d, want >= %d", area, minEditorMeasure)
	}
}

func TestInspectorToggleAndRender(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = nm.(model)
	m.editor.SetValue("Some prose here.\n")

	if strings.Contains(m.View(), "WORDS") {
		t.Fatal("inspector should be hidden by default")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // placeholder; replace with ctrl+i below
	_ = nm
	// Toggle directly (key wiring covered by the case; assert View reflects state):
	m.inspector.visible = true
	m.layout()
	if !strings.Contains(m.View(), "WORDS") {
		t.Fatal("writing View should contain the inspector when visible")
	}
}
```

**Note for the implementer:** the second test's `KeyCtrlAt` line is a placeholder — drive the real toggle through the `ctrl+i` case you add (e.g. `m.Update(tea.KeyMsg{Type: tea.KeyCtrlI})` if that's how other ctrl keys are matched in this file, or the string-keyed equivalent the writing-update uses) and assert `m.inspector.visible` flipped. Keep the View-contains-WORDS assertion. Match how existing `ctrl+*` keys are tested/handled in `main.go` (they use string cases like `case "ctrl+p":`).

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestEffectivePanels|TestInspectorToggle' 2>&1 | tail`
Expected: build errors — `effectivePanels`, `m.inspector`, `inspectorWidth` undefined.

- [ ] **Step 3: Add the model field, constants, and `effectivePanels`**

In `main.go`: add `inspector inspectorModel` to the `model` struct; initialize it in `initialModel` (zero value is fine — `visible:false`). Add the constants near `sidebarWidth`:

```go
const inspectorWidth = 32
const minEditorMeasure = 50
```

Add the helper:

```go
// effectivePanels resolves which side panels are shown this render and the
// width left for the editor. When the inspector is open and showing both panels
// would squeeze the editor below minEditorMeasure, the sidebar is suppressed for
// this render (m.sidebarVisible is not mutated).
func (m model) effectivePanels() (showSidebar, showInspector bool, editorArea int) {
	showSidebar = m.sidebarVisible
	showInspector = m.inspector.visible
	if showInspector && showSidebar && m.width-sidebarWidth-inspectorWidth < minEditorMeasure {
		showSidebar = false
	}
	editorArea = m.width
	if showSidebar {
		editorArea -= sidebarWidth
	}
	if showInspector {
		editorArea -= inspectorWidth
	}
	return
}
```

- [ ] **Step 4: Use `effectivePanels` in `layout()` and compose the column in `View()`**

In `layout()`, replace the sidebar/width block so the editor width derives from `effectivePanels`:

```go
	showSidebar, _, editorArea := m.effectivePanels()
	cw := min(m.colWidth, editorArea-2)
	if showSidebar {
		m.files.height = bodyH - 3
		m.files.width = sidebarWidth - 3
	}
	m.editor.SetWidth(cw)
	m.editor.SetHeight(bodyH)
	m.preview.Width = cw
	m.preview.Height = bodyH - 1
```

In `View()` (writing screen), replace the `if m.sidebarVisible { … } else { … }` body block with a three-column compose driven by `effectivePanels`:

```go
	showSidebar, showInspector, editorArea := m.effectivePanels()

	cols := []string{}
	if showSidebar {
		sideInner := lipgloss.JoinVertical(
			lipgloss.Left,
			func() string { row, _ := m.files.breadcrumbBar(sidebarWidth - 3); return breadcrumbStyle.Render(row) }(),
			m.files.View(),
		)
		cols = append(cols, sidebarStyle.Width(sidebarWidth-1).Height(bodyH-2).Render(sideInner))
	}
	cols = append(cols, lipgloss.Place(editorArea, bodyH, lipgloss.Center, lipgloss.Top, pane))
	if showInspector {
		doc := computeDocStats(m.editor.Value())
		proj := computeProjStats(m.files.dir, m.files.view, m.files.wc)
		insInner := m.inspector.View(inspectorWidth-3, doc, proj)
		cols = append(cols, inspectorStyle.Width(inspectorWidth-1).Height(bodyH-2).Render(insInner))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
```

(Delete the old `var body string` / `if m.sidebarVisible` block this replaces; keep the trailing `status := …` / `JoinVertical(body, status)`.)

- [ ] **Step 5: Add the `ctrl+i` toggle + status hint**

In the writing-screen key switch (where `case "ctrl+b":` etc. live), add:

```go
		case "ctrl+i":
			m.inspector.visible = !m.inspector.visible
			m.layout()
```

Add ` · ctrl+i inspector` to the writing-screen status hint string (the one near `initialModel` / `statusBar`).

- [ ] **Step 6: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestEffectivePanels|TestInspectorToggle' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "inspector: wire into the writing screen (ctrl+i toggle, 3-column layout, auto-hide sidebar)"
```

---

## Self-Review

**Spec coverage:** component shell + tab bar + Words body + `computeDocStats`/`computeProjStats` → Task 1; three-column layout via `effectivePanels` (shared by `layout`+`View`), `ctrl+i` toggle (writing only), auto-hide rule, status hint → Task 2. Inspector read-only/non-focusable (no focus changes) → both tasks. Tab-switch key deferred (one tab) → noted.

**Placeholder scan:** Task 1 fully concrete. Task 2's test has ONE flagged placeholder (`KeyCtrlAt`) with an explicit instruction to drive the real `ctrl+i` case the way `main.go` matches ctrl keys (string cases) — the implementer wires it to the actual key; the assertions are real.

**Type consistency:** `inspectorModel{visible, tab}`, `docStats{words,chars,paragraphs}`, `projStats{words,chapters,manuscript}`, `computeDocStats(string) docStats`, `computeProjStats(string, manuscriptView, *wordCountCache) projStats`, `inspectorModel.View(int, docStats, projStats) string`, `effectivePanels() (bool,bool,int)`, consts `inspectorWidth`/`minEditorMeasure` — defined in Task 1/early Task 2 and used consistently. Reuses `m.files.view` (`manuscriptView`), `m.files.wc`, `wordCount`, `commafy`, `selectedStyle`, `breadcrumbStyle`, `subtle`, `sidebarStyle`.

**Integration risk (for the executor):** `View()`'s sidebar/editor block is being restructured into the `cols` compose — preserve the exact `sideInner`/`breadcrumbBar`/`sidebarStyle` rendering (only how it's assembled changes) and the trailing status row. Confirm the existing full-screen (no sidebar) behavior still holds: with both panels off, `cols` is just the centered editor — identical to today.
