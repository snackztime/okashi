# Desktop File-Pane Feature Set Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** Turn the file pane into a desktop-style manager — in-place rename, in-place create with a clickable `+`, delete (confirmed), and duplicate.

**Architecture:** Rename and create reuse the existing `m.nameInput`/`confirmRename`/`confirmCreate` but render the field IN the file list (a row or a new top row) instead of the bottom bar. Triggers (right-click, F2, the `+`, Delete, d) call those existing flows. No floating menus.

**Tech Stack:** Go, Bubble Tea, lipgloss, the existing `filelist`/`framedPanel`/rename+create engine.

**Design spec:** `docs/superpowers/specs/2026-06-29-desktop-file-pane-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Reuse `startRename`/`confirmRename`/`confirmCreate`/`sidebarRow`/`m.files.selectRow` — do NOT reimplement validation; manifest chapters stay blocked.
- In-place edits suppress the bottom-bar prompt; outline rename keeps the bottom bar.
- Delete is permanent (with a confirm); manifest chapters / `..` / `manifest.json` refuse.
- Panel click geometry must stay aligned (the `+` is on the top-border row; file rows below) — render-based check.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: In-place input render (`filelist.go`, `main.go`)

**Files:** Modify `filelist.go`, `main.go`; Test `filelist_test.go`, `smoke_test.go`

**Interfaces:**
- Produces: `filelist.View(editRow int, editField string)` — `editRow >= 0` renders `editField` at that row; `editRow == -1` is normal; `editRow == createRowSentinel (-2)` renders `editField` as a new row at the top (after `..`). `m.renamingInPane bool`, `m.creatingInPane bool`.

- [ ] **Step 1: Write the failing test** — add to `filelist_test.go`:

```go
func TestFileListInlineEditRow(t *testing.T) {
	f := filelist{width: 20, height: 10, icons: plainIcons()}
	f.entries = []fileEntry{{name: "alpha.md"}, {name: "bravo.md"}}
	f.selected = 1
	out := ansi.Strip(f.View(1, "EDITING_HERE"))
	if !strings.Contains(out, "EDITING_HERE") {
		t.Fatalf("editRow should render the field:\n%s", out)
	}
	if strings.Contains(out, "bravo.md") {
		t.Fatalf("the edited row should show the field, not the filename:\n%s", out)
	}
	// Normal render (editRow -1) is unchanged.
	if !strings.Contains(ansi.Strip(f.View(-1, "")), "bravo.md") {
		t.Fatal("normal render should show filenames")
	}
}

func TestFileListInlineCreateRow(t *testing.T) {
	f := filelist{width: 20, height: 10, icons: plainIcons()}
	f.entries = []fileEntry{{name: "alpha.md"}}
	out := ansi.Strip(f.View(createRowSentinel, "NEWFILE"))
	if !strings.Contains(out, "NEWFILE") || !strings.Contains(out, "alpha.md") {
		t.Fatalf("create row should add the field AND keep existing entries:\n%s", out)
	}
}
```

(If `plainIcons()` isn't a helper, construct `f.icons` the way other filelist tests do — read `filelist_test.go`.)

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run 'TestFileListInline' 2>&1 | tail` → `View` arity / `createRowSentinel` undefined.

- [ ] **Step 3: Change `filelist.View`** — add `const createRowSentinel = -2` and the params:

```go
func (f filelist) View(editRow int, editField string) string {
	if len(f.entries) == 0 && editRow != createRowSentinel {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty)")
	}
	editRowStyle := lipgloss.NewStyle().Foreground(accent).Width(f.width)
	var b strings.Builder
	if editRow == createRowSentinel {
		b.WriteString(editRowStyle.Render(ansi.Truncate(" "+editField, f.width, "")) + "\n")
	}
	// ... the existing loop, but at the top of the loop body:
	//   if i == editRow && editRow >= 0 {
	//       b.WriteString(editRowStyle.Render(ansi.Truncate(" "+editField, f.width, "")))
	//       continue   // (don't forget the existing newline handling between rows)
	//   }
	// keep the rest of the existing row rendering unchanged.
}
```

(Apply the `editRow`/`createRowSentinel` checks inside the existing loop, preserving the existing inter-row `\n` behavior. The create row is prepended before the loop.)

