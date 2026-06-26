# Rename + Convert-to-manuscript Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** a recurring generation bug emits a stray
> `court` token and/or drops the `antml:` namespace, silently no-op'ing tool calls.
> Mitigation: one tool call per message, as the FIRST element of the reply, explanation
> AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Add `r` to rename the selected loose file, folder, or section title, and fold a "convert folder → manuscript" step into `ctrl+l` so a plain folder of chapter files gains numbering + the outline.

**Architecture:** A small `rename.go` with three pure, unit-tested helpers (`sectionRetitle`, `looseRename`, `planConvert`); the rest is wiring in `main.go` — a shared rename prompt (`renaming` mode + `renameTarget`, reusing `nameInput`/`confirmRename`) hooked from both the sidebar and the outline, and a `convertPrompt` y/n confirm hooked from `ctrl+l`. Reuses `slugify`, `splitPrefix`, `padWidth`, `applyRenames`, `backupFiles`/`backupStamp`, and the open-file-tracking pattern.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, `charmbracelet/bubbles/textinput`.

**Design spec:** `docs/superpowers/specs/2026-06-26-rename-and-convert-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt` (not on PATH).
- **Rename "what you see":** numbered section → title only (keep `NN-` prefix + extension, `slugify` the new title); loose file → filename (keep the original extension if the typed name omits one); folder → directory name.
- **Collision-safe:** if the rename target already exists, refuse with a status message — never overwrite. Reject a typed name containing `/`, `.`, or `..` (loose file / folder only; a section title is slugified so it can't contain them).
- **Open-file tracking:** if the renamed/converted file is the one open in the editor, `m.currentFile` follows to the new path.
- A single rename is one atomic `os.Rename` (no backup — reversible). **Convert** renames many files, so it snapshots all the folder's doc files to `.backup/<stamp>/` first, via the existing two-phase `applyRenames`.
- **Convert** numbers a plain folder's doc files contiguously in current (alphabetical) order, keeping each existing name as the title: `Chapter-00.md` → `01-Chapter-00.md`. Triggered by `ctrl+l` when the folder is not a manuscript but has ≥1 doc file.
- Reuse existing helpers; do not reimplement: `slugify`, `splitPrefix`, `padWidth`, `applyRenames`, `backupFiles`, `backupStamp`, `sectionTitle`, `sectionOrder`, `isManuscript`, `allowedDocExts`.
- Tests hermetic: `t.TempDir()` / `t.Setenv("OKASHI_DIR", …)`; pure helpers take values in.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Pure rename/convert helpers (`rename.go`)

**Files:**
- Create: `rename.go`, `rename_test.go`

**Interfaces:**
- Consumes: `fileEntry` (filelist.go), `splitPrefix`/`slugify`/`renameOp` (outline.go).
- Produces (package `main`):
  - `func sectionRetitle(name, newTitle string) string`
  - `func looseRename(oldName, typed string) string`
  - `func planConvert(files []fileEntry, width int) []renameOp`

- [ ] **Step 1: Write the failing tests**

Create `rename_test.go`:

```go
package main

import "testing"

func TestSectionRetitleKeepsPrefixAndExt(t *testing.T) {
	cases := []struct{ name, title, want string }{
		{"02-the-letter.md", "the telegram", "02-the-telegram.md"},
		{"01-opening.md", "A New Dawn", "01-a-new-dawn.md"},
		{"003-x.md", "scene two", "003-scene-two.md"},
	}
	for _, c := range cases {
		if got := sectionRetitle(c.name, c.title); got != c.want {
			t.Errorf("sectionRetitle(%q,%q) = %q, want %q", c.name, c.title, got, c.want)
		}
	}
}

func TestLooseRenameKeepsExtensionWhenOmitted(t *testing.T) {
	if got := looseRename("draft.md", "notes"); got != "notes.md" {
		t.Errorf("looseRename kept ext: got %q, want notes.md", got)
	}
	if got := looseRename("draft.md", "outline.txt"); got != "outline.txt" {
		t.Errorf("looseRename with explicit ext: got %q, want outline.txt", got)
	}
	if got := looseRename("README", "NOTES"); got != "NOTES" {
		t.Errorf("looseRename no original ext: got %q, want NOTES", got)
	}
}

func TestPlanConvertNumbersAndKeepsNames(t *testing.T) {
	files := []fileEntry{{name: "Chapter-00.md"}, {name: "Chapter-01.md"}}
	ops := planConvert(files, 2)
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["Chapter-00.md"] != "01-Chapter-00.md" || got["Chapter-01.md"] != "02-Chapter-01.md" {
		t.Fatalf("planConvert = %v, want contiguous NN- prefixes keeping the name", got)
	}
	// The result must read as a manuscript and de-slug back to the original name.
	if _, ok := sectionOrder("01-Chapter-00.md"); !ok {
		t.Fatal("converted name should parse as a numbered section")
	}
	if title := sectionTitle("01-Chapter-00.md"); title != "Chapter 00" {
		t.Fatalf("sectionTitle of converted name = %q, want 'Chapter 00'", title)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSectionRetitle|TestLooseRename|TestPlanConvert' 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `rename.go`**

```go
package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// sectionRetitle renames a numbered section to a new title, keeping its numeric
// prefix and extension and slugifying the title. "02-the-letter.md" + "the
// telegram" -> "02-the-telegram.md".
func sectionRetitle(name, newTitle string) string {
	digits, _ := splitPrefix(name)
	ext := filepath.Ext(name)
	return digits + "-" + slugify(newTitle) + ext
}

// looseRename is the new base name for a loose file: the typed name, with the
// original extension restored when the typed name omits one. "draft.md" +
// "notes" -> "notes.md"; "draft.md" + "outline.txt" -> "outline.txt".
func looseRename(oldName, typed string) string {
	if filepath.Ext(typed) == "" {
		typed += filepath.Ext(oldName)
	}
	return typed
}

// planConvert numbers a plain folder's files contiguously, keeping each existing
// name as the title portion: "Chapter-00.md" -> "01-Chapter-00.md". Every file
// gains a prefix, so there are no no-ops.
func planConvert(files []fileEntry, width int) []renameOp {
	ops := make([]renameOp, 0, len(files))
	for i, e := range files {
		ops = append(ops, renameOp{from: e.name, to: fmt.Sprintf("%0*d-%s", width, i+1, e.name)})
	}
	return ops
}
```

- [ ] **Step 4: Run the tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSectionRetitle|TestLooseRename|TestPlanConvert' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w rename.go rename_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add rename.go rename_test.go
git commit -m "rename: pure section-retitle, loose-rename, and convert planners"
```

---

## Task 2: Rename infrastructure + sidebar rename (`main.go`)

**Files:**
- Modify: `main.go` (`model` struct, `Update` prompt-capture, sidebar key branch, `statusBar`)
- Test: `rename_wiring_test.go` (create)

**Interfaces:**
- Consumes: `sectionRetitle`, `looseRename` (Task 1); `sectionOrder`, `sectionTitle`, `isManuscript`, `m.files` (`entries`/`selected`/`dir`/`SetDir`/`selectName`), `nameInput`, `focusSidebar`.
- Produces: `type renameTarget struct { dir, name string; isDir, section bool }`; `model.renaming bool`; `model.renameTarget renameTarget`; `func (m *model) beginRename(t renameTarget, prefill string)`; `func (m *model) startRename()` (sidebar); `func (m *model) confirmRename()`; `func (m *model) refreshAfterRename()`. The `r` key renames the selected sidebar entry.

- [ ] **Step 1: Write the failing tests**

Create `rename_wiring_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeInto sends each rune of s to the model as a key message.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = nm.(model)
	}
	return m
}

func sidebarModel(t *testing.T, dir string) model {
	t.Helper()
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.files.SetDir(dir)
	m.focus = focusSidebar
	return m
}

func TestSidebarRenameLooseFileKeepsExt(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("hi"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "notes")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "notes.md")); err != nil {
		t.Fatalf("expected renamed notes.md (ext kept): %v", err)
	}
}

func TestSidebarRenameSectionTitleOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "02-the-letter.md"), []byte("x"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("02-the-letter.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the telegram")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "02-the-telegram.md")); err != nil {
		t.Fatalf("section rename should keep the 02- prefix: %v", err)
	}
}

func TestSidebarRenameFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.MkdirAll(filepath.Join(root, "oldname"), 0o755)
	m := sidebarModel(t, root)
	m.files.selectName("oldname")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "newname")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(root, "newname")); err != nil {
		t.Fatalf("folder rename should rename the directory: %v", err)
	}
}

func TestSidebarRenameRefusesCollision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "b.md"), []byte("y"), 0o644)
	m := sidebarModel(t, root)
	m.files.selectName("a.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "b")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	// Both originals must still exist — no overwrite.
	if b, _ := os.ReadFile(filepath.Join(root, "b.md")); string(b) != "y" {
		t.Fatal("rename onto an existing name must not overwrite it")
	}
	if _, err := os.Stat(filepath.Join(root, "a.md")); err != nil {
		t.Fatal("the source must be left intact when the rename is refused")
	}
}

func TestSidebarRenameTracksOpenFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	os.WriteFile(filepath.Join(root, "draft.md"), []byte("x"), 0o644)
	m := sidebarModel(t, root)
	m.currentFile = filepath.Join(root, "draft.md")
	m.files.selectName("draft.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	m.nameInput.SetValue("")
	m = typeInto(t, m, "final.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.currentFile != filepath.Join(root, "final.md") {
		t.Fatalf("open file path should follow the rename, got %q", m.currentFile)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSidebarRename' 2>&1 | tail`
Expected: build error / FAIL — `renaming` and the `r` handler don't exist.

- [ ] **Step 3: Add the struct fields and the `renameTarget` type**

In `main.go`, add to the `model` struct (after the `outline outlineModel` / `outlineCreating bool` fields):

```go
	renaming     bool
	renameTarget renameTarget
```

Add the type (near the `model` struct definition):

```go
// renameTarget is the item a pending rename prompt will rename.
type renameTarget struct {
	dir     string // directory containing the item
	name    string // current base name
	isDir   bool
	section bool // a numbered section -> title-only rename
}
```

- [ ] **Step 4: Add the rename helpers**

Add to `main.go` (near `confirmCreate`):

```go
// beginRename opens the rename prompt for t, pre-filled with prefill.
func (m *model) beginRename(t renameTarget, prefill string) {
	m.renameTarget = t
	m.renaming = true
	m.creatingFile = false
	m.nameInput.SetValue(prefill)
	m.nameInput.CursorEnd()
	m.nameInput.Focus()
	m.editor.Blur()
	m.status = ""
}

// startRename begins renaming the selected sidebar entry (skips the ".." row).
func (m *model) startRename() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	_, numbered := sectionOrder(e.name)
	section := numbered && !e.isDir && isManuscript(m.files.entries)
	prefill := e.name
	if section {
		prefill = sectionTitle(e.name)
	}
	m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: section}, prefill)
}

// confirmRename applies the pending rename: builds the new name by target kind,
// refuses a collision, renames on disk, follows the open file, and refreshes.
func (m *model) confirmRename() {
	m.renaming = false
	m.nameInput.Blur()
	typed := strings.TrimSpace(m.nameInput.Value())
	t := m.renameTarget
	if typed == "" {
		m.status = "rename cancelled (empty)"
		m.refreshAfterRename()
		return
	}

	var newName string
	if t.section {
		newName = sectionRetitle(t.name, typed)
	} else {
		if strings.Contains(typed, "/") || typed == "." || typed == ".." {
			m.status = "name can't contain a path separator"
			m.refreshAfterRename()
			return
		}
		if t.isDir {
			newName = typed
		} else {
			newName = looseRename(t.name, typed)
		}
	}
	if newName == t.name {
		m.status = "unchanged"
		m.refreshAfterRename()
		return
	}

	oldPath := filepath.Join(t.dir, t.name)
	newPath := filepath.Join(t.dir, newName)
	if _, err := os.Stat(newPath); err == nil {
		m.status = "a file named " + newName + " already exists"
		m.refreshAfterRename()
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		m.status = "rename failed: " + err.Error()
		m.refreshAfterRename()
		return
	}
	if m.currentFile == oldPath {
		m.currentFile = newPath
	}
	m.refreshAfterRename()
	m.status = "renamed to " + newName
}

// refreshAfterRename re-reads the sidebar (and the outline, if active) and
// restores focus to the pane the rename came from.
func (m *model) refreshAfterRename() {
	m.files.SetDir(m.files.dir)
	if m.screen == screenOutline {
		m.outline.load(m.outline.dir, m.files.wc)
		return
	}
	m.focus = focusSidebar
	m.editor.Blur()
}
```

- [ ] **Step 5: Route prompt input and the `r` key**

In `Update`, add a rename prompt-capture block right after the existing `if m.creatingFile { … }` block:

```go
	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.renaming = false
				m.nameInput.Blur()
				m.status = "rename cancelled"
				m.refreshAfterRename()
				return m, nil
			case "enter":
				m.confirmRename()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
```

In the sidebar key branch (`} else if m.focus == focusSidebar && m.sidebarVisible {`), add a case to its switch:

```go
			case "r":
				m.startRename()
```

- [ ] **Step 6: Render the rename prompt in `statusBar`**

In `statusBar`, add right after the `if m.creatingFile { … }` block:

```go
	if m.renaming {
		return "rename ▸ " + m.nameInput.View()
	}
```

- [ ] **Step 7: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebarRename' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go rename_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go rename_wiring_test.go
git commit -m "rename: r renames the selected sidebar file/folder/section (collision-safe, open-file tracking)"
```

---

## Task 3: Rename in the outline (`main.go`)

**Files:**
- Modify: `main.go` (`updateOutline`)
- Test: `rename_wiring_test.go`

**Interfaces:**
- Consumes: `beginRename`/`confirmRename`/`m.renaming` (Task 2); `m.outline` (`selectedRow`/`dir`), `sectionTitle`.
- Produces: `func (m *model) startRenameOutline()`; the `r` key in the outline renames the selected section title (or loose row); the outline routes prompt input while `m.renaming`.

- [ ] **Step 1: Write the failing test**

Add to `rename_wiring_test.go`:

```go
func TestOutlineRenameSectionTitle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, "02-the-letter.md"), []byte("x"), 0o644)
	m := sidebarModel(t, proj)
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) // enter the outline
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // select 02-the-letter
	m = nm.(model)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r in the outline should start a rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the telegram")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "02-the-telegram.md")); err != nil {
		t.Fatalf("outline rename should retitle keeping the prefix: %v", err)
	}
	if m.screen != screenOutline {
		t.Fatalf("after an outline rename we should still be in the outline, got %v", m.screen)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineRenameSectionTitle' 2>&1 | tail`
Expected: FAIL — the outline has no `r` handler / prompt routing yet.

- [ ] **Step 3: Add `startRenameOutline`**

Add to `main.go`:

```go
// startRenameOutline begins renaming the selected outline row (section title or
// loose file).
func (m *model) startRenameOutline() {
	row, ok := m.outline.selectedRow()
	if !ok {
		return
	}
	prefill := row.entry.name
	if row.isSection {
		prefill = sectionTitle(row.entry.name)
	}
	m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, isDir: false, section: row.isSection}, prefill)
}
```

- [ ] **Step 4: Route prompt input and the `r` key in `updateOutline`**

In `updateOutline`, add a rename prompt-capture block right after the `if m.outlineCreating { … }` block (and before the mouse / confirm-gate handling):

```go
	if m.renaming {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc":
				m.renaming = false
				m.nameInput.Blur()
				m.status = "rename cancelled"
				return m, nil
			case "enter":
				m.confirmRename()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
```

In `updateOutline`'s main key switch (alongside `n`, `m`, `esc`), add:

```go
	case "r":
		m.startRenameOutline()
```

- [ ] **Step 5: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineRenameSectionTitle|TestSidebarRename' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go rename_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go rename_wiring_test.go
git commit -m "rename: r retitles the selected section from the outline"
```

---

## Task 4: Convert folder → manuscript via `ctrl+l` (`main.go`)

**Files:**
- Modify: `main.go` (`ctrl+l` case, `Update` prompt-capture, `statusBar`, new helpers)
- Test: `rename_wiring_test.go`

**Interfaces:**
- Consumes: `planConvert` (Task 1); `padWidth`, `applyRenames`, `backupFiles`, `backupStamp`, `enterOutline`, `m.files`.
- Produces: `model.convertPrompt bool`; `func (m model) hasConvertibleFiles() bool`; `func (m *model) convertToManuscript()`. `ctrl+l` on a plain folder with files raises a y/n convert prompt.

- [ ] **Step 1: Write the failing tests**

Add to `rename_wiring_test.go`:

```go
func TestConvertPromptOnPlainFolder(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	book := filepath.Join(root, "book")
	os.MkdirAll(book, 0o755)
	os.WriteFile(filepath.Join(book, "Chapter-00.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(book, "Chapter-01.md"), []byte("y"), 0o644)
	m := sidebarModel(t, book)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if !m.convertPrompt {
		t.Fatal("ctrl+l on a plain folder with files should raise the convert prompt")
	}
	if m.screen == screenOutline {
		t.Fatal("must not enter the outline before the user confirms")
	}
}

func TestConvertNumbersFilesAndOpensOutline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	book := filepath.Join(root, "book")
	os.MkdirAll(book, 0o755)
	os.WriteFile(filepath.Join(book, "Chapter-00.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(book, "Chapter-01.md"), []byte("y"), 0o644)
	m := sidebarModel(t, book)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(book, "01-Chapter-00.md")); err != nil {
		t.Fatalf("convert should number the first file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(book, "02-Chapter-01.md")); err != nil {
		t.Fatalf("convert should number the second file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(book, ".backup")); err != nil {
		t.Fatalf("convert should snapshot to .backup/ first: %v", err)
	}
	if m.screen != screenOutline {
		t.Fatalf("convert should open the outline, got screen %v", m.screen)
	}
}

func TestCtrlLNoDocsShowsNothingToConvert(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	empty := filepath.Join(root, "empty")
	os.MkdirAll(empty, 0o755)
	m := sidebarModel(t, empty)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.convertPrompt || m.screen == screenOutline {
		t.Fatal("ctrl+l on a folder with no documents should neither prompt nor enter the outline")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestConvert|TestCtrlLNoDocs' 2>&1 | tail`
Expected: FAIL — `convertPrompt` and the convert handler don't exist.

- [ ] **Step 3: Add the struct field and convert helpers**

Add to the `model` struct (after `renameTarget renameTarget`):

```go
	convertPrompt bool
```

Add to `main.go`:

```go
// hasConvertibleFiles reports whether the current pane dir has at least one
// document file (non-dir entry) that a convert could number.
func (m model) hasConvertibleFiles() bool {
	for _, e := range m.files.entries {
		if !e.isDir {
			return true
		}
	}
	return false
}

// convertToManuscript numbers the current folder's document files contiguously
// (backup first), follows the open file, and opens the outline.
func (m *model) convertToManuscript() {
	dir := m.files.dir
	var files []fileEntry
	for _, e := range m.files.entries {
		if !e.isDir {
			files = append(files, e)
		}
	}
	if len(files) == 0 {
		m.status = "nothing to convert"
		return
	}
	ops := planConvert(files, padWidth(len(files), 0))
	var paths []string
	for _, f := range files {
		paths = append(paths, filepath.Join(dir, f.name))
	}
	if err := backupFiles(dir, backupStamp(time.Now()), paths); err != nil {
		m.status = "convert failed: " + err.Error()
		return
	}
	if err := applyRenames(dir, ops); err != nil {
		m.status = "convert failed: " + err.Error()
		return
	}
	for _, op := range ops {
		if m.currentFile == filepath.Join(dir, op.from) {
			m.currentFile = filepath.Join(dir, op.to)
		}
	}
	m.files.SetDir(dir)
	m.enterOutline()
	m.status = "converted to manuscript"
}
```

- [ ] **Step 4: Extend `ctrl+l` and route the convert prompt**

In the editor key switch, replace the existing `ctrl+l` case:

```go
		case "ctrl+l":
			if isManuscript(m.files.entries) {
				m.enterOutline()
			} else {
				m.status = "not a manuscript folder (no numbered sections)"
			}
			return m, nil
```

with:

```go
		case "ctrl+l":
			switch {
			case isManuscript(m.files.entries):
				m.enterOutline()
			case m.hasConvertibleFiles():
				m.convertPrompt = true
				m.status = "make this a manuscript? (y / n)"
			default:
				m.status = "nothing to convert (no documents here)"
			}
			return m, nil
```

Add a convert prompt-capture block right after the `if m.renaming { … }` block in `Update`:

```go
	if m.convertPrompt {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "y":
				m.convertPrompt = false
				m.convertToManuscript()
				return m, nil
			case "n", "esc":
				m.convertPrompt = false
				m.status = "convert cancelled"
				return m, nil
			}
		}
		return m, nil
	}
```

- [ ] **Step 5: Render the convert prompt in `statusBar`**

In `statusBar`, add right after the `if m.renaming { … }` block:

```go
	if m.convertPrompt {
		return "make this a manuscript? (y / n)"
	}
```

- [ ] **Step 6: Run the tests; full suite; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestConvert|TestCtrlLNoDocs' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go rename_wiring_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go rename_wiring_test.go
git commit -m "convert: ctrl+l offers to number a plain folder into a manuscript (backup first)"
```

---

## Task 5: Docs & status line

**Files:**
- Modify: `main.go` (editor status string), `README.md`

**Interfaces:** none (docs + a status-hint string).

- [ ] **Step 1: Add `r` to the editor status hint**

In `initialModel`, the `status:` field currently reads
`"ctrl+b sidebar · esc switch · ctrl+n new · ctrl+l outline · ctrl+p preview · ctrl+t typewriter · ctrl+d dim · ctrl+s save · ctrl+c quit"`.
Insert the rename hint after `ctrl+n new`:

```go
		status:         "ctrl+b sidebar · esc switch · ctrl+n new · r rename · ctrl+l outline · ctrl+p preview · ctrl+t typewriter · ctrl+d dim · ctrl+s save · ctrl+c quit",
```

- [ ] **Step 2: Document rename + convert in `README.md`**

Read `README.md` first to match its heading style. Add to the keys/sidebar area and the outline section:

```markdown
### Rename & convert

- In the sidebar (or outline), press **r** to rename the selected item — a loose
  file (keeps its extension), a folder, or a section *title* (the `NN-` number is
  kept; only the title changes). Renames refuse to overwrite an existing name.
- Press **ctrl+l** in a plain folder of chapter files (e.g. `Chapter-00.md`,
  `Chapter-01.md`) and okashi offers to **make it a manuscript** — it numbers the
  files (`01-Chapter-00.md`, …) after a `.backup/` snapshot and opens the outline,
  where you can reorder and retitle.
```

- [ ] **Step 3: Verify build + full suite; commit**

```bash
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go README.md
git commit -m "docs: r rename + ctrl+l convert-to-manuscript keymap"
```

---

## Self-Review

**Spec coverage:**
- §1 Rename (`r`, context-sensitive: section title-only / loose filename / folder; collision-safe; open-file tracking; no backup) → Task 1 (`sectionRetitle`/`looseRename`), Task 2 (sidebar wiring + `confirmRename`), Task 3 (outline wiring).
- §2 Convert folder → manuscript (`ctrl+l` branch, y/n prompt, `planConvert`, backup-first, open the outline, open-file tracking) → Task 1 (`planConvert`), Task 4 (wiring).
- §3 Keys & prompt routing (`r`, `ctrl+l`, reuse `nameInput` with a `renaming` mode + `convertPrompt`; status hints) → Tasks 2–4 (routing + `statusBar`), Task 5 (status string + README).

**Placeholder scan:** none — full code in every step.

**Type consistency:** `renameTarget{dir,name,isDir,section}` defined in Task 2, consumed by `startRename`/`startRenameOutline`/`confirmRename` (Tasks 2–3); `m.renaming`/`m.renameTarget` (Task 2) reused by the outline (Task 3) — one shared mode, captured in both the writing path and `updateOutline`; `m.convertPrompt` (Task 4); helpers `sectionRetitle`/`looseRename`/`planConvert` (Task 1) used in Tasks 2–4. `confirmRename` branches on `m.screen` so the same method serves the sidebar and the outline. `beginRename` is the single prompt-opening path for both entry points.

**Cross-cutting checks baked into tests:** collision refusal leaves both files intact (Task 2); open-file path follows a rename (Task 2) and a convert (implicit via the same pattern); section rename keeps the prefix from both the sidebar (Task 2) and the outline (Task 3); convert snapshots to `.backup/` before numbering and opens the outline (Task 4); `ctrl+l` on a no-doc folder neither prompts nor enters (Task 4).

**Note for the executor:** Tasks 2–4 each add one prompt-capture block to `Update` (rename, then convert) — they must sit after the existing `if m.creatingFile` block and return early, so nav/editor keys never leak into a prompt. The `r` key is handled in the sidebar focus branch (Task 2) and in `updateOutline`'s key switch (Task 3); it must NOT be added to the main editor key switch (where `r` is ordinary text input).
