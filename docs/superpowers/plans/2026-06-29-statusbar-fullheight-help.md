# Status Bar + Full-Height Panels + F1 Help Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** Full-height side panels; a status bar with stats on the left and the last save/open on the right aligned to the editor text column; and the verbose keybind hints moved to an F1 cheatsheet.

**Architecture:** Three small, independent changes — panel heights in `View`; `composeStatus` reworked to span the editor text column; an `m.showHelp` toggle on F1 rendering a centered `framedPanel` cheatsheet.

**Tech Stack:** Go, lipgloss, `charmbracelet/x/ansi`, the existing `framedPanel`/`effectivePanels`.

**Design spec:** `docs/superpowers/specs/2026-06-29-statusbar-fullheight-help-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- `?` types text in the editor — the help toggle is **F1** (`tea.KeyF1`), never a plain key.
- Status content positions are 0-based in the string `composeStatus` returns; `statusStyle` adds 1 col left padding, so content col 0 = screen col 1 (the existing `-1` adjustment).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Full-height panels (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestPanelsFullHeight(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = nm.(model)
	m.sidebarVisible = true
	m.inspector.visible = true
	m.layout()
	bodyH := m.height - 1
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	// The sidebar's bottom border (╰) must be on the last body row (bodyH-1), i.e. flush with the editor.
	if !strings.Contains(lines[bodyH-1], "╰") {
		t.Fatalf("panels should be full height — bottom border expected on row %d:\n%s", bodyH-1, lines[bodyH-1])
	}
	if m.files.height != bodyH-2 {
		t.Fatalf("files.height = %d, want bodyH-2 = %d", m.files.height, bodyH-2)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestPanelsFullHeight 2>&1 | tail` → bottom border on the wrong row / files.height mismatch.

- [ ] **Step 3: Make panels full height** — in `main.go`'s `View`, change both `framedPanel(..., bodyH-2)` calls to `framedPanel(..., bodyH)` (sidebar + inspector). In the layout function, change `m.files.height = bodyH - 4` → `m.files.height = bodyH - 2`.

- [ ] **Step 4: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPanelsFullHeight|TestSidebar|TestInspector' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "panels: render the framed sidebar + inspector at full height (flush with the editor)"
```

---

## Task 2: Status bar layout — stats left, save/open right over the editor text column (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Reworks `composeStatus(status, stats string) string`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestStatusBarLeftRight(t *testing.T) {
	m := initialModel()
	m.width = 120
	m.height = 30
	m.colWidth = 72
	m.sidebarVisible = false
	m.inspector.visible = false
	stats := "✓ 1,240 words · +142 session"
	out := ansi.Strip(m.composeStatus("saved draft.md", stats))
	si := strings.Index(out, "1,240")
	di := strings.Index(out, "saved")
	if si < 0 || di < 0 {
		t.Fatalf("both stats and status should render: %q", out)
	}
	if si >= di {
		t.Fatalf("stats must be left of the status: stats@%d status@%d in %q", si, di, out)
	}
	// A very long status truncates rather than pushing the stats off / overrunning.
	long := strings.Repeat("verylongmessage ", 20)
	out2 := ansi.Strip(m.composeStatus(long, stats))
	if !strings.Contains(out2, "1,240") {
		t.Fatalf("stats must survive a long status: %q", out2)
	}
	if lipgloss.Width(out2) > m.width {
		t.Fatalf("status row overflows width: %d > %d", lipgloss.Width(out2), m.width)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — the current `composeStatus` puts status left + stats centered → `si >= di`.

- [ ] **Step 3: Rewrite `composeStatus`** in `main.go`:

```go
// composeStatus lays the stats at the editor text's left edge and the status
// (last save/open) right-aligned to the text's right edge, within the editor
// text column. Stats win if both don't fit.
func (m model) composeStatus(status, stats string) string {
	showSidebar, _, editorArea := m.effectivePanels()
	editorStart := 0
	if showSidebar {
		editorStart = sidebarWidth
	}
	cw := min(m.colWidth, editorArea-2)
	totalW := m.width - 2 // statusStyle pads one col each side
	sw := lipgloss.Width(stats)
	if cw < sw+1 || totalW < sw {
		return stats // too narrow for the two-element layout
	}
	left := editorStart + (editorArea-cw)/2 - 1 // content col of the text's left edge
	if left < 0 {
		left = 0
	}
	if left+sw > totalW {
		left = totalW - sw
	}
	// status right-aligned to the text right edge (content col left+cw), truncated to fit.
	avail := cw - sw - 1
	st := status
	if lipgloss.Width(st) > avail {
		if avail <= 0 {
			return strings.Repeat(" ", left) + stats
		}
		st = ansi.Truncate(st, avail, "…")
	}
	stW := lipgloss.Width(st)
	statusStart := left + cw - stW
	used := left + sw
	if statusStart < used+1 {
		statusStart = used + 1
	}
	if statusStart+stW > totalW {
		statusStart = totalW - stW
	}
	if statusStart < used {
		return strings.Repeat(" ", left) + stats
	}
	return strings.Repeat(" ", left) + stats + strings.Repeat(" ", statusStart-used) + st
}
```

(The `statusBar()` normal-case call `return m.composeStatus(m.status, stats)` is unchanged — only the layout inside `composeStatus` changes. `ansi` must be imported in main.go — it already is.)

- [ ] **Step 4: Run; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestStatusBar|TestCompose|TestStatus' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "status: stats left + save/open right, aligned to the editor text column"
```

