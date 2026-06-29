# Sidebar Framed-Card Styling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** Frame the file-pane sidebar as a rounded card (matching the inspector) with the current folder name as the title, keeping file-row and breadcrumb clicks aligned.

**Architecture:** Reuse the existing `framedPanel` helper; render the sidebar at exactly `sidebarWidth` (render == reservation == offset); the top border shifts both clickable zones (breadcrumb + file list) down one row.

**Tech Stack:** Go, lipgloss, the existing `framedPanel`.

**Design spec:** `docs/superpowers/specs/2026-06-29-sidebar-framed-card-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Chrome only — no file-pane behavior change.
- Render the sidebar at EXACTLY `sidebarWidth` so render == reservation (`editorArea -= sidebarWidth`) == offset; sidebar is at screen col 0.
- Click alignment is the gate: a file row and a breadcrumb segment, clicked at their on-screen positions, do the right thing.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before commit.

---

## Task 1: Frame the sidebar + shift click geometry (`main.go`, `styles.go`) — atomic

**Files:** Modify `styles.go`, `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `framedPanel` (existing), `filepath.Base`, `sidebarRow`, `m.files`.

- [ ] **Step 1: Write the failing alignment test** — add to `smoke_test.go` (the gate):

```go
func TestSidebarFramedClickAlignment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-alpha.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-bravo.md"), []byte("b"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 170, Height: 40})
	m = nm.(model)
	m.sidebarVisible = true
	m.inspector.visible = false
	m.layout()
	// Find a file entry's on-screen row and click it; it must become selected.
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	target, wantRow := "bravo", -1
	for y, ln := range lines {
		if strings.Contains(ln, target) {
			wantRow = y
			break
		}
	}
	if wantRow < 0 {
		t.Fatalf("file %q not on screen:\n%s", target, strings.Join(lines, "\n"))
	}
	nm, _ = m.Update(tea.MouseMsg{X: 3, Y: wantRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if sel := m.files.entries[m.files.selected].name; !strings.Contains(sel, target) {
		t.Fatalf("clicking the on-screen %q row (y=%d) selected %q instead — geometry misaligned", target, wantRow, sel)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestSidebarFramedClickAlignment 2>&1 | tail` → clicking the framed row selects the wrong entry (top border not accounted for).

- [ ] **Step 3: Widen the sidebar** — in `styles.go`, `const sidebarWidth = 32` → `34`.

- [ ] **Step 4: Frame the sidebar column** — in `main.go`, replace the sidebar build:

```go
	if showSidebar {
		sideInner := lipgloss.JoinVertical(
			lipgloss.Left,
			func() string { row, _ := m.files.breadcrumbBar(sidebarWidth - 4); return breadcrumbStyle.Render(row) }(),
			m.files.View(),
		)
		title := filepath.Base(m.files.dir)
		if m.files.dir == "" {
			title = "Files"
		}
		cols = append(cols, framedPanel(title, sideInner, sidebarWidth, bodyH-2))
	}
```

(Was `breadcrumbBar(sidebarWidth - 3)` and `sidebarStyle.Width(sidebarWidth-1).Height(bodyH-2).Render(sideInner)`. Ensure `path/filepath` is imported — it already is. If `sidebarStyle` is now unused anywhere, remove its declaration in `styles.go`; if still referenced, leave it.)

- [ ] **Step 5: Update the sidebar content size** — in `main.go`'s layout (the `if showSidebar {` block that sets `m.files.height`/`m.files.width`):

```go
		m.files.height = bodyH - 5 // framed: content (bodyH-4) minus the breadcrumb row
		m.files.width = sidebarWidth - 4
```

(Was `bodyH - 3` and `sidebarWidth - 3`.)

- [ ] **Step 6: Shift the sidebar mouse hit-tests by the top border** — in `main.go`'s `MouseMsg` sidebar block:
  - Breadcrumb row: `if msg.Y == 0 {` → `if msg.Y == 1 {`; inside it `col := msg.X - 1` → `col := msg.X - 2` (left border + padding).
  - File rows: `row := sidebarRow(msg.Y, 1, m.files.height)` → `row := sidebarRow(msg.Y, 2, m.files.height)` (top border + breadcrumb).
  Update the adjacent comment (`Breadcrumb header occupies row 0…`) to reflect the new offsets.

- [ ] **Step 7: Run the gate + update existing tests** — run `TestSidebarFramedClickAlignment` (now PASS) and the existing file-pane/sidebar tests; any that click the sidebar or assert `m.files.height`/positions must move by the top border (+1 row) or find-on-screen, WITHOUT weakening assertions.

- [ ] **Step 8: Verify, gofmt, build, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSidebar|TestFiles|TestBreadcrumb' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w styles.go main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add styles.go main.go smoke_test.go
git commit -m "sidebar: framed-card panel (rounded border + folder title); realign click geometry"
```

---

## Self-Review

**Spec coverage:** frame via `framedPanel(folderName,…,sidebarWidth,…)` + widen 32→34 + inner width `sidebarWidth-4` + `files.height bodyH-5` → Steps 3-5; breadcrumb/file click Y+1 and breadcrumb X `-2` → Step 6; alignment gate → Step 1.

**Placeholder scan:** none — exact edits with before/after values.

**Type consistency:** `framedPanel(title,inner string,width,height int) string` reused; `sidebarWidth` (now 34) drives the frame width, the reservation (`editorArea -= sidebarWidth`), the breadcrumb width (`-4`), `m.files.width` (`-4`), and the click offsets — all consistent; `sidebarRow(mouseY, bannerH=2, listHeight)`.

**Risk:** the click geometry (top border + breadcrumb = 2 banner rows; breadcrumb X gains the border col). The render-based `TestSidebarFramedClickAlignment` gate plus controller re-verification (rune-column, file rows AND breadcrumb segments, no overflow) is the check — same approach that de-risked the inspector. Confirm the sidebar at col 0 still sums with editor+inspector to ≤ window width.
