# File-Pane Clean Look + Clickable Breadcrumb — Implementation Plan (Plan 2 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the file-pane width-wrapping bug, give the pane a yazi-style clean look (gutter, full-width selection bar, dim extensions, scroll indicator), and make the breadcrumb a clickable folder navigator with head-truncation.

**Architecture:** All in `filelist.go` (rendering + breadcrumb helpers) and `main.go` (sidebar render width + breadcrumb mouse). A `breadcrumbBar(width)` helper owns truncation + indicator + clickable column ranges so render and hit-test share one computation.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, `charmbracelet/x/ansi`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- `sidebarWidth = 32`. The sidebar frame is `sidebarStyle` (right border + `Padding(0,1)`). Measured: `sidebarStyle.Width(n)` renders `n+1` total columns and gives `n-2` content columns. So: frame `Width(sidebarWidth-1)` → total `sidebarWidth` (32); file-pane content budget = `sidebarWidth-3` (29). `filelist.width` and the breadcrumb width MUST be `sidebarWidth-3`.
- Clean look (all four): one-column left **gutter**, full-width **selection bar** (accent), **dim** file extensions (`subtle`), right-aligned **scroll indicator** `sel+1/total` (only when `len(entries) > height`, lives in the breadcrumb row).
- **Clickable breadcrumb:** click a segment → `SetDir` to that ancestor; **head-truncation** (`okashi / … / Drafts`) keeps the segments nearest you visible+clickable; the `…` is not clickable. Breadcrumb is at sidebar screen row 0; the sidebar's left padding is 1 column (click X → breadcrumb col = X − 1).
- Tests touching `writingDir()` set `t.Setenv("OKASHI_DIR", t.TempDir())`.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Fix the sidebar width accounting (wrapping bug)

**Files:**
- Modify: `main.go` (`View` sidebar `Width` + breadcrumb truncate; `layout` `m.files.width`)
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `m.files.width == sidebarWidth-3`; sidebar frame `Width(sidebarWidth-1)`; breadcrumb truncated to `sidebarWidth-3`.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestSidebarFrameFitsAndAligns(t *testing.T) {
	line := strings.Repeat("x", sidebarWidth-3) // the file-pane content budget
	out := sidebarStyle.Width(sidebarWidth - 1).Render(line)
	if lipgloss.Height(out) != 1 {
		t.Fatalf("a %d-char line wrapped in the sidebar frame (height %d)", sidebarWidth-3, lipgloss.Height(out))
	}
	if w := lipgloss.Width(out); w != sidebarWidth {
		t.Fatalf("sidebar frame total width = %d, want %d", w, sidebarWidth)
	}
}

