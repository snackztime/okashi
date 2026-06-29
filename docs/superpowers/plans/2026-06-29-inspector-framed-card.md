# Inspector Framed-Card Styling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result.

**Goal:** Restyle the inspector panel as a rounded "card" — full border with the active tab name as the title, and labeled-rule section headers — keeping mouse clicks aligned.

**Architecture:** A pure `framedPanel()` + `sectionHeader()` (Task 1) are wired into `inspector.View` + `main.go` (Task 2), which also shifts the click geometry (top border adds a row; content narrows by 2 cols) and is gated by a render-based alignment test.

**Tech Stack:** Go, lipgloss, `charmbracelet/x/ansi`.

**Design spec:** `docs/superpowers/specs/2026-06-29-inspector-framed-card-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Chrome only — no inspector *data* changes.
- The frame: full rounded border (`subtle`) + active-tab title (`accent`, in the top border); inner
  content width = `panelWidth - 4` (2 borders + 2 padding); top border is row 0, **tab bar is row 1**.
- Click alignment is the gate: each tab and each Analysis checkbox's on-screen position must equal
  its hit-test target (render-based test).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Rendering helpers (`framedPanel`, `sectionHeader`) — pure, additive

**Files:** Modify `inspector.go` (or a new `panelchrome.go`); Test `inspector_test.go`

**Interfaces:**
- Produces: `framedPanel(title, inner string, width, height int) string`; `sectionHeader(label string, width int) string`.

- [ ] **Step 1: Write the failing test** — add to `inspector_test.go`:

```go
func TestFramedPanel(t *testing.T) {
	out := framedPanel("Words", "alpha\nbeta", 20, 6)
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(lines) != 6 {
		t.Fatalf("framedPanel height: want 6 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.Contains(lines[0], "Words") || !strings.HasSuffix(lines[0], "╮") {
		t.Fatalf("top border malformed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[5], "╰") || !strings.HasSuffix(lines[5], "╯") {
		t.Fatalf("bottom border malformed: %q", lines[5])
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != 20 {
			t.Fatalf("line %d width = %d, want 20: %q", i, w, ln)
		}
	}
	// content present and bordered
	if !strings.Contains(lines[1], "alpha") || !strings.HasPrefix(lines[1], "│") || !strings.HasSuffix(lines[1], "│") {
		t.Fatalf("content line malformed: %q", lines[1])
	}
}

func TestSectionHeader(t *testing.T) {
	out := ansi.Strip(sectionHeader("Document", 24))
	if !strings.HasPrefix(out, "DOCUMENT ") {
		t.Fatalf("section header should start with UPPERCASE label + space: %q", out)
	}
	if !strings.Contains(out, "─") {
		t.Fatalf("section header should have a rule fill: %q", out)
	}
	if lipgloss.Width(out) != 24 {
		t.Fatalf("section header width = %d, want 24", lipgloss.Width(out))
	}
}
```

(Ensure `inspector_test.go` imports `"strings"`, `"github.com/charmbracelet/lipgloss"`, `"github.com/charmbracelet/x/ansi"`.)

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run 'TestFramedPanel|TestSectionHeader' 2>&1 | tail` → undefined.

- [ ] **Step 3: Implement the helpers** — add to `inspector.go`:

```go
// sectionHeader renders an UPPERCASE accent label followed by a subtle rule to width.
func sectionHeader(label string, width int) string {
	up := strings.ToUpper(label)
	hs := lipgloss.NewStyle().Foreground(accent).Bold(true)
	fill := width - lipgloss.Width(up) - 1
	if fill < 0 {
		fill = 0
	}
	return hs.Render(up) + " " + lipgloss.NewStyle().Foreground(subtle).Render(strings.Repeat("─", fill))
}

// framedPanel wraps inner (multi-line) in a rounded box of the given total width/height,
// with title injected into the top border. Inner lines are padded/truncated ansi-aware.
func framedPanel(title, inner string, width, height int) string {
	if width < 6 {
		width = 6
	}
	if height < 2 {
		height = 2
	}
	bs := lipgloss.NewStyle().Foreground(subtle)
	ts := lipgloss.NewStyle().Foreground(accent).Bold(true)
	contentW := width - 4 // │ <space> content <space> │

	titleStr := title
	maxTitle := width - 4 // ╭, " ", at least one ─, " " is folded into title segment, ╮
	if lipgloss.Width(titleStr) > maxTitle {
		titleStr = ansi.Truncate(titleStr, maxTitle, "")
	}
	fill := width - 2 - (lipgloss.Width(titleStr) + 2) // minus ╭╮, minus the two spaces around the title
	if fill < 0 {
		fill = 0
	}
	top := bs.Render("╭") + ts.Render(" "+titleStr+" ") + bs.Render(strings.Repeat("─", fill)+"╮")

	cell := lipgloss.NewStyle().Width(contentW).MaxWidth(contentW)
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, height)
	out = append(out, top)
	for r := 0; r < height-2; r++ {
		c := ""
		if r < len(lines) {
			c = lines[r]
		}
		out = append(out, bs.Render("│")+" "+cell.Render(c)+" "+bs.Render("│"))
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(out, "\n")
}
```

(If `inspector.go` doesn't already import `ansi`, add `"github.com/charmbracelet/x/ansi"`.)

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFramedPanel|TestSectionHeader' -v 2>&1 | tail -10
/opt/homebrew/bin/gofmt -w inspector.go inspector_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go inspector_test.go
git commit -m "inspector: framedPanel + sectionHeader chrome helpers"
```

---

## Task 2: Wire the frame + shift click geometry (`inspector.go`, `main.go`) — atomic

**Files:** Modify `inspector.go`, `main.go`; Test `inspector_test.go`, `smoke_test.go`

**Interfaces:** Consumes `framedPanel`/`sectionHeader` (Task 1), `inspectorTabLabels`, the existing `analysisRowY`/`inspectorTabAtX`/`inspectorAnalysisRowAtY`/`inspectorInnerWidth`.

- [ ] **Step 1: Write the failing alignment test** — add to `smoke_test.go` (this is the gate):

```go
func TestFramedInspectorClickAlignment(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = nm.(model)
	m.inspector.visible = true
	m.inspector.tab = tabAnalysis
	m.layout()
	// Click the Adverb checkbox row at its on-screen position; it must toggle adverb.
	x := m.width - inspectorWidth + 4 // into the content (left border+padding+indent)
	// y must be wherever Adverb actually renders on screen — find it:
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	yAdverb := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Adverb") {
			yAdverb = i
		}
	}
	if yAdverb < 0 {
		t.Fatal("Adverb row not found on screen")
	}
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: yAdverb, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.adverb {
		t.Fatalf("clicking the on-screen Adverb row (y=%d) must toggle adverb — geometry misaligned", yAdverb)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — initially the click is a row off (top border not accounted for) → adverb stays false.

- [ ] **Step 3: Use rule headers + indent in `inspector.View`** — replace each `breadcrumbStyle.Render("X")` header with `sectionHeader("X", width)`, and indent content rows by 2 spaces. Example for the Words tab body (apply the same pattern to Goals/Outline/Analysis):

```go
	default: // tabWords
		b.WriteString(sectionHeader("Document", width) + "\n")
		b.WriteString("  " + kvRow("Words", doc.words, width-2) + "\n")
		b.WriteString("  " + kvRow("Characters", doc.chars, width-2) + "\n")
		b.WriteString("  " + kvRow("Paragraphs", doc.paragraphs, width-2) + "\n\n")
		b.WriteString(sectionHeader("Project", width) + "\n")
		b.WriteString("  " + kvRow("Words", proj.words, width-2))
		if proj.manuscript {
			b.WriteString("\n  " + kvRow("Chapters", proj.chapters, width-2))
		}
```

(Keep the tab bar as the first inner line. The Analysis tab's `[ ] Spellcheck`/POS rows likewise get a 2-space indent; `analysisRowY` must continue to point at the rendered checkbox rows — see Step 5.)

- [ ] **Step 4: Frame the panel in `main.go`** — replace the inspector column build (`inspectorStyle.Width(inspectorWidth-1).Height(bodyH-2).Render(insInner)`) with:

```go
		title := inspectorTabLabels()[m.inspector.tab]
		cols = append(cols, framedPanel(title, insInner, inspectorWidth-1, bodyH-2))
```

And update `inspectorInnerWidth()` to the framed content width — full box at total `inspectorWidth-1`: `return inspectorWidth - 1 - 4` (2 borders + 2 padding). Keep it the single source `View` is called with.

- [ ] **Step 5: Shift the mouse hit-tests by the top border** — in `main.go`'s `MouseMsg` handler, the inspector content now starts one row lower (top border). Where it currently treats `msg.Y` as the content row, subtract 1. Concretely, compute `contentY := msg.Y - 1` once and use it for both the tab-row check (`contentY == 0`) and `inspectorAnalysisRowAtY(contentY)`. The `localX` offset is unchanged (content still starts at panel col 2: left border + padding). If `analysisRowY`/`inspectorTabAtX` are defined relative to the inner content (row 0 = tab bar), they stay as-is; only the main.go `msg.Y - 1` shift is new. Verify against the indentation: `localX := msg.X - (m.width - inspectorWidth) - 2` may need `- 2` more if the 2-space content indent pushes the checkbox glyph right — adjust so a click on the visible `[ ]` toggles (the alignment test is the check).

- [ ] **Step 6: Update existing geometry tests** — `TestTabBarFitsOneRow` (inner width is now `inspectorWidth-1-4`), `TestAnalysisRowAtY`, and the existing tab-click/checkbox-click smoke tests must use the framed positions (find the row on-screen like the alignment test, or add 1 for the top border). Make them pass against the new layout without weakening assertions.

- [ ] **Step 7: Run tests; gofmt; build; visual check; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestFramedInspectorClickAlignment|TestAnalysis|TestInspector|TestTabBar|TestSpellcheckToggle|TestPOSToggle' -v 2>&1 | tail -30
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go main.go inspector_test.go smoke_test.go
git commit -m "inspector: framed-card panel (rounded border + tab title + rule headers); realign click geometry"
```

---

## Self-Review

**Spec coverage:** `framedPanel` + `sectionHeader` → Task 1; rounded border + tab title + rule headers + 2-space indent + the geometry shift (inner width `-4`, `msg.Y-1`) + alignment gate → Task 2.

**Placeholder scan:** none — full code for the helpers; Task 2 Steps 3/5/6 describe a mechanical pattern applied across tabs with the exact transform shown and the alignment test as the objective gate (Step 5's "adjust so the click toggles" is bounded by `TestFramedInspectorClickAlignment`, not a vague placeholder).

**Type consistency:** `framedPanel(title,inner string,width,height int) string`, `sectionHeader(label string,width int) string` (Task 1) consumed by `main.go`/`inspector.View` (Task 2); `inspectorInnerWidth()` recomputed once and used by `View`, `framedPanel` width, and the tab bar; `msg.Y-1` shift paired with unchanged `analysisRowY`/`inspectorTabAtX`.

**Risk:** the click geometry is the danger (top border + indent). The render-based `TestFramedInspectorClickAlignment` (find the row on-screen, click it, assert it toggles) is the gate — and the controller will re-verify alignment empirically for ALL tabs/checkboxes before merge, as with the prior tab-bar fix. The tab bar must still fit one row at the new narrower inner width.
