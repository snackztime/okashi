# Long-form Plan B — Outline View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward from Plan A):** a recurring generation bug
> emits a stray `court` token and/or drops the `antml:` namespace, silently
> no-op'ing tool calls. Mitigation: one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** A full-screen **outline view** for a manuscript — list ordered sections with titles + word counts, open one in the editor, reorder them (renumbering files on disk, backed up and behind a confirm gate), and add a new section after the selection.

**Architecture:** A self-contained `outline.go` holding an `outlineModel` (mirrors `filelist`) plus pure, unit-tested renumber helpers (`planRenames`, `planInsertRenames`) and a thin disk layer (`applyRenames`, `commitReorder`). `main.go` adds `screenOutline` to the `screen` enum and delegates key/mouse/`View` to the outline when that screen is active. Builds entirely on the Plan A data layer (`orderedSections`, `sectionTitle`, `projectWordCount`, `wordCountCache`, `backupFiles`/`backupStamp`), already on `main`.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, `charmbracelet/x/ansi`.

**Design spec:** `docs/superpowers/specs/2026-06-26-outline-view-plan-b-design.md` (this plan governs the code; the spec governs intent).

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` and gofmt as `/opt/homebrew/bin/gofmt` (not on PATH).
- **Manuscript** = a folder with ≥1 numerically-prefixed file; the outline operates on `m.files.dir` and is reachable only when `isManuscript(m.files.entries)`.
- **Renumber is lossless:** only the leading digit run is replaced; everything after it (separator, title slug, extension) is kept verbatim. `02-the-letter.md` → `01-the-letter.md`.
- **Pad width** = `max(2, digits(count), widest-existing-prefix-width)`: auto-grows at 100, **never shrinks**.
- **Reorder is deferred + confirmed:** moves rearrange in memory; on leave/open with a pending change a confirm gate (`y` apply · `n` discard · `esc` keep editing) runs. Apply = backup (via `backupFiles`) then a **two-phase** `os.Rename` (temp pass) so swaps don't collide. Renames confined to the project dir (no path escape). The open file's path follows a rename.
- **No separator row** in the outline list — sections then loose files, grouped by position + styling, so the screen-row↔selectable-index map stays 1:1 (mouse/keyboard).
- Deferred (NOT this plan): manuscript view (`m` is a stub), section delete, standalone `b` backup.
- Tests are hermetic: `t.TempDir()` and `t.Setenv("OKASHI_DIR", …)`; pure helpers take timestamps in (no wall-clock inside helpers).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Pure renumber & title helpers (`outline.go`)

**Files:**
- Create: `outline.go`, `outline_test.go`

**Interfaces:**
- Consumes: `fileEntry` (existing, `filelist.go`).
- Produces (package `main`):
  - `type renameOp struct { from, to string }` — base names within a project dir.
  - `func splitPrefix(name string) (digits, rest string)`
  - `func existingPrefixWidth(sections []fileEntry) int`
  - `func padWidth(count, existingWidth int) int`
  - `func planRenames(ordered []fileEntry, width int) []renameOp`
  - `func projectTitle(name string) string`

- [ ] **Step 1: Write the failing tests**

Create `outline_test.go`:

```go
package main

import "testing"

func TestSplitPrefix(t *testing.T) {
	cases := []struct{ name, digits, rest string }{
		{"02-the-letter.md", "02", "-the-letter.md"},
		{"1-x.md", "1", "-x.md"},
		{"notes.md", "", "notes.md"},
		{"01.md", "01", ".md"},
	}
	for _, c := range cases {
		d, r := splitPrefix(c.name)
		if d != c.digits || r != c.rest {
			t.Errorf("splitPrefix(%q) = (%q,%q), want (%q,%q)", c.name, d, r, c.digits, c.rest)
		}
	}
}

func TestPadWidth(t *testing.T) {
	cases := []struct{ count, existing, want int }{
		{3, 2, 2},    // small project, 2 digits
		{99, 2, 2},   // still 2
		{100, 2, 3},  // crossing 100 widens
		{50, 3, 3},   // never shrink below existing width
		{1, 0, 2},    // floor of 2
	}
	for _, c := range cases {
		if got := padWidth(c.count, c.existing); got != c.want {
			t.Errorf("padWidth(%d,%d) = %d, want %d", c.count, c.existing, got, c.want)
		}
	}
}

func TestExistingPrefixWidth(t *testing.T) {
	w := existingPrefixWidth([]fileEntry{{name: "01-a.md"}, {name: "001-b.md"}, {name: "notes.md"}})
	if w != 3 {
		t.Fatalf("existingPrefixWidth = %d, want 3 (widest run)", w)
	}
}

func TestPlanRenamesReorder(t *testing.T) {
	// Move section #3 up one slot: working order [01,03,02].
	working := []fileEntry{{name: "01-a.md"}, {name: "03-c.md"}, {name: "02-b.md"}}
	ops := planRenames(working, 2)
	// 01-a stays; 03-c -> 02-c; 02-b -> 03-b.
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["03-c.md"] != "02-c.md" || got["02-b.md"] != "03-b.md" {
		t.Fatalf("ops = %v, want 03-c->02-c and 02-b->03-b", got)
	}
	if _, ok := got["01-a.md"]; ok {
		t.Fatalf("01-a.md should not be renamed (already correct)")
	}
}

func TestPlanRenamesNoop(t *testing.T) {
	working := []fileEntry{{name: "01-a.md"}, {name: "02-b.md"}}
	if ops := planRenames(working, 2); len(ops) != 0 {
		t.Fatalf("already-correct order should yield no ops, got %v", ops)
	}
}