func TestLayoutFilePaneWidth(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	if m.files.width != sidebarWidth-3 {
		t.Fatalf("files.width = %d, want %d", m.files.width, sidebarWidth-3)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSidebarFrameFits|TestLayoutFilePaneWidth' -v 2>&1 | tail`
Expected: `TestLayoutFilePaneWidth` FAILs (`files.width = 30, want 29`); the frame test may already pass (it asserts the target values).

- [ ] **Step 3: Apply the width fix**

In `main.go` `layout`, change:

```go
		m.files.width = sidebarWidth - 2
```

to:

```go
		m.files.width = sidebarWidth - 3
```

In `main.go` `View`, the sidebar block: change the breadcrumb truncate width and the frame `Width`:

```go
			breadcrumbStyle.Render(ansi.Truncate(m.files.breadcrumb(), sidebarWidth-2, "…")),
```
→
```go
			breadcrumbStyle.Render(ansi.Truncate(m.files.breadcrumb(), sidebarWidth-3, "…")),
```

and

```go
		side := sidebarStyle.
			Width(sidebarWidth - 2).
```
→
```go
		side := sidebarStyle.
			Width(sidebarWidth - 1).
```

- [ ] **Step 4: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebarFrameFits|TestLayoutFilePaneWidth' -v 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Fix file-pane width accounting (selected rows no longer wrap)"
```

---

## Task 2: Clean look — gutter, full-width bar, dim extensions

**Files:**
- Modify: `filelist.go` (`View`)
- Test: `filelist_test.go`

**Interfaces:**
- Produces: each row prefixed with a one-space gutter; selected row is a full-width `accent` bar; non-selected file rows render the extension in `subtle`.

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go`:

```go
func TestFilelistGutterAndDimExtension(t *testing.T) {
	f := newFilelist()
	f.width = 29
	f.height = 5
	f.selected = -1 // nothing selected → file uses the dim-extension path
	f.entries = []fileEntry{{name: "chapter.md"}}

	view := f.View()
	if !strings.HasPrefix(view, " ") {
		t.Fatal("rows should start with a one-column gutter")
	}
	wantExt := lipgloss.NewStyle().Foreground(subtle).Render(".md")
	if !strings.Contains(view, wantExt) {
		t.Fatal("a file extension should be dimmed with the subtle style")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestFilelistGutterAndDimExtension -v 2>&1 | tail`
Expected: FAIL — no gutter / extension not dimmed.

- [ ] **Step 3: Rewrite the row loop in `filelist.View`**

Replace the per-entry loop body in `filelist.View()`:

```go
		e := f.entries[i]
		label := ansi.Truncate(f.icons.icon(e)+e.name, f.width, "…")
		switch {
		case i == f.selected:
			b.WriteString(selectedStyle.Width(f.width).Render(label))
		case e.isDir:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(label))
		default:
			b.WriteString(label)
		}
```

with:

```go
		e := f.entries[i]
		head := " " + f.icons.icon(e) // one-column gutter, then the icon
		full := head + e.name
		switch {
		case i == f.selected:
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(full, f.width, "…")))
		case e.isDir:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(full, f.width, "…")))
		default:
			// Non-selected file: dim the extension when the whole row fits.
			ext := filepath.Ext(e.name)
			if ext != "" && lipgloss.Width(full) <= f.width {
				stem := head + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(full, f.width, "…"))
			}
		}
```

(Ensure `path/filepath` is imported in `filelist.go` — it already is.)

- [ ] **Step 4: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFilelist' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w filelist.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go filelist_test.go
git commit -m "File pane clean look: gutter, full-width bar, dim extensions"
```

---

## Task 3: Breadcrumb segments, head-truncation, scroll indicator

**Files:**
- Modify: `filelist.go` (`breadcrumbSegments`, `breadcrumbBar`), `main.go` (`View` renders `breadcrumbBar`)
- Test: `filelist_test.go`

**Interfaces:**
- Produces:
  - `type breadcrumbSeg struct { label, path string }`
  - `type segHit struct { start, end int; path string }`
  - `func (f filelist) breadcrumbSegments() []breadcrumbSeg`
  - `func (f filelist) breadcrumbBar(width int) (row string, hits []segHit)` — head-truncated breadcrumb + right-aligned `sel+1/total` indicator; `hits` are the clickable column ranges of visible segments (the `…` is excluded).

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go`:

```go
func TestBreadcrumbSegments(t *testing.T) {
	f := filelist{root: "/home/me/okashi", dir: "/home/me/okashi/Book/Drafts"}
	segs := f.breadcrumbSegments()
	want := []breadcrumbSeg{
		{"okashi", "/home/me/okashi"},
		{"Book", "/home/me/okashi/Book"},
		{"Drafts", "/home/me/okashi/Book/Drafts"},
	}
	if len(segs) != len(want) {
		t.Fatalf("got %d segments, want %d: %+v", len(segs), len(want), segs)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, segs[i], want[i])
		}
	}
}

func TestBreadcrumbBarFitsWithHits(t *testing.T) {
	f := filelist{root: "/r/okashi", dir: "/r/okashi/Book", height: 5}
	f.entries = []fileEntry{{name: "a"}, {name: "b"}} // 2 < height → no indicator
	row, hits := f.breadcrumbBar(40)
	if !strings.Contains(row, "okashi / Book") {
		t.Fatalf("row = %q", row)
	}
	if len(hits) != 2 {
		t.Fatalf("want 2 clickable segments, got %d", len(hits))
	}
	// The "Book" hit's column range should contain its rune.
	if hits[1].path != "/r/okashi/Book" || hits[1].start >= hits[1].end {
		t.Fatalf("bad hit %+v", hits[1])
	}
}

func TestBreadcrumbBarIndicator(t *testing.T) {
	f := filelist{root: "/r/okashi", dir: "/r/okashi", height: 2}
	f.selected = 2
	f.entries = make([]fileEntry, 10) // 10 > height 2 → indicator
	row, _ := f.breadcrumbBar(40)
	if !strings.Contains(row, "3/10") {
		t.Fatalf("expected scroll indicator 3/10, row = %q", row)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestBreadcrumbSegments|TestBreadcrumbBar' -v 2>&1 | tail`
Expected: build error — `breadcrumbSegments`/`breadcrumbBar` undefined.

- [ ] **Step 3: Implement the helpers**

Add to `filelist.go` (add `"fmt"` to imports):

```go
type breadcrumbSeg struct {
	label string
	path  string
}

// segHit is a clickable column range [start,end) in the breadcrumb row.
type segHit struct {
	start, end int
	path       string
}

// breadcrumbSegments returns the segments from the workspace root (base name
// first) down to the current dir, each with its target path.
func (f filelist) breadcrumbSegments() []breadcrumbSeg {
	segs := []breadcrumbSeg{{label: filepath.Base(f.root), path: f.root}}
	rel, err := filepath.Rel(f.root, f.dir)
	if err == nil && rel != "." && rel != "" {
		cur := f.root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			segs = append(segs, breadcrumbSeg{label: part, path: cur})
		}
	}
	return segs
}

// breadcrumbBar renders the breadcrumb head-truncated to width, with a
// right-aligned "sel/total" indicator when the list overflows, and returns the
// clickable column ranges of the visible segments (the "…" is not clickable).
func (f filelist) breadcrumbBar(width int) (string, []segHit) {
	segs := f.breadcrumbSegments()

	ind := ""
	if f.height > 0 && len(f.entries) > f.height {
		ind = fmt.Sprintf("%d/%d", f.selected+1, len(f.entries))
	}
	avail := width
	if ind != "" {
		avail -= lipgloss.Width(ind) + 1
	}
	if avail < 1 {
		avail = 1
	}

	const sep = " / "
	labels := make([]string, len(segs))
	for i, s := range segs {
		labels[i] = s.label
	}

	var visible []breadcrumbSeg
	if lipgloss.Width(strings.Join(labels, sep)) <= avail {
		visible = segs
	} else {
		root := segs[0]
		used := lipgloss.Width(root.label) + lipgloss.Width(sep) + lipgloss.Width("…")
		var tail []breadcrumbSeg
		for i := len(segs) - 1; i >= 1; i-- {
			w := lipgloss.Width(sep) + lipgloss.Width(segs[i].label)
			if used+w > avail {
				break
			}
			used += w
			tail = append([]breadcrumbSeg{segs[i]}, tail...)
		}
		visible = append([]breadcrumbSeg{root, {label: "…", path: ""}}, tail...)
	}

	var b strings.Builder
	var hits []segHit
	col := 0
	for i, s := range visible {
		if i > 0 {
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		start := col
		b.WriteString(s.label)
		col += lipgloss.Width(s.label)
		if s.path != "" {
			hits = append(hits, segHit{start: start, end: col, path: s.path})
		}
	}
	left := b.String()

	if ind == "" {
		return left, hits
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(ind)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + ind, hits
}
```

- [ ] **Step 4: Render `breadcrumbBar` in `main.go` View**

In `main.go` `View`, the sidebar block currently renders the breadcrumb via `m.files.breadcrumb()`. Replace:

```go
			breadcrumbStyle.Render(ansi.Truncate(m.files.breadcrumb(), sidebarWidth-3, "…")),
```

with:

```go
			func() string { row, _ := m.files.breadcrumbBar(sidebarWidth - 3); return breadcrumbStyle.Render(row) }(),
```

- [ ] **Step 5: Run tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestBreadcrumb' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w filelist.go main.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go main.go filelist_test.go
git commit -m "Breadcrumb: head-truncation, scroll indicator, clickable ranges"
```

---

## Task 4: Clickable breadcrumb (mouse → segment → SetDir)

**Files:**
- Modify: `main.go` (`tea.MouseMsg` click branch)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `breadcrumbBar` / `segHit` (Task 3).
- Produces: a left-click in the breadcrumb row (sidebar screen row 0) maps X→segment and `SetDir`s to it.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestBreadcrumbClickNavigates(t *testing.T) {
	t.Setenv("OKASHI_DIR", t.TempDir())
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	root := m.files.root
	if err := os.MkdirAll(filepath.Join(root, "Book", "Drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.files.SetDir(filepath.Join(root, "Book", "Drafts"))

	// Find the "okashi" (root) segment's column and click it (breadcrumb is at
	// screen row 0; sidebar left padding is 1 col → screen X = col + 1).
	_, hits := m.files.breadcrumbBar(sidebarWidth - 3)
	if len(hits) == 0 {
		t.Fatal("expected clickable segments")
	}
	rootHit := hits[0] // the workspace root
	clickX := rootHit.start + 1
	nm, _ = m.Update(tea.MouseMsg{X: clickX, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.files.dir != root {
		t.Fatalf("clicking the root breadcrumb should navigate to the root, got %q", m.files.dir)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestBreadcrumbClickNavigates -v 2>&1 | tail`
Expected: FAIL — the click on row 0 currently does nothing (sidebarRow returns -1 at Y=0).

- [ ] **Step 3: Handle a breadcrumb-row click**

In `main.go`'s `tea.MouseMsg` left-click branch, BEFORE the existing `row := sidebarRow(msg.Y, 1, m.files.height)` block, add a breadcrumb-row case (the breadcrumb is at sidebar screen row 0; the sidebar's left padding is 1 column):

```go
		if msg.Y == 0 {
			_, hits := m.files.breadcrumbBar(sidebarWidth - 3)
			col := msg.X - 1 // sidebar left padding
			for _, h := range hits {
				if col >= h.start && col < h.end {
					m.files.SetDir(h.path)
					m.focus = focusSidebar
					m.editor.Blur()
					break
				}
			}
			return m, nil
		}
```

(Place it inside the `inSidebar && left-press` block, right after `bannerH`/offset setup — i.e. only when the click is in the sidebar. The existing `row := sidebarRow(msg.Y, 1, ...)` handles Y≥1.)

- [ ] **Step 4: Run tests, full suite, build, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestBreadcrumbClick|TestSidebarClickRow|TestMouseClick' -v 2>&1 | tail -8
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "Clickable breadcrumb: click a segment to navigate"
```

---

## Task 5: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the file-pane docs**

In `README.md`, extend the file-pane/breadcrumb paragraph:

```markdown
The breadcrumb segments are clickable — click `okashi` or a parent folder to jump
there. On deep paths it shows `okashi / … / Drafts`, keeping the nearest folders
clickable. A `3/12` indicator appears when the list is taller than the pane.
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
git commit -m "Docs: clickable breadcrumb + scroll indicator"
```

- [ ] **Step 4: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` → confirm `chapter-01.md` no longer wraps when selected; the selection bar is a clean full-width accent row with a left gutter; file extensions are dimmed; navigate deep and click a breadcrumb segment to jump up; the `N/total` indicator shows when the list overflows.

---

## Self-Review

**Spec coverage (Plan 2 scope — spec Sections 3b + 4, plus the reported width bug):**
- Width-accounting fix → Task 1. Gutter / full-width bar / dim extensions → Task 2. Scroll indicator + breadcrumb segments + head-truncation → Task 3. Clickable breadcrumb → Tasks 3 (ranges) + 4 (mouse). Docs → Task 5.

**Placeholder scan:** none — full code in every step.

**Type consistency:** `breadcrumbSeg`, `segHit`, `breadcrumbSegments`, `breadcrumbBar` are defined in Task 3 and consumed in Task 4. `sidebarWidth-3` (content budget) is used consistently in Tasks 1, 3, 4. The `TestBreadcrumbSegments` expectation uses the literal current path; the implementer should assert `segs[2].path == "/home/me/okashi/Book/Drafts"` (the test as written pins that).

**Cross-cutting check:** the breadcrumb width (`sidebarWidth-3`) passed to `breadcrumbBar` in both the View render (Task 3) and the click hit-test (Task 4) MUST match, so click columns line up with the rendered row; both use `sidebarWidth-3`. The breadcrumb row is screen row 0 with a 1-col left padding (`col = X-1`).