---

## Task 3: F1 keybind cheatsheet (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Adds `m.showHelp bool`, a `helpText` constant, F1 handling, and a help render branch.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestF1HelpToggle(t *testing.T) {
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = nm.(model)
	if m.showHelp {
		t.Fatal("help should default closed")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = nm.(model)
	if !m.showHelp {
		t.Fatal("F1 should open help")
	}
	if !strings.Contains(ansi.Strip(m.View()), "ctrl+b") {
		t.Fatalf("help view should list keybinds:\n%s", ansi.Strip(m.View()))
	}
	// esc closes; '?' would NOT toggle (it types text) — F1 closes here.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = nm.(model)
	if m.showHelp {
		t.Fatal("F1 again should close help")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — undefined `m.showHelp`.

- [ ] **Step 3: Implement** — in `main.go`:
  - Add `showHelp bool` to the `model` struct.
  - Add a `helpText` constant — the keybinds as a readable multi-line list, e.g.:
    ```go
    const helpText = `ctrl+b   toggle sidebar
ctrl+y   toggle inspector
ctrl+n   new file
r        rename (sidebar)
ctrl+l   outline
ctrl+k   binder
ctrl+e   export
ctrl+p   preview
ctrl+t   typewriter
ctrl+d   focus dim
ctrl+s   save
ctrl+g   set goals
ctrl+r   spelling suggestions
ctrl+o   home
esc      switch focus / back
ctrl+c   quit`
    ```
  - Change the default `status:` field (the long hint at the struct literal) to `""` (the hints now live in F1 help).
  - In `Update`, BEFORE the editor consumes keys: if `m.showHelp`, any key (esp. F1/esc) closes it and returns; else a `tea.KeyF1` opens it. (Mirror the `m.suggesting` modal placement.)
  - In `View` (writing screen), when `m.showHelp`, render a centered help card over the body instead of the columns:
    ```go
    if m.showHelp {
        card := framedPanel("Keys", helpText, 36, min(bodyH, lipgloss.Height(helpText)+2))
        body := lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Center, card)
        return lipgloss.JoinVertical(lipgloss.Left, body, statusStyle.Width(m.width).Render("F1/esc close"))
    }
    ```
    (Place this early in the writing-screen View branch.)

- [ ] **Step 4: Run; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestF1Help|TestStatus' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "help: F1 toggles a keybind cheatsheet; drop the always-on hint from the status default"
```

---

## Self-Review

**Spec coverage:** full-height panels (`bodyH`, files.height `bodyH-2`) → Task 1; stats-left/status-right over editor text column → Task 2; F1 cheatsheet + default-status change → Task 3.

**Placeholder scan:** none — full code. `helpText` keybind list is concrete (kept in sync with the real keymap).

**Type consistency:** `composeStatus(status, stats string) string` signature unchanged (only internals); `m.showHelp` + `helpText` + F1 handling consistent; `framedPanel`/`effectivePanels` reused.

**Risk:** Task 2's column arithmetic is fiddly — the `TestStatusBarLeftRight` gate (stats left of status, long status truncates, no overflow) plus controller visual re-check is the safety net. Task 3's F1 modal must intercept before the editor (so help keys don't edit) and `?` must STILL type text (only F1 toggles). The three tasks are independent — each leaves the app working.