func TestPlanRenamesWidens(t *testing.T) {
	working := []fileEntry{{name: "1-a.md"}, {name: "2-b.md"}}
	ops := planRenames(working, 2)
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["1-a.md"] != "01-a.md" || got["2-b.md"] != "02-b.md" {
		t.Fatalf("ops = %v, want zero-padded to width 2", got)
	}
}

func TestProjectTitle(t *testing.T) {
	cases := map[string]string{
		"my-novel":         "my novel",
		"2024-trip-journal": "2024 trip journal", // leading digits NOT stripped
		"Essays_draft":     "Essays draft",
	}
	for in, want := range cases {
		if got := projectTitle(in); got != want {
			t.Errorf("projectTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSplitPrefix|TestPadWidth|TestExistingPrefixWidth|TestPlanRenames|TestProjectTitle' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `outline.go`**

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// renameOp is a single base-name rename within a manuscript dir.
type renameOp struct {
	from, to string
}

// splitPrefix splits name into its leading run of digits and the remainder
// (everything after the digits, verbatim). "02-the-letter.md" -> ("02",
// "-the-letter.md"); "notes.md" -> ("", "notes.md"). Renumbering keeps rest
// untouched, so the title slug, separator, and extension survive losslessly.
func splitPrefix(name string) (digits, rest string) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	return name[:i], name[i:]
}

// existingPrefixWidth returns the widest leading-digit-run length among the
// sections (0 if none). Used so renumbering never shrinks the pad width.
func existingPrefixWidth(sections []fileEntry) int {
	w := 0
	for _, s := range sections {
		if d, _ := splitPrefix(s.name); len(d) > w {
			w = len(d)
		}
	}
	return w
}

// padWidth picks the zero-pad width for count sections: at least 2, at least the
// digits needed for count, and never narrower than the existing width.
func padWidth(count, existingWidth int) int {
	w := 2
	if d := len(fmt.Sprintf("%d", count)); d > w {
		w = d
	}
	if existingWidth > w {
		w = existingWidth
	}
	return w
}

// planRenames maps an ordered section list onto contiguous, zero-padded prefixes
// of the given width, keeping everything after the old digit run verbatim. Ops
// whose name is already correct are omitted.
func planRenames(ordered []fileEntry, width int) []renameOp {
	var ops []renameOp
	for i, e := range ordered {
		_, rest := splitPrefix(e.name)
		next := fmt.Sprintf("%0*d", width, i+1) + rest
		if next != e.name {
			ops = append(ops, renameOp{from: e.name, to: next})
		}
	}
	return ops
}

// projectTitle de-slugs a manuscript folder name for display: drop a trailing
// extension if any, turn -/_ into spaces. Unlike sectionTitle it does NOT strip a
// leading digit run ("2024-trip-journal" -> "2024 trip journal").
func projectTitle(name string) string {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSplitPrefix|TestPadWidth|TestExistingPrefixWidth|TestPlanRenames|TestProjectTitle' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w outline.go outline_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add outline.go outline_test.go
git commit -m "outline: pure renumber + project-title helpers"
```

---

## Task 2: Disk renumber & backup (`outline.go`)

**Files:**
- Modify: `outline.go`
- Test: `outline_test.go`

**Interfaces:**
- Consumes: `planRenames`, `padWidth`, `existingPrefixWidth`, `renameOp` (Task 1); `withinRoot`, `fileEntry` (existing); `backupFiles` (existing, `backup.go`).
- Produces:
  - `func applyRenames(dir string, ops []renameOp) error` — two-phase temp rename, confined to dir.
  - `func commitReorder(dir string, working []fileEntry, stamp string) (map[string]string, error)` — backup + renumber to the working order; returns old→new **absolute** paths for moved files (nil if order already correct). `stamp` is supplied by the caller (no wall-clock here).

- [ ] **Step 1: Write the failing tests**

Add to `outline_test.go` (add imports `os`, `path/filepath`, `strings`):

```go
func TestApplyRenamesSwapNoCollision(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("AAA"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("BBB"), 0o644)
	// Swap their numbers: 01-a -> 02-a, 02-b -> 01-b.
	ops := []renameOp{{"01-a.md", "02-a.md"}, {"02-b.md", "01-b.md"}}
	if err := applyRenames(dir, ops); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "02-a.md")); string(b) != "AAA" {
		t.Fatalf("02-a.md content = %q, want AAA", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "01-b.md")); string(b) != "BBB" {
		t.Fatalf("01-b.md content = %q, want BBB", b)
	}
}

func TestApplyRenamesRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	err := applyRenames(dir, []renameOp{{"01-a.md", "../escaped.md"}})
	if err == nil {
		t.Fatal("expected an error for a target escaping the project dir")
	}
}

func TestCommitReorderBacksUpAndRenames(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "03-c.md"), []byte("c"), 0o644)
	// Working order with #3 moved up one: [01-a, 03-c, 02-b].
	working := []fileEntry{{name: "01-a.md"}, {name: "03-c.md"}, {name: "02-b.md"}}
	moved, err := commitReorder(dir, working, "STAMP")
	if err != nil {
		t.Fatal(err)
	}
	// 03-c -> 02-c and 02-b -> 03-b on disk.
	if _, err := os.Stat(filepath.Join(dir, "02-c.md")); err != nil {
		t.Fatalf("expected 02-c.md after reorder: %v", err)
	}
	if moved[filepath.Join(dir, "03-c.md")] != filepath.Join(dir, "02-c.md") {
		t.Fatalf("moved map should record 03-c -> 02-c, got %v", moved)
	}
	// A backup snapshot of the pre-reorder files exists.
	if _, err := os.Stat(filepath.Join(dir, ".backup", "STAMP", "01-a.md")); err != nil {
		t.Fatalf("expected pre-reorder backup: %v", err)
	}
}