- [ ] **Step 4: Wire the model** — add `renamingInPane bool` and `creatingInPane bool` to `model`. In `startRename`, set `m.renamingInPane = true` and `m.nameInput.Width = m.files.width` before `beginRename` (and `beginRename`/cancel paths clear it — set `m.renamingInPane = false` in `confirmRename` and the esc-cancel). In `startRenameOutline`, leave `renamingInPane = false`.

  Update the `m.files.View()` call site (main.go ~1047):
  ```go
  editRow, editField := -1, ""
  if m.renaming && m.renamingInPane {
      editRow, editField = m.files.selected, m.nameInput.View()
  } else if m.creatingFile && m.creatingInPane {
      editRow, editField = createRowSentinel, m.nameInput.View()
  }
  cols = append(cols, framedPanel(title, m.files.View(editRow, editField), sidebarWidth, m.height))
  ```

  In `statusBar`, change the rename/create branches to only fire for non-in-pane edits:
  `if m.renaming && !m.renamingInPane { return "rename ▸ " + m.nameInput.View() }` and the
  `m.creatingFile` branch → `if m.creatingFile && !m.creatingInPane { … }`.

- [ ] **Step 5: Add a smoke test** — `r` rename now renders in the row, not the bar:

```go
func TestRenameRendersInRow(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.focus = focusSidebar
	m.files.selectName("01-a.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming || !m.renamingInPane {
		t.Fatal("r should start an in-pane rename")
	}
	if strings.Contains(ansi.Strip(m.statusBar()), "rename ▸") {
		t.Fatal("in-pane rename must NOT use the bottom bar")
	}
}
```

- [ ] **Step 6: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFileListInline|TestRenameRendersInRow|TestRename|TestConfirmRename' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w filelist.go main.go filelist_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add filelist.go main.go filelist_test.go smoke_test.go
git commit -m "filepane: render rename/create input in the file row (not the bottom bar)"
```

---

## Task 2: Rename triggers — right-click + F2 (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `startRename`, `m.files.selectRow`, `sidebarRow`, `m.renamingInPane`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestRightClickAndF2Rename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.sidebarVisible = true
	m.layout()
	// Right-click a file row → that row is selected and an in-pane rename starts.
	var row int
	for y, ln := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(ln, "02-b") {
			row = y
			break
		}
	}
	nm, _ = m.Update(tea.MouseMsg{X: 3, Y: row, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.renaming || !m.renamingInPane || !strings.Contains(m.files.entries[m.files.selected].name, "02-b") {
		t.Fatalf("right-click should select 02-b and start in-pane rename; renaming=%v sel=%q", m.renaming, m.files.entries[m.files.selected].name)
	}
	// Cancel, then F2 starts rename on the selection.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	m = nm.(model)
	if !m.renaming || !m.renamingInPane {
		t.Fatal("F2 should start an in-pane rename")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — right-click/F2 do nothing yet.

- [ ] **Step 3: Add the triggers** — in `main.go`:
  - In the `MouseMsg` block (after the existing left-click sidebar handling, where `inSidebar` is known), add a right-click case:
    ```go
    if inSidebar && msg.Button == tea.MouseButtonRight && msg.Action == tea.MouseActionPress {
        if row := sidebarRow(msg.Y, 1, m.files.height); row >= 0 {
            m.focus = focusSidebar
            m.editor.Blur()
            m.files.selectRow(row)
            m.startRename()
            return m, textinput.Blink
        }
    }
    ```
    (Use the same `bannerH` value the left-click file-row handler uses — `1` for the framed top border.)
  - Add an `F2` case in the writing-screen key switch (alongside the global keys like `ctrl+n`):
    ```go
    case "f2":
        m.focus = focusSidebar
        m.editor.Blur()
        m.startRename()
        return m, textinput.Blink
    ```

- [ ] **Step 4: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestRightClickAndF2Rename|TestRename' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "filepane: right-click + F2 start in-place rename"
```

---

## Task 3: In-place create + clickable `+` (`inspector.go`, `main.go`)

**Files:** Modify `inspector.go` (framedPanel), `main.go`; Test `inspector_test.go`, `smoke_test.go`

**Interfaces:** Produces `framedPanel(title, inner string, width, height int, action string)` (extra `action` arg); consumes `confirmCreate`, `createRowSentinel`.

- [ ] **Step 1: Write the failing test** — add to `inspector_test.go`:

```go
func TestFramedPanelAction(t *testing.T) {
	out := ansi.Strip(framedPanel("Files", "x", 20, 4, "+"))
	top := strings.Split(out, "\n")[0]
	if !strings.Contains(top, "+") || !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Fatalf("top border should carry the + action: %q", top)
	}
	if strings.Contains(strings.Split(ansi.Strip(framedPanel("Files", "x", 20, 4, "")), "\n")[0], "+") {
		t.Fatal("no action → no + in the border")
	}
}
```

And to `smoke_test.go`:

```go
func TestPlusStartsInPlaceCreate(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.sidebarVisible = true
	m.layout()
	// Find the '+' column on the sidebar's top border (row 0) and click it.
	top := strings.Split(ansi.Strip(m.View()), "\n")[0]
	plus := -1
	for i, r := range []rune(top) {
		if r == '+' && i < sidebarWidth {
			plus = i
		}
	}
	if plus < 0 {
		t.Fatalf("no + on the sidebar top border: %q", top)
	}
	nm, _ = m.Update(tea.MouseMsg{X: plus, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.creatingFile || !m.creatingInPane {
		t.Fatal("clicking + should start an in-pane create")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `framedPanel` arity; `+` not rendered.

- [ ] **Step 3: Add the `action` to `framedPanel`** — render it at the right of the top border before `╮`:

```go
func framedPanel(title, inner string, width, height int, action string) string {
	// ... existing setup ...
	// top border: ╭ <title> ─fill <action> ╮
	rightSeg := ""
	if action != "" {
		rightSeg = " " + action // styled like the title accent
	}
	fill := width - 2 - (lipgloss.Width(titleSeg) + lipgloss.Width(rightSeg))
	if fill < 0 { fill = 0 }
	top := bs.Render("╭") + ts.Render(" "+titleStr+" ") + bs.Render(strings.Repeat("─", fill)) + ts.Render(rightSeg) + bs.Render("╮")
	// ... rest unchanged ...
}
```

Update BOTH existing `framedPanel(...)` call sites: the inspector passes `""`; the sidebar passes `"+"`.

- [ ] **Step 4: Wire create in-place** — in `main.go`:
  - The sidebar `framedPanel(title, ..., "+")`.
  - A helper `startCreate()`: `m.creatingFile = true; m.creatingInPane = true; m.creatingFolder = false; m.nameInput.SetValue(""); m.nameInput.Width = m.files.width; m.nameInput.Focus(); m.editor.Blur(); m.focus = focusSidebar`.
  - `ctrl+n` calls `startCreate()` (replacing its inline body); `confirmCreate`/cancel clear `m.creatingInPane`.
  - Click handler: a `MouseButtonLeft` press at `msg.Y == 0` with `msg.X` at the sidebar's `+` column → `startCreate()`. Compute the `+` column from the rendered top border (find `+` in row 0 within `[0, sidebarWidth)`), or place the `+` at a fixed column `sidebarWidth-3` and test `msg.X == sidebarWidth-3`. Use the fixed-column approach and assert it in the test.

- [ ] **Step 5: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFramedPanelAction|TestPlusStartsInPlaceCreate|TestConfirmCreate|TestInspector' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go main.go inspector_test.go smoke_test.go
git commit -m "filepane: clickable + in the title bar starts an in-place create (ctrl+n too)"
```

---

