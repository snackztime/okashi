> **RESUME NOTE — Plan A DONE; paused 2026-06-22, resume 2026-06-23+**
>
> **TOOL-CALL WORKAROUND (critical):** a recurring generation bug emits a stray
> `court` token and/or drops the `antml:` namespace, silently no-op'ing tool
> calls. The mitigation that works: emit **exactly one tool call per message, as
> the FIRST element of the reply** (no prose before it), and put all explanation
> AFTER the tool result. The opening tag MUST be `<invoke name="...">` with
> `<parameter name="...">` children. (See memory `tool-call-syntax-court-bug`.)
>
> **STATUS: Plan A COMPLETE — merged + pushed to `main` @ `b104314`** (was 4 plans).
> Executed via subagent-driven-development on 2026-06-22 (branch
> `feat/longform-project-model`, ff-merged; branch left for cleanup). Build,
> `go vet`, full `go test ./...` clean on main. Spec:
> `docs/superpowers/specs/2026-06-22-long-form-projects-design.md`.
>
> **Commits (all 4 tasks + 1 final-review fix):**
> - `18a4d1f` Task 1 — `project.go`: `sectionOrder`/`sectionTitle`/`orderedSections`/`isManuscript`
> - `dc32468` Task 2 — `project.go`: modtime-keyed `wordCountCache` + `projectWordCount`
> - `bddd4bf` Task 3 — `filelist.go`: manuscript-aware sidebar (numeric order, stripped
>   titles, right-aligned per-chapter `Nw`, loose after sections, NO separator row)
> - `8db79b1` Task 4 — `backup.go`: `backupStamp` + `backupFiles` → `.backup/<stamp>/`
> - `b104314` final-review fix — `sectionTitle` strips ext BEFORE separators so
>   `01.md` → `""` not `"md"` (+2 test cases)
>
> **Review outcome:** per-task reviews + a whole-branch opus review. NO Critical /
> NO Important survived. Two per-task "Important" flags downgraded (verbatim plan
> code, no real-world impact) and confirmed by final review: (a) double
> `sectionOrder` parse in the sort comparator; (b) `sectionRow` count clip at
> `f.width < ~5` (cannot occur; pane = `sidebarWidth-2`; guards prevent panic).
> Final review verified 3 invariants: row↔index nav mapping, `.backup`/dotfile
> exclusion, cross-`SetDir` cache persistence.
>
> **Deferred Minors → fold into Plan B:**
> - `count()` runs `os.Stat` per visible section each render (read cached, stat not) — negligible, bounded by height.
> - `wordCountCache` has no eviction (session-lifetime growth) — flip side of correct cross-`SetDir` persistence.
> - `projectWordCount` has NO caller yet — intentional Plan B groundwork (sidebar uses per-section `count`, not the rollup).
> - `backupFiles` flat-by-basename silently overwrites same-basename-different-dir — by design, add a doc-comment note.
> - stale comment "or non-manuscript dir" on the `default:` case at `filelist.go:131`.
> - test gaps: narrow-width `sectionRow`; loose-after-sections in rendered `View`; `backupFiles` empty-list/error paths; `TestSectionTitle` uses a map (non-deterministic msg order); some unchecked `os.WriteFile` returns (verbatim from plan).
> - heuristic to note: a category folder containing e.g. `1984-notes.md` reads as a manuscript (matches spec's "≥1 numerically-prefixed file" definition).
>
> **NEXT (paused at user request — resume tomorrow):** write **Plan B (outline
> view)** against this merged code. Open design Qs to brainstorm first: reorder
> UX, where word counts surface, how/when backups trigger. Plan B is expected to
> add `readSections(dir)` (path-based; outline needs sections without a loaded
> `filelist`) and to be the first consumer of `projectWordCount` + `backupFiles`
> (wired into reorder/delete). Then **Plan C** (manuscript pager), **Plan D**
> (RTF+PDF export), then **Tufte preview** brainstorm, then **project rename**
> (okashi → TBD: sweeps module path, `OKASHI_*` env, workspace folder, repo, formula).
> - Build/test via `/opt/homebrew/bin/go` (not on PATH).
> - Full per-task ledger was at `.superpowers/sdd/progress.md` (git-ignored scratch — may not survive; this note is the durable record).

# Long-form Plan A — Project Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The data foundation for long-form projects — section ordering/titles, word-count rollups, a manuscript-aware sidebar, and a backup helper — that the outline (Plan B), pager (Plan C), and export (Plan D) build on.

**Architecture:** Pure helpers in a new `project.go` (ordering, titles, splitting, word-count cache); the existing `filelist` sorts and renders sections when its dir is a manuscript; a small `backup.go` for pre-destructive-op snapshots. No new screens — this plan is purely the model layer plus the sidebar upgrade.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, `charmbracelet/x/ansi`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- **Manuscript** = a folder with ≥1 numerically-prefixed `.md` file; **category** = a plain folder of loose docs; **loose** = unnumbered files (excluded from a manuscript's ordering/export).
- **Ordering:** the *leading run of digits* parsed as an integer (`1`=`01`=`001`; `2` before `10`); no leading digit = loose. Display strips the prefix; files keep their real names.
- Loose files inside a manuscript are shown after the ordered sections (no separator row — grouping is by order + styling, to keep the screen-row↔entry-index mapping 1:1 for mouse/keyboard).
- `.backup/` (and all dotfiles) are already excluded from the pane (`SetDir` skips `.`-prefixed names) and must stay excluded.
- Tests touching real dirs use `t.TempDir()`; pure-helper tests pass timestamps in (no wall-clock calls inside helpers).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Ordering & title helpers (`project.go`)

**Files:**
- Create: `project.go`, `project_test.go`

**Interfaces:**
- Produces (package `main`):
  - `func sectionOrder(name string) (n int, ok bool)`
  - `func sectionTitle(name string) string`
  - `func orderedSections(files []fileEntry) (sections, loose []fileEntry)`
  - `func isManuscript(entries []fileEntry) bool`

- [ ] **Step 1: Write the failing tests**

Create `project_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestSectionOrder(t *testing.T) {
	cases := []struct {
		name string
		n    int
		ok   bool
	}{
		{"01-opening.md", 1, true},
		{"1-opening.md", 1, true},
		{"001-opening.md", 1, true},
		{"2-x.md", 2, true},
		{"10-x.md", 10, true},
		{"notes.md", 0, false},
		{"opening.md", 0, false},
	}
	for _, c := range cases {
		n, ok := sectionOrder(c.name)
		if n != c.n || ok != c.ok {
			t.Errorf("sectionOrder(%q) = (%d,%v), want (%d,%v)", c.name, n, ok, c.n, c.ok)
		}
	}
}

func TestSectionTitle(t *testing.T) {
	cases := map[string]string{
		"02-the-letter.md": "the letter",
		"01-opening.md":    "opening",
		"10_two_words.md":  "two words",
		"notes.md":         "notes",
	}
	for in, want := range cases {
		if got := sectionTitle(in); got != want {
			t.Errorf("sectionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrderedSections(t *testing.T) {
	files := []fileEntry{
		{name: "10-ten.md"}, {name: "2-two.md"}, {name: "notes.md"},
		{name: "01-one.md"}, {name: "apple.md"},
	}
	sections, loose := orderedSections(files)
	var gs, gl []string
	for _, s := range sections {
		gs = append(gs, s.name)
	}
	for _, l := range loose {
		gl = append(gl, l.name)
	}
	if strings.Join(gs, ",") != "01-one.md,2-two.md,10-ten.md" {
		t.Fatalf("sections = %v, want numeric order 1,2,10", gs)
	}
	if strings.Join(gl, ",") != "apple.md,notes.md" {
		t.Fatalf("loose = %v, want alpha", gl)
	}
}

func TestIsManuscript(t *testing.T) {
	if !isManuscript([]fileEntry{{name: "notes.md"}, {name: "01-x.md"}}) {
		t.Fatal("a numbered file makes the folder a manuscript")
	}
	if isManuscript([]fileEntry{{name: "a.md"}, {name: "b.md"}}) {
		t.Fatal("no numbered files = not a manuscript")
	}
	if isManuscript([]fileEntry{{name: "Sub", isDir: true}}) {
		t.Fatal("a subdir alone is not a manuscript")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSectionOrder|TestSectionTitle|TestOrderedSections|TestIsManuscript' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `project.go`**

```go
package main

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sectionOrder parses the leading run of digits in name as an integer. ok is
// false when name has no leading digit (a loose file). "1", "01", "001" all
// yield 1, so sorting by n orders 2 before 10.
func sectionOrder(name string) (int, bool) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(name[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// sectionTitle is the display title for a section file: the leading digits and
// one separator stripped, the extension dropped, and -/_ turned into spaces.
// "02-the-letter.md" -> "the letter".
func sectionTitle(name string) string {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	s := strings.TrimLeft(name[i:], "-_. ")
	s = strings.TrimSuffix(s, filepath.Ext(s))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// orderedSections splits file entries (non-dir) into numbered sections (sorted
// by their numeric prefix, then name) and loose files (alphabetical).
func orderedSections(files []fileEntry) (sections, loose []fileEntry) {
	for _, f := range files {
		if _, ok := sectionOrder(f.name); ok {
			sections = append(sections, f)
		} else {
			loose = append(loose, f)
		}
	}
	sort.SliceStable(sections, func(i, j int) bool {
		ni, _ := sectionOrder(sections[i].name)
		nj, _ := sectionOrder(sections[j].name)
		if ni != nj {
			return ni < nj
		}
		return sections[i].name < sections[j].name
	})
	sort.Slice(loose, func(i, j int) bool { return loose[i].name < loose[j].name })
	return sections, loose
}

// isManuscript reports whether any non-dir entry is a numbered section.
func isManuscript(entries []fileEntry) bool {
	for _, e := range entries {
		if e.isDir {
			continue
		}
		if _, ok := sectionOrder(e.name); ok {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSectionOrder|TestSectionTitle|TestOrderedSections|TestIsManuscript' -v 2>&1 | tail
/opt/homebrew/bin/gofmt -w project.go project_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add project.go project_test.go
git commit -m "project: section ordering, titles, manuscript detection"
```

---

## Task 2: Word-count cache & rollup (`project.go`)

**Files:**
- Modify: `project.go`
- Test: `project_test.go`

**Interfaces:**
- Consumes: `wordCount` (existing, `main.go`), `fileEntry`, `orderedSections`.
- Produces:
  - `type wordCountCache struct { … }` with `func newWordCountCache() *wordCountCache` and `func (c *wordCountCache) count(path string) int`
  - `func projectWordCount(dir string, sections []fileEntry, c *wordCountCache) int`

- [ ] **Step 1: Write the failing tests**

Add to `project_test.go` (add imports `os`, `path/filepath`, `time`):

```go
func TestWordCountCacheRecountsOnChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte("one two three"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newWordCountCache()
	if got := c.count(p); got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	if err := os.WriteFile(p, []byte("one two three four five"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force a later modtime so the cache invalidates deterministically.
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	if got := c.count(p); got != 5 {
		t.Fatalf("recount = %d, want 5", got)
	}
}

func TestProjectWordCountSumsSections(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("a b c"), 0o644)   // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("d e"), 0o644)     // 2
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("x y z q"), 0o644) // loose, excluded
	sections, _ := orderedSections([]fileEntry{
		{name: "01-a.md"}, {name: "02-b.md"}, {name: "notes.md"},
	})
	c := newWordCountCache()
	if got := projectWordCount(dir, sections, c); got != 5 {
		t.Fatalf("project total = %d, want 5 (loose excluded)", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestWordCountCache|TestProjectWordCount' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement (append to `project.go`)**

Add `"os"` and `"time"` to `project.go`'s imports, then:

```go
// wordCountCache memoizes per-file word counts keyed by path + modtime so the
// sidebar and rollups don't re-read unchanged files every render.
type wordCountCache struct {
	entries map[string]wcEntry
}

type wcEntry struct {
	mod   time.Time
	words int
}

func newWordCountCache() *wordCountCache {
	return &wordCountCache{entries: map[string]wcEntry{}}
}

// count returns the word count of the file at path, reading it only when its
// modtime has changed since the last read. Missing/unreadable files count 0.
func (c *wordCountCache) count(path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if e, ok := c.entries[path]; ok && e.mod.Equal(info.ModTime()) {
		return e.words
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	w := wordCount(string(data))
	c.entries[path] = wcEntry{mod: info.ModTime(), words: w}
	return w
}

// projectWordCount sums the word counts of the ordered sections in dir.
func projectWordCount(dir string, sections []fileEntry, c *wordCountCache) int {
	total := 0
	for _, s := range sections {
		total += c.count(filepath.Join(dir, s.name))
	}
	return total
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestWordCountCache|TestProjectWordCount' -v 2>&1 | tail
/opt/homebrew/bin/gofmt -w project.go project_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add project.go project_test.go
git commit -m "project: word-count cache (modtime-keyed) and project rollup"
```

---

## Task 3: Manuscript-aware sidebar (`filelist.go`)

**Files:**
- Modify: `filelist.go` (`filelist` struct, `newFilelist`, `SetDir`, `View`)
- Test: `filelist_test.go`

**Interfaces:**
- Consumes: `sectionOrder`, `sectionTitle`, `orderedSections`, `isManuscript`, `wordCountCache`/`newWordCountCache` (Tasks 1–2); `commafy` (existing).
- Produces: `filelist` gains `wc *wordCountCache`; inside a manuscript the pane lists sections in numeric order as `title …… Nw` and loose files as filenames.

- [ ] **Step 1: Write the failing tests**

Add to `filelist_test.go`:

```go
func TestSidebarShowsTitlesAndCounts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("a b"), 0o644)
	f := newFilelist()
	f.root = ""
	f.width = 29
	f.height = 10
	f.SetDir(dir)

	view := f.View()
	if !strings.Contains(view, "opening") || strings.Contains(view, "01-opening") {
		t.Fatalf("manuscript pane should show stripped title 'opening', not raw filename:\n%s", view)
	}
	if !strings.Contains(view, "3w") {
		t.Fatalf("manuscript pane should show the section word count '3w':\n%s", view)
	}
}

func TestSidebarOrdersSectionsNumerically(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "10-ten.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "2-two.md"), []byte("x"), 0o644)
	f := newFilelist()
	f.root = ""
	f.width = 29
	f.height = 10
	f.SetDir(dir)

	var names []string
	for _, e := range f.entries {
		if !e.isDir {
			names = append(names, e.name)
		}
	}
	if strings.Join(names, ",") != "2-two.md,10-ten.md" {
		t.Fatalf("sections should sort numerically (2 before 10): %v", names)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSidebarShowsTitlesAndCounts|TestSidebarOrdersSectionsNumerically' 2>&1 | tail`
Expected: FAIL — `newFilelist` has no `wc`/counts behavior yet; titles/counts/order not rendered.

- [ ] **Step 3: Add the `wc` field and init**

In `filelist.go`, add to the `filelist` struct (after `icons iconSet`):

```go
	wc *wordCountCache
```

In `newFilelist`, set it on the returned struct literal:

```go
	wc: newWordCountCache(),
```

- [ ] **Step 4: Sort files in section order in `SetDir`**

In `SetDir`, replace the file sort + append:

```go
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	f.entries = append(f.entries, dirs...)
	f.entries = append(f.entries, files...)
```

with (dirs first, then ordered sections, then loose):

```go
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sections, loose := orderedSections(files)
	f.entries = append(f.entries, dirs...)
	f.entries = append(f.entries, sections...)
	f.entries = append(f.entries, loose...)
```

- [ ] **Step 5: Render titles + counts in `View`**

Add a row helper (after `View`):

```go
// sectionRow builds a width-f.width row for a manuscript section: gutter+icon+
// title on the left, the word count right-aligned. dimCount styles the count
// subtle (used for non-selected rows; the selected bar keeps it plain).
func (f filelist) sectionRow(e fileEntry, dimCount bool) string {
	n := 0
	if f.wc != nil {
		n = f.wc.count(filepath.Join(f.dir, e.name))
	}
	count := commafy(n) + "w"
	left := " " + f.icons.icon(e) + sectionTitle(e.name)
	maxLeft := f.width - lipgloss.Width(count) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = ansi.Truncate(left, maxLeft, "…")
	gap := f.width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 1 {
		gap = 1
	}
	rendered := count
	if dimCount {
		rendered = lipgloss.NewStyle().Foreground(subtle).Render(count)
	}
	return left + strings.Repeat(" ", gap) + rendered
}
```

Then change the `View` per-entry loop. Replace the existing loop body:

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

with (a `manuscript` flag computed once before the loop, and a `section` check per row):

```go
		e := f.entries[i]
		head := " " + f.icons.icon(e) // one-column gutter, then the icon
		full := head + e.name
		_, ord := sectionOrder(e.name)
		section := manuscript && !e.isDir && ord
		switch {
		case i == f.selected:
			content := full
			if section {
				content = f.sectionRow(e, false)
			}
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(content, f.width, "…")))
		case e.isDir:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(full, f.width, "…")))
		case section:
			b.WriteString(f.sectionRow(e, true))
		default:
			// Loose file (or non-manuscript dir): filename with dim extension.
			ext := filepath.Ext(e.name)
			if ext != "" && lipgloss.Width(full) <= f.width {
				stem := head + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(full, f.width, "…"))
			}
		}
```

Add `manuscript := isManuscript(f.entries)` just before the `for i := f.offset; …` loop in `View`.

- [ ] **Step 6: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebar|TestFilelist' -v 2>&1 | tail -12
/opt/homebrew/bin/gofmt -w filelist.go filelist_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add filelist.go filelist_test.go
git commit -m "Sidebar: ordered section titles + per-chapter word counts in manuscripts"
```

---

## Task 4: Backup helper (`backup.go`)

**Files:**
- Create: `backup.go`, `backup_test.go`

**Interfaces:**
- Produces:
  - `func backupStamp(t time.Time) string` — filesystem-safe timestamp
  - `func backupFiles(projectDir, stamp string, paths []string) error` — copies paths into `<projectDir>/.backup/<stamp>/`

- [ ] **Step 1: Write the failing tests**

Create `backup_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupStampSafe(t *testing.T) {
	s := backupStamp(time.Date(2026, 6, 22, 14, 3, 5, 0, time.UTC))
	if s != "2026-06-22T14-03-05" {
		t.Fatalf("stamp = %q, want 2026-06-22T14-03-05", s)
	}
	if strings.ContainsAny(s, ":/ ") {
		t.Fatalf("stamp has filesystem-unsafe chars: %q", s)
	}
}

func TestBackupFilesCopies(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "01-a.md")
	if err := os.WriteFile(a, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := backupStamp(time.Date(2026, 6, 22, 14, 3, 0, 0, time.UTC))
	if err := backupFiles(dir, stamp, []string{a}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".backup", stamp, "01-a.md"))
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("backup content = %q, want hello", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestBackupStampSafe|TestBackupFilesCopies' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `backup.go`**

```go
package main

import (
	"os"
	"path/filepath"
	"time"
)

// backupStamp formats t as a filesystem-safe directory name (no colons/slashes).
func backupStamp(t time.Time) string {
	return t.Format("2006-01-02T15-04-05")
}

// backupFiles copies each path into <projectDir>/.backup/<stamp>/ (flat, by base
// name). Used to snapshot files before a destructive op (reorder/delete).
func backupFiles(projectDir, stamp string, paths []string) error {
	dest := filepath.Join(projectDir, ".backup", stamp)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dest, filepath.Base(p)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the tests; full suite; build; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestBackup' -v 2>&1 | tail
/opt/homebrew/bin/gofmt -w backup.go backup_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add backup.go backup_test.go
git commit -m "Add backup helper: snapshot files into .backup/<timestamp>/"
```

---

## Self-Review

**Spec coverage (Plan A = spec §§1, 2, 3, 7):**
- §1 ordering/title/detection → Task 1 (`sectionOrder`/`sectionTitle`/`orderedSections`/`isManuscript`); `SetDir` sorts in section order → Task 3.
- §2 word-count rollup + cache → Task 2 (`wordCountCache`, `projectWordCount`); surfaced per-chapter in the sidebar → Task 3.
- §3 enhanced sidebar (titles + counts, loose grouped by order/styling, non-manuscript unchanged) → Task 3.
- §7 backup helper (`.backup/<timestamp>/`, pre-destructive) → Task 4. (Wiring into reorder/delete + on-demand key is Plan B, which performs the destructive ops.)

**Deviation from spec wording:** the spec sketched `isManuscript(dir string)`; this plan implements `isManuscript(entries []fileEntry)` because the sidebar already holds loaded entries — avoids a redundant directory read. A path-based reader (`readSections(dir)`) is deferred to Plan B, where the outline needs sections without a loaded `filelist`.

**Placeholder scan:** none — full code in every step.

**Type consistency:** `fileEntry{name,isDir}` (existing) used throughout; `wordCountCache`/`newWordCountCache()*`/`count`/`projectWordCount(dir,sections,*cache)` consistent across Tasks 2–3; `sectionOrder`/`sectionTitle`/`orderedSections`/`isManuscript` consistent across Tasks 1, 3; `commafy` reused from `main.go`. The sidebar adds **no separator row** (loose files follow sections via ordering + styling), preserving the existing screen-row↔entry-index mapping that mouse/keyboard nav rely on — so Plan B/C hit-testing stays valid.

**Non-manuscript safety:** in a folder with no numbered files, `isManuscript` is false → `View` renders exactly as today (existing `TestFilelistGutterAndDimExtension` etc. use unprefixed names and stay green); `SetDir`'s new sort puts all files in the `loose` (alphabetical) group, matching the previous alphabetical order.