func TestCommitReorderNoopNoBackup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("b"), 0o644)
	working := []fileEntry{{name: "01-a.md"}, {name: "02-b.md"}}
	moved, err := commitReorder(dir, working, "STAMP")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 0 {
		t.Fatalf("no-op reorder should move nothing, got %v", moved)
	}
	if _, err := os.Stat(filepath.Join(dir, ".backup")); !os.IsNotExist(err) {
		t.Fatalf("no-op reorder should not create a backup dir")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestApplyRenames|TestCommitReorder' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement (append to `outline.go`)**

Add `"os"` to `outline.go`'s imports, then:

```go
// applyRenames performs ops within dir using a two-phase temp pass so that order
// swaps (01<->02) don't collide. Every target must stay inside dir.
func applyRenames(dir string, ops []renameOp) error {
	type pend struct{ tmp, final string }
	var pending []pend
	for i, op := range ops {
		final := filepath.Join(dir, op.to)
		if !withinRoot(final, dir) {
			return fmt.Errorf("rename target escapes project: %s", op.to)
		}
		tmp := filepath.Join(dir, fmt.Sprintf(".okashi-renumber-%d.tmp", i))
		if err := os.Rename(filepath.Join(dir, op.from), tmp); err != nil {
			return err
		}
		pending = append(pending, pend{tmp: tmp, final: final})
	}
	for _, p := range pending {
		if err := os.Rename(p.tmp, p.final); err != nil {
			return err
		}
	}
	return nil
}

// commitReorder snapshots the section files, then renumbers them on disk to match
// the working order. Returns old->new absolute paths for moved files (nil if the
// order was already correct). stamp is supplied by the caller.
func commitReorder(dir string, working []fileEntry, stamp string) (map[string]string, error) {
	width := padWidth(len(working), existingPrefixWidth(working))
	ops := planRenames(working, width)
	if len(ops) == 0 {
		return nil, nil
	}
	var paths []string
	for _, w := range working {
		paths = append(paths, filepath.Join(dir, w.name))
	}
	if err := backupFiles(dir, stamp, paths); err != nil {
		return nil, err
	}
	if err := applyRenames(dir, ops); err != nil {
		return nil, err
	}
	moved := make(map[string]string, len(ops))
	for _, op := range ops {
		moved[filepath.Join(dir, op.from)] = filepath.Join(dir, op.to)
	}
	return moved, nil
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestApplyRenames|TestCommitReorder' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w outline.go outline_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add outline.go outline_test.go
git commit -m "outline: two-phase disk renumber + backup-first commit"
```

---

## Task 3: outlineModel state + rows + render (`outline.go`)

**Files:**
- Modify: `outline.go`
- Test: `outline_test.go`

**Interfaces:**
- Consumes: `orderedSections`, `sectionTitle`, `projectWordCount`, `wordCountCache`, `commafy` (existing); `projectTitle`, `splitPrefix` (Tasks 1); `selectedStyle`, `subtle`, `accent` (existing styles); `lipgloss`, `ansi` (existing deps).
- Produces:
  - `type outlineRow struct { entry fileEntry; isSection bool }`
  - `type outlineModel struct { … }` (fields below)
  - `func (o *outlineModel) load(dir string, wc *wordCountCache)`
  - `func (o outlineModel) rows() []outlineRow`
  - `func (o outlineModel) dirty() bool`
  - `func (o *outlineModel) moveSelection(d int)`
  - `func (o *outlineModel) moveSection(d int)`
  - `func (o outlineModel) selectedRow() (outlineRow, bool)`
  - `const outlineHeaderHeight = 2`
  - `func (o outlineModel) View() string`

- [ ] **Step 1: Write the failing tests**

Add to `outline_test.go`:

```go
func TestOutlineLoadAndRows(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	rows := o.rows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (2 sections + 1 loose)", len(rows))
	}
	if rows[0].entry.name != "01-a.md" || !rows[0].isSection {
		t.Fatalf("row 0 should be section 01-a.md, got %+v", rows[0])
	}
	if rows[2].entry.name != "notes.md" || rows[2].isSection {
		t.Fatalf("row 2 should be loose notes.md, got %+v", rows[2])
	}
}

func TestOutlineMoveSectionMakesDirty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	if o.dirty() {
		t.Fatal("freshly loaded outline should not be dirty")
	}
	o.selected = 0
	o.moveSection(1) // move section 1 down
	if !o.dirty() {
		t.Fatal("after moving a section the outline should be dirty")
	}
	if o.working[0].name != "02-b.md" || o.working[1].name != "01-a.md" {
		t.Fatalf("working order should be swapped, got %v", o.working)
	}
	if o.selected != 1 {
		t.Fatalf("selection should follow the moved section to index 1, got %d", o.selected)
	}
}

func TestOutlineViewShowsTitlesCountsAndTotal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("a b"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	o.width = 60
	o.height = 12
	view := o.View()
	if !strings.Contains(view, "opening") || strings.Contains(view, "01-opening") {
		t.Fatalf("outline should show stripped title 'opening', not the raw filename:\n%s", view)
	}
	if !strings.Contains(view, "3w") {
		t.Fatalf("outline should show the per-section count '3w':\n%s", view)
	}
	if !strings.Contains(view, "5w") {
		t.Fatalf("outline header should show the project total '5w':\n%s", view)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineLoad|TestOutlineMoveSection|TestOutlineViewShows' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement (append to `outline.go`)**

Add `"strings"` is already imported; add `"github.com/charmbracelet/lipgloss"` and `"github.com/charmbracelet/x/ansi"` to `outline.go`'s imports, then:

```go
const outlineHeaderHeight = 2 // title line + blank spacer

// outlineRow is one selectable row: a numbered section or a loose file.
type outlineRow struct {
	entry     fileEntry
	isSection bool
}

// outlineModel is the full-screen manuscript outline. working is the (possibly
// reordered) section order; disk is the on-disk order, for dirty detection.
type outlineModel struct {
	dir      string
	working  []fileEntry
	disk     []fileEntry
	loose    []fileEntry
	selected int
	width    int
	height   int
	wc       *wordCountCache
	confirm  bool // apply/discard gate visible
}

// load reads dir's sections (ordered) and loose files into the outline.
func (o *outlineModel) load(dir string, wc *wordCountCache) {
	entries := readEntries(dir)
	sections, loose := orderedSections(entries)
	o.dir = dir
	o.working = sections
	o.disk = append([]fileEntry(nil), sections...)
	o.loose = loose
	o.selected = 0
	o.wc = wc
	o.confirm = false
}

// readEntries lists dir's non-hidden .md/.txt files as fileEntry values (dirs
// excluded — the outline lists section files only).
func readEntries(dir string) []fileEntry {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") || it.IsDir() {
			continue
		}
		out = append(out, fileEntry{name: name})
	}
	return out
}