## Task 4: Delete with confirm (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Produces `m.deleting bool`, `m.deleteTarget string`, `startDelete()`, `confirmDelete()`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestDeleteWithConfirm(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doomed.md")
	os.WriteFile(p, []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.focus = focusSidebar
	m.files.selectName("doomed.md")
	// Delete key → confirm prompt, file still there.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = nm.(model)
	if !m.deleting {
		t.Fatal("Delete should open a confirm")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("file must not be deleted before confirm")
	}
	// y confirms.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = nm.(model)
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("file should be gone after y")
	}
	if m.deleting {
		t.Fatal("confirm should close after y")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `m.deleting` undefined.

- [ ] **Step 3: Implement delete** — add `deleting bool` + `deleteTarget string` to `model`; a `startDelete()` that validates (skip `..`/`manifest.json`; block manifest chapters via `isChapterOf(m.files.view, name) && v.source==sourceManifest` with the status note) and sets `m.deleting = true`, `m.deleteTarget = name`, `m.status = "delete '" + name + "'? [y]es · esc cancel"`; and `confirmDelete()` that `os.Remove`/`os.RemoveAll`s `filepath.Join(m.files.dir, m.deleteTarget)`, refreshes (`m.files.SetDir(m.files.dir)` + clamp selection), clears `m.deleting`, sets a status. A modal block (like `m.renaming`) intercepts keys while `m.deleting`: `y` → `confirmDelete()`, any other key → cancel. Add a `tea.KeyDelete` trigger in the sidebar-focus key handling → `startDelete()`. The status bar already shows `m.status`.

- [ ] **Step 4: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestDeleteWithConfirm|TestRename' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "filepane: Delete key removes the selected file/folder (with y confirm; manifest chapters blocked)"
```

---

## Task 5: Duplicate (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Produces `duplicateSelected()`; consumes `copyFreeName`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestDuplicateFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "draft.md"), []byte("hello"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	m.focus = focusSidebar
	m.files.selectName("draft.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = nm.(model)
	b, err := os.ReadFile(filepath.Join(dir, "draft copy.md"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("duplicate should create 'draft copy.md' with the same bytes: err=%v", err)
	}
	// A second duplicate → "draft copy 2.md".
	m.files.selectName("draft.md")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(dir, "draft copy 2.md")); err != nil {
		t.Fatalf("second duplicate should be 'draft copy 2.md': %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `d` does nothing in the sidebar.

- [ ] **Step 3: Implement duplicate** — add a `d` case in the sidebar-focus key handling → `m.duplicateSelected()`:

```go
func (m *model) duplicateSelected() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." || e.isDir {
		m.status = "duplicate: files only"
		return
	}
	ext := filepath.Ext(e.name)
	stem := strings.TrimSuffix(e.name, ext)
	target := copyFreeName(m.files.dir, stem, ext)
	data, err := os.ReadFile(filepath.Join(m.files.dir, e.name))
	if err != nil {
		m.status = "duplicate failed: " + err.Error()
		return
	}
	if err := atomicWrite(filepath.Join(m.files.dir, target), data, 0o644); err != nil {
		m.status = "duplicate failed: " + err.Error()
		return
	}
	m.files.SetDir(m.files.dir)
	m.files.selectName(target)
	m.status = "duplicated → " + target
}

// copyFreeName returns "stem copy.ext", then "stem copy 2.ext", … that doesn't exist in dir.
func copyFreeName(dir, stem, ext string) string {
	name := stem + " copy" + ext
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	for i := 2; ; i++ {
		name = fmt.Sprintf("%s copy %d%s", stem, i, ext)
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}
```

(`atomicWrite` exists in `atomicwrite.go`; `fmt`/`os`/`strings`/`filepath` are imported in main.go.)

- [ ] **Step 4: Update `helpText` + run; gofmt; build; commit** — add to `helpText`: `F2  rename file`, `del  delete`, `d  duplicate` (and note right-click renames, `+` new):

```bash
/opt/homebrew/bin/go test . -run 'TestDuplicateFile|TestF1Help' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "filepane: d duplicates the selected file; document the file-pane keys in F1 help"
```

---

## Self-Review

**Spec coverage:** in-place input render (rename + create rows, bottom-bar suppression) → Task 1; right-click + F2 rename → Task 2; `+` action glyph + in-place create + ctrl+n → Task 3; Delete + confirm → Task 4; duplicate + helpText → Task 5.

**Placeholder scan:** none — full code or precise reuse of existing functions (`confirmRename`/`confirmCreate`/`startRename`/`sidebarRow`/`atomicWrite`). Task 1 Step 3 shows the View change as a pattern over the existing loop (the implementer adapts the existing rows), which is a real instruction, not a placeholder.

**Type consistency:** `filelist.View(editRow int, editField string)` + `createRowSentinel` (Task 1) used by main.go's call site and Task 3's create; `framedPanel(…, action string)` (Task 3) updates both call sites; `m.renamingInPane`/`m.creatingInPane`/`m.deleting`/`m.deleteTarget` consistent; `startRename`/`startCreate`/`startDelete`/`duplicateSelected`/`copyFreeName` names stable.

**Risk:** the `framedPanel` signature change (Task 3) touches the sidebar AND inspector call sites — both must pass the new arg or the build breaks; the controller will re-verify panel click alignment (the `+` on row 0, file rows below) empirically. Delete is destructive — the confirm modal must intercept before the editor/sidebar consume keys. Each task leaves the app compiling and the suite green.