// rows returns the selectable rows: working sections, then loose files.
func (o outlineModel) rows() []outlineRow {
	rows := make([]outlineRow, 0, len(o.working)+len(o.loose))
	for _, e := range o.working {
		rows = append(rows, outlineRow{entry: e, isSection: true})
	}
	for _, e := range o.loose {
		rows = append(rows, outlineRow{entry: e, isSection: false})
	}
	return rows
}

// dirty reports whether the working order differs from the on-disk order.
func (o outlineModel) dirty() bool {
	if len(o.working) != len(o.disk) {
		return true
	}
	for i := range o.working {
		if o.working[i].name != o.disk[i].name {
			return true
		}
	}
	return false
}

// moveSelection moves the cursor by d, clamped across all rows.
func (o *outlineModel) moveSelection(d int) {
	n := len(o.working) + len(o.loose)
	if n == 0 {
		return
	}
	o.selected += d
	if o.selected < 0 {
		o.selected = 0
	}
	if o.selected >= n {
		o.selected = n - 1
	}
}

// moveSection moves the selected section by d within the working order (no-op
// unless the selection is a section). The selection follows the moved section.
func (o *outlineModel) moveSection(d int) {
	i := o.selected
	if i < 0 || i >= len(o.working) {
		return // selection is a loose row (or empty): not reorderable
	}
	j := i + d
	if j < 0 || j >= len(o.working) {
		return
	}
	o.working[i], o.working[j] = o.working[j], o.working[i]
	o.selected = j
}

// selectedRow returns the row under the cursor.
func (o outlineModel) selectedRow() (outlineRow, bool) {
	rows := o.rows()
	if o.selected < 0 || o.selected >= len(rows) {
		return outlineRow{}, false
	}
	return rows[o.selected], true
}

// View renders the outline: a header line, then one row per section/loose file.
func (o outlineModel) View() string {
	title := projectTitle(filepath.Base(o.dir))
	total := projectWordCount(o.dir, o.working, o.wc)
	head := fmt.Sprintf("%s · %sw · %d sections", title, commafy(total), len(o.working))
	if o.dirty() {
		head += "   ● unsaved order"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(head, o.width, "…")))
	b.WriteString("\n\n") // outlineHeaderHeight = 2 rows

	rows := o.rows()
	for i, r := range rows {
		var line string
		if r.isSection {
			digits, _ := splitPrefix(r.entry.name)
			count := commafy(o.wc.count(filepath.Join(o.dir, r.entry.name))) + "w"
			left := " " + digits + "  " + sectionTitle(r.entry.name)
			maxLeft := o.width - lipgloss.Width(count) - 1
			if maxLeft < 1 {
				maxLeft = 1
			}
			left = ansi.Truncate(left, maxLeft, "…")
			gap := o.width - lipgloss.Width(left) - lipgloss.Width(count)
			if gap < 1 {
				gap = 1
			}
			line = left + strings.Repeat(" ", gap) + count
		} else {
			line = ansi.Truncate(" "+r.entry.name, o.width, "…")
		}
		switch {
		case i == o.selected:
			b.WriteString(selectedStyle.Width(o.width).Render(ansi.Truncate(line, o.width, "…")))
		case !r.isSection:
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(line))
		default:
			b.WriteString(line)
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run the tests; gofmt; full suite; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineLoad|TestOutlineMoveSection|TestOutlineViewShows' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w outline.go outline_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add outline.go outline_test.go
git commit -m "outline: model state, selectable rows, and full-screen render"
```

---

## Task 4: Screen wiring — enter, render, select, open, back (`main.go`)

**Files:**
- Modify: `main.go` (`screen` const, `model` struct, `Update`, `View`, `layout`, status string)
- Test: `outline_wiring_test.go` (create)

**Interfaces:**
- Consumes: `outlineModel`, `outlineHeaderHeight`, `isManuscript` (existing); `loadFile`, `m.files` (existing).
- Produces: `screenOutline`; `model.outline outlineModel`; `func (m *model) enterOutline()`; `func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd)`; `func (m model) outlineView() string`. Entry is `ctrl+l` from the editor when `isManuscript(m.files.entries)`.

- [ ] **Step 1: Write the failing tests**

Create `outline_wiring_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func setupManuscript(t *testing.T) (model, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-b.md"), []byte("three"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(proj)
	return m, proj
}

func TestCtrlLEntersOutlineInManuscript(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("ctrl+l in a manuscript should enter screenOutline, got %v", m.screen)
	}
	if len(m.outline.working) != 2 {
		t.Fatalf("outline should load 2 sections, got %d", len(m.outline.working))
	}
}

func TestCtrlLRejectedOutsideManuscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "loose.md"), []byte("x"), 0o644)
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(root)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen == screenOutline {
		t.Fatal("ctrl+l outside a manuscript should not enter the outline")
	}
}

func TestOutlineEnterOpensSection(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select 02-b
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("Enter should return to the editor, got screen %v", m.screen)
	}
	if m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("Enter should open the selected section, currentFile = %q", m.currentFile)
	}
}

func TestOutlineEscReturnsToEditor(t *testing.T) {
	m, _ := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.screen != screenWriting {
		t.Fatalf("esc from the outline should return to the editor, got %v", m.screen)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestCtrlL|TestOutlineEnter|TestOutlineEsc' 2>&1 | tail`
Expected: build error — `screenOutline` undefined, etc.

- [ ] **Step 3: Add the screen, struct field, and entry helper**

In `main.go`, extend the `screen` const block (currently `screenHome`, `screenWriting`):

```go
const (
	screenHome screen = iota
	screenWriting
	screenOutline
)
```

Add to the `model` struct (after `icons iconSet`):

```go
	outline outlineModel
```

Add the entry helper (place near `loadFile`):

```go
// enterOutline opens the manuscript outline for the current pane dir. Caller
// must have verified isManuscript(m.files.entries).
func (m *model) enterOutline() {
	m.outline.width = m.width
	m.outline.height = m.height - 1 // status bar
	m.outline.load(m.files.dir, m.files.wc)
	m.screen = screenOutline
	m.previewing = false
	m.status = "outline · ↑↓ select · J/K reorder · enter open · n new · esc back"
}
```

- [ ] **Step 4: Wire `ctrl+l` entry and screen dispatch in `Update`**

In `Update`, add the outline dispatch right after the home dispatch (`if m.screen == screenHome { return m.updateHome(msg) }`):

```go
	if m.screen == screenOutline {
		return m.updateOutline(msg)
	}
```

In the editor `case tea.KeyMsg` switch (alongside `ctrl+p`, `ctrl+t`, …), add:

```go
		case "ctrl+l":
			if isManuscript(m.files.entries) {
				m.enterOutline()
			} else {
				m.status = "not a manuscript folder (no numbered sections)"
			}
			return m, nil
```

- [ ] **Step 5: Add `updateOutline` and `outlineView`**

Add to `main.go`:

```go
// updateOutline handles input on the outline screen: select, open, back.
// (Reorder and new-section are layered on in later tasks.)
func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.outline.moveSelection(-1)
	case "down", "j":
		m.outline.moveSelection(1)
	case "enter":
		if row, ok := m.outline.selectedRow(); ok {
			m.loadFile(filepath.Join(m.outline.dir, row.entry.name))
			m.screen = screenWriting
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "m":
		m.status = "manuscript view — Plan C"
	case "esc":
		m.screen = screenWriting
		m.focus = focusEditor
		m.editor.Focus()
	}
	return m, nil
}

// outlineView renders the outline screen with the status bar.
func (m model) outlineView() string {
	body := m.outline.View()
	status := statusStyle.Width(m.width).Render(m.statusBar())
	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}
```

In `View`, add the dispatch after the home check (`if m.screen == screenHome { return m.homeView() }`):

```go
	if m.screen == screenOutline {
		return m.outlineView()
	}
```

- [ ] **Step 6: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestCtrlL|TestOutlineEnter|TestOutlineEsc' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go outline_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go outline_wiring_test.go
git commit -m "outline: screen wiring — ctrl+l entry, select, open, back"
```

---

## Task 5: Reorder with confirm-on-exit commit (`main.go`, `outline.go`)

**Files:**
- Modify: `main.go` (`updateOutline`)
- Test: `outline_wiring_test.go`

**Interfaces:**
- Consumes: `outlineModel.moveSection`/`dirty`/`load`, `commitReorder`, `backupStamp` (existing), `m.files.SetDir`, `m.currentFile`.
- Produces: reorder keys (`J`/`K`, `shift+↑`/`shift+↓`) move the selected section; a confirm gate (`y`/`n`/`esc`) fires when leaving (`esc`) or opening (`enter`) with `o.dirty()`; apply = `commitReorder` (backup+rename), `m.currentFile` follows a rename, `m.files` refreshes, outline reloads.

- [ ] **Step 1: Write the failing tests**

Add to `outline_wiring_test.go` (add `"time"` import if not present — not needed; uses key msgs only):

```go
func TestOutlineReorderCommitsOnEscConfirm(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// Move section 1 (01-a) down past 02-b.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	if !m.outline.dirty() {
		t.Fatal("after J the outline should be dirty")
	}
	// esc -> confirm gate appears, no disk change yet.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if !m.outline.confirm {
		t.Fatal("esc with a pending reorder should raise the confirm gate")
	}
	if _, err := os.Stat(filepath.Join(proj, "01-b.md")); !os.IsNotExist(err) {
		t.Fatal("disk must not change before the gate is confirmed")
	}
	// y -> apply: a now becomes section 02, b becomes 01.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-b.md")); err != nil {
		t.Fatalf("after confirm, 02-b should be renumbered to 01-b: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "02-a.md")); err != nil {
		t.Fatalf("after confirm, 01-a should be renumbered to 02-a: %v", err)
	}
	if m.screen != screenWriting {
		t.Fatalf("apply should complete the pending exit, got screen %v", m.screen)
	}
}

func TestOutlineReorderDiscard(t *testing.T) {
	m, proj := setupManuscript(t)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}) // discard
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-a.md")); err != nil {
		t.Fatalf("discard must leave the disk untouched: %v", err)
	}
	if m.screen != screenWriting {
		t.Fatalf("discard should complete the exit, got %v", m.screen)
	}
}

func TestOutlineReorderTracksOpenFile(t *testing.T) {
	m, proj := setupManuscript(t)
	m.currentFile = filepath.Join(proj, "01-a.md") // 01-a is open in the editor
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}}) // a moves to slot 2
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if m.currentFile != filepath.Join(proj, "02-a.md") {
		t.Fatalf("the open file path should follow the rename to 02-a.md, got %q", m.currentFile)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineReorder' 2>&1 | tail`
Expected: FAIL — no reorder keys / confirm gate yet.

- [ ] **Step 3: Add a commit helper to the model**

Add to `main.go` (near `enterOutline`):

```go
// commitOutlineOrder applies a pending reorder: backup + renumber on disk, then
// follow the open file's rename and refresh the sidebar + outline. Returns an
// error string for the status line (empty on success/no-op).
func (m *model) commitOutlineOrder() string {
	moved, err := commitReorder(m.outline.dir, m.outline.working, backupStamp(time.Now()))
	if err != nil {
		return "reorder failed: " + err.Error()
	}
	if newPath, ok := moved[m.currentFile]; ok {
		m.currentFile = newPath
	}
	m.files.SetDir(m.files.dir) // re-sort the sidebar to the new names
	m.outline.load(m.outline.dir, m.files.wc)
	return ""
}
```

- [ ] **Step 4: Handle the confirm gate and reorder keys in `updateOutline`**

Replace the body of `updateOutline` (from Task 4) with:

```go
func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// The apply/discard gate captures input while it is up.
	if m.outline.confirm {
		switch key.String() {
		case "y":
			m.outline.confirm = false
			if s := m.commitOutlineOrder(); s != "" {
				m.status = s
			}
			m.leaveOutlinePending()
			return m, nil
		case "n":
			m.outline.confirm = false
			m.outline.working = append([]fileEntry(nil), m.outline.disk...) // discard moves
			m.leaveOutlinePending()
			return m, nil
		case "esc":
			m.outline.confirm = false // keep editing the outline
			return m, nil
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.outline.moveSelection(-1)
	case "down", "j":
		m.outline.moveSelection(1)
	case "J", "shift+down":
		m.outline.moveSection(1)
	case "K", "shift+up":
		m.outline.moveSection(-1)
	case "enter":
		m.outline.pendingOpen = true
		return m.outlineLeave()
	case "m":
		m.status = "manuscript view — Plan C"
	case "esc":
		m.outline.pendingOpen = false
		return m.outlineLeave()
	}
	return m, nil
}

// outlineLeave handles an exit/open request: if a reorder is pending, raise the
// confirm gate; otherwise complete the action immediately.
func (m model) outlineLeave() (tea.Model, tea.Cmd) {
	if m.outline.dirty() {
		m.outline.confirm = true
		m.status = "apply reordering?  y apply · n discard · esc keep editing"
		return m, nil
	}
	m.leaveOutlinePending()
	return m, nil
}

// leaveOutlinePending completes a pending exit or open (set via pendingOpen).
func (m *model) leaveOutlinePending() {
	if m.outline.pendingOpen {
		if row, ok := m.outline.selectedRow(); ok {
			m.loadFile(filepath.Join(m.outline.dir, row.entry.name))
		}
	}
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}
```

Add a `pendingOpen bool` field to `outlineModel` in `outline.go` (after `confirm bool`):

```go
	pendingOpen bool // the pending leave is an open (Enter), not a back (esc)
```

- [ ] **Step 5: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineReorder|TestCtrlL|TestOutlineEnter|TestOutlineEsc' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w main.go outline.go outline_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go outline.go outline_wiring_test.go
git commit -m "outline: deferred reorder with confirm-on-exit commit + open-file tracking"
```

---

## Task 6: New section — insert after selection (`main.go`, `outline.go`)

**Files:**
- Modify: `main.go` (`updateOutline`, input routing), `outline.go` (insert planner)
- Test: `outline_test.go` (planner), `outline_wiring_test.go` (flow)

**Interfaces:**
- Consumes: `splitPrefix`, `padWidth`, `existingPrefixWidth`, `applyRenames`, `backupFiles`, `backupStamp`, `m.nameInput`.
- Produces:
  - `func planInsertRenames(working []fileEntry, insertIndex, width int) []renameOp` (pure).
  - `func commitInsert(dir, slug string, working []fileEntry, insertIndex int, stamp string) (newName string, moved map[string]string, err error)`.
  - `n` in the outline opens a name prompt; on enter the new section is created at `selected+1` and the rest renumber.

- [ ] **Step 1: Write the failing planner test**

Add to `outline_test.go`:

```go
func TestPlanInsertRenamesShiftsBelow(t *testing.T) {
	working := []fileEntry{{name: "01-a.md"}, {name: "02-b.md"}, {name: "03-c.md"}}
	// Insert after index 0 (new section takes slot 2): b->03, c->04.
	ops := planInsertRenames(working, 1, 2)
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["02-b.md"] != "03-b.md" || got["03-c.md"] != "04-c.md" {
		t.Fatalf("ops = %v, want b->03 and c->04", got)
	}
	if _, ok := got["01-a.md"]; ok {
		t.Fatalf("01-a.md is above the insert point and must not move")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestPlanInsertRenames' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement the insert planner and commit (`outline.go`)**

```go
// planInsertRenames renumbers the existing sections to open a gap at
// insertIndex (0-based slot the new section will occupy). Sections at or below
// insertIndex shift down by one; those above keep their number.
func planInsertRenames(working []fileEntry, insertIndex, width int) []renameOp {
	var ops []renameOp
	for i, e := range working {
		target := i + 1
		if i >= insertIndex {
			target = i + 2 // make room at insertIndex+1
		}
		_, rest := splitPrefix(e.name)
		next := fmt.Sprintf("%0*d", width, target) + rest
		if next != e.name {
			ops = append(ops, renameOp{from: e.name, to: next})
		}
	}
	return ops
}

// commitInsert backs up the sections, renumbers existing files to open a slot at
// insertIndex, then creates an empty <NN>-<slug>.md there. Returns the new file's
// base name and the old->new absolute paths of the shifted files.
func commitInsert(dir, slug string, working []fileEntry, insertIndex, padW int, stamp string) (string, map[string]string, error) {
	if insertIndex < 0 {
		insertIndex = 0
	}
	if insertIndex > len(working) {
		insertIndex = len(working)
	}
	var paths []string
	for _, w := range working {
		paths = append(paths, filepath.Join(dir, w.name))
	}
	if err := backupFiles(dir, stamp, paths); err != nil {
		return "", nil, err
	}
	ops := planInsertRenames(working, insertIndex, padW)
	if err := applyRenames(dir, ops); err != nil {
		return "", nil, err
	}
	newName := fmt.Sprintf("%0*d-%s.md", padW, insertIndex+1, slug)
	if err := os.WriteFile(filepath.Join(dir, newName), nil, 0o644); err != nil {
		return "", nil, err
	}
	moved := make(map[string]string, len(ops))
	for _, op := range ops {
		moved[filepath.Join(dir, op.from)] = filepath.Join(dir, op.to)
	}
	return newName, moved, nil
}

// slugify turns a typed section title into a filename slug: lowercase, spaces and
// underscores to hyphens, stripped of other punctuation.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "section"
	}
	return s
}
```

- [ ] **Step 4: Write the failing flow test**

Add to `outline_wiring_test.go`:

```go
func TestOutlineNewSectionInsertsAfterSelection(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a, 02-b ; select 01-a
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// n -> prompt; type a title; enter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = nm.(model)
	for _, r := range "scene two" {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// New section is slot 2; old 02-b shifts to 03-b.
	if _, err := os.Stat(filepath.Join(proj, "02-scene-two.md")); err != nil {
		t.Fatalf("expected new 02-scene-two.md after the selection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "03-b.md")); err != nil {
		t.Fatalf("expected 02-b renumbered to 03-b: %v", err)
	}
}
```

- [ ] **Step 5: Wire the `n` prompt into `updateOutline` and input routing**

Add `outlineCreating bool` to the `model` struct (after `outline outlineModel`).

In `updateOutline`, add a branch at the very top (before the confirm-gate block) to capture prompt input while creating:

```go
	if m.outlineCreating {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc":
				m.outlineCreating = false
				m.nameInput.Blur()
				m.status = "new section cancelled"
				return m, nil
			case "enter":
				m.confirmNewSection()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
```

Add the `n` case to the main key switch in `updateOutline`:

```go
	case "n":
		m.outlineCreating = true
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.status = "new section title — enter to create, esc to cancel"
		return m, textinput.Blink
```

Add the confirm handler to `main.go`:

```go
// confirmNewSection creates a new section after the selected one and renumbers
// the rest. The new file opens in the editor.
func (m *model) confirmNewSection() {
	m.outlineCreating = false
	m.nameInput.Blur()
	title := strings.TrimSpace(m.nameInput.Value())
	if title == "" {
		m.status = "new section cancelled (no title)"
		return
	}
	insertIndex := m.outline.selected + 1
	if m.outline.selected >= len(m.outline.working) {
		insertIndex = len(m.outline.working) // a loose row is selected: append
	}
	padW := padWidth(len(m.outline.working)+1, existingPrefixWidth(m.outline.working))
	newName, moved, err := commitInsert(m.outline.dir, slugify(title), m.outline.working, insertIndex, padW, backupStamp(time.Now()))
	if err != nil {
		m.status = "new section failed: " + err.Error()
		return
	}
	if newPath, ok := moved[m.currentFile]; ok {
		m.currentFile = newPath
	}
	m.files.SetDir(m.files.dir)
	m.loadFile(filepath.Join(m.outline.dir, newName))
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.status = "new section: " + newName + " — ctrl+s to save"
}
```

Confirm `main.go` imports `"github.com/charmbracelet/bubbles/textinput"` (it does — `nameInput` is a `textinput.Model`).

- [ ] **Step 6: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPlanInsertRenames|TestOutlineNewSection' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go outline.go outline_test.go outline_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go outline.go outline_test.go outline_wiring_test.go
git commit -m "outline: new section inserted after selection with renumber + backup"
```

---

## Task 7: Mouse — click to select, double-click to open (`main.go`)

**Files:**
- Modify: `main.go` (`updateOutline` mouse handling)
- Test: `outline_wiring_test.go`

**Interfaces:**
- Consumes: `outlineModel.rows`, `outlineHeaderHeight`, `loadFile`, `m.lastClickRow`/`lastClickTime` (existing double-click state), `sidebarRow` (existing).
- Produces: in the outline, a left-click sets the selection from the hit-tested row; a second click on the same row within 400ms opens it (mirrors the file-pane behavior). Clicks are ignored while the confirm gate or the new-section prompt is up.

- [ ] **Step 1: Write the failing test**

Add to `outline_wiring_test.go`:

```go
func TestOutlineClickSelectsThenDoubleClickOpens(t *testing.T) {
	m, proj := setupManuscript(t) // 01-a (row 0), 02-b (row 1)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	// Click row 1 (02-b): mouse Y = header height + 1.
	clickY := outlineHeaderHeight + 1
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.outline.selected != 1 {
		t.Fatalf("click should select row 1, got %d", m.outline.selected)
	}
	// Second click on the same row opens it.
	nm, _ = m.Update(tea.MouseMsg{X: 2, Y: clickY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.screen != screenWriting || m.currentFile != filepath.Join(proj, "02-b.md") {
		t.Fatalf("double-click should open 02-b.md, screen=%v file=%q", m.screen, m.currentFile)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineClick' 2>&1 | tail`
Expected: FAIL — mouse not handled in the outline.

- [ ] **Step 3: Handle mouse in `updateOutline`**

In `updateOutline`, before the `key, ok := msg.(tea.KeyMsg)` line, add a mouse branch (and skip it while a prompt/gate is up):

```go
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if m.outlineCreating || m.outline.confirm {
			return m, nil
		}
		if mouse.Button != tea.MouseButtonLeft || mouse.Action != tea.MouseActionPress {
			return m, nil
		}
		row := sidebarRow(mouse.Y, outlineHeaderHeight, len(m.outline.rows()))
		if row < 0 {
			return m, nil
		}
		m.outline.selected = row
		now := time.Now()
		if row == m.lastClickRow && now.Sub(m.lastClickTime) < 400*time.Millisecond {
			if r, ok := m.outline.selectedRow(); ok {
				m.loadFile(filepath.Join(m.outline.dir, r.entry.name))
				m.screen = screenWriting
				m.focus = focusEditor
				m.editor.Focus()
			}
			m.lastClickTime = time.Time{}
		} else {
			m.lastClickRow = row
			m.lastClickTime = now
		}
		return m, nil
	}
```

- [ ] **Step 4: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineClick' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go outline_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go outline_wiring_test.go
git commit -m "outline: mouse — click selects, double-click opens (shared hit-test)"
```

---

## Task 8: Docs & status line

**Files:**
- Modify: `README.md` (keymap / features), the editor status string (`main.go:192`).

**Interfaces:** none (docs + a status-hint string).

- [ ] **Step 1: Add `ctrl+l` to the editor status hint**

In `initialModel` (the `status:` field, currently ending `… · ctrl+s save · ctrl+c quit`), insert the outline hint:

```go
		status:         "ctrl+b sidebar · esc switch · ctrl+n new · ctrl+l outline · ctrl+p preview · ctrl+t typewriter · ctrl+d dim · ctrl+s save · ctrl+c quit",
```

- [ ] **Step 2: Document the outline in `README.md`**

Add a section (match the README's existing heading style — read it first):

```markdown
### Outline view (manuscripts)

Inside a manuscript folder (any folder with numerically-prefixed sections like
`01-opening.md`), press **ctrl+l** to open the outline:

- `↑ ↓` / `j k` — move the selection; **enter** (or double-click) opens a section.
- `J K` or `shift+↑ ↓` — reorder the selected section. Reordering is staged; on
  **esc**/enter you're asked to **apply** (`y`), **discard** (`n`), or keep
  editing (`esc`). Applying renumbers the files on disk after writing a
  `.backup/` snapshot.
- `n` — new section, inserted after the selection (the rest renumber).
- `esc` — back to the editor.
```

- [ ] **Step 3: Verify build + full suite; commit**

```bash
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go README.md
git commit -m "docs: outline view keymap + ctrl+l status hint"
```

---

## Self-Review

**Spec coverage (Plan B = design spec §4 + the finalized decisions):**
- Screen + entry/exit (`screenOutline`, `ctrl+l` manuscript-gated, `esc` back, `m` stub) → Task 4.
- Layout & render (shared `rows()` for render+hit-test, header title·total·count, `NN Title ···· Nw`, dim loose group, unsaved marker) → Task 3 (render), Task 7 (hit-test reuses `rows()`).
- Select & open (↑↓/jk, Enter/double-click, single-click select) → Task 4 (keys), Task 7 (mouse).
- Reorder (J/K + shift-arrows, deferred, confirm `y`/`n`/`esc`, backup+two-phase rename, open-file follows, sidebar refresh) → Tasks 1–2 (renumber), Task 5 (interaction).
- New section (`n`, insert-after-selected, renumber below, backup) → Task 6.
- Numbering width (`max(2, digits(count), widest-existing)`, grow-not-shrink) → Task 1 (`padWidth`), exercised in Tasks 2/6.
- Deferred items (`m` stub, no delete, no standalone `b`) honored — none implemented.

**Placeholder scan:** none — full code in every step.

**Type consistency:** `renameOp{from,to}` used across Tasks 1–2,6; `planRenames(ordered,width)`, `planInsertRenames(working,insertIndex,width)`, `applyRenames(dir,ops)`, `commitReorder(dir,working,stamp)`, `commitInsert(dir,slug,working,insertIndex,padW,stamp)` consistent where consumed; `outlineModel` fields (`dir/working/disk/loose/selected/width/height/wc/confirm/pendingOpen`) introduced in Task 3 and 5 and used consistently; `outlineHeaderHeight` shared by render (Task 3) and hit-test (Task 7); `model.outline`, `model.outlineCreating` consistent across Tasks 4–7. `readEntries` lists files only (dirs excluded), matching `orderedSections`' input contract.

**Risk checks baked into tests:** two-phase rename swap without collision (Task 2); path-escape rejected (Task 2); disk untouched until the gate is confirmed + discard leaves disk clean (Task 5); open-file path follows a rename (Task 5); insert shifts only files at/below the point (Task 6); hit-test row math matches the render header height (Task 7).

**Note for the executor:** Task 5 introduces `pendingOpen` on `outlineModel` and replaces the Task-4 `updateOutline` body wholesale — read both tasks together. `outlineLeave()` is a value-receiver method returning `(tea.Model, tea.Cmd)` to match Bubble Tea's update style; it reads `m.outline.dirty()` and either raises the gate or calls `leaveOutlinePending` (a pointer method on the addressable local copy `m`, which is then returned).
