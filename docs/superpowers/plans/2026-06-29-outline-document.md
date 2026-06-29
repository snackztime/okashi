# Outline Document + Inspector Outline Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Add a per-project planning `outline.md` (edited in the main editor via `ctrl+l`), shown read-only in the inspector's new Outline tab; rename the existing chapter navigator to "Binder" on `ctrl+k`; make `ctrl+y` cycle inspector tabs.

**Architecture:** `ctrl+l` toggles the editor between the current chapter and `outline.md` (reusing `loadFile`/`save`), remembering the return file. The binder (existing `screenOutline`) moves to `ctrl+k` with user-facing labels relabeled. The inspector gains a second tab and a `cycle()` method; `ctrl+y` cycles Words→Outline→closed.

**Tech Stack:** Go, lipgloss, `charmbracelet/x/ansi`, existing `loadFile`/`save`/`atomicWrite`/`enterOutline`.

**Design spec:** `docs/superpowers/specs/2026-06-29-outline-document-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt`.
- `ctrl+l` = outline-doc toggle (writing screen); `ctrl+k` = binder; `ctrl+y` = cycle inspector tabs.
- `outline.md` lives in `m.files.dir`; created with starter `"- \n"` if absent; it's a normal visible Resource (no order/export changes). Writes go through `save()`/`atomicWrite` (already atomic).
- Binder behavior unchanged — only user-facing "outline" labels become "Binder"/"binder". Internal `screenOutline`/`outlineModel`/`enterOutline` unchanged.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Outline-doc toggle (ctrl+l) + Binder rebind (ctrl+k) + relabel (`main.go`)

**Files:**
- Modify: `main.go`
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `outlineReturnFile string` field on `model`; `ctrl+l` outline toggle; `ctrl+k` binder.

- [ ] **Step 1: Write the failing tests**

Add to `smoke_test.go`:

```go
func TestOutlineDocToggle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("chapter body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	m.loadFile(filepath.Join(dir, "01-a.md"))

	// ctrl+l opens outline.md (created on disk).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if filepath.Base(m.currentFile) != "outline.md" {
		t.Fatalf("ctrl+l should open outline.md, got %q", m.currentFile)
	}
	if _, err := os.Stat(filepath.Join(dir, "outline.md")); err != nil {
		t.Fatal("outline.md should be created on disk")
	}
	// ctrl+l again returns to the chapter.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if filepath.Base(m.currentFile) != "01-a.md" {
		t.Fatalf("second ctrl+l should return to the chapter, got %q", m.currentFile)
	}
}

func TestBinderOnCtrlK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("body"), 0o644)
	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.screen = screenWriting
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = nm.(model)
	if m.screen != screenOutline {
		t.Fatalf("ctrl+k should open the binder, got screen %v", m.screen)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestOutlineDocToggle|TestBinderOnCtrlK' 2>&1 | tail`
Expected: FAIL — `ctrl+l` still opens the binder (currentFile not outline.md); `ctrl+k` does nothing (screen stays writing).

- [ ] **Step 3: Add the `outlineReturnFile` field**

In `main.go`, add to the `model` struct (near `currentFile`):

```go
	outlineReturnFile string // chapter to return to after editing outline.md (ctrl+l)
```

- [ ] **Step 4: Replace the `ctrl+l` case with the outline toggle; add `ctrl+k` for the binder**

Find the existing writing-screen case:

```go
		case "ctrl+l":
			if m.files.view.ordered() {
				m.enterOutline()
			} else {
				m.status = "not a manuscript"
			}
			return m, nil
```

Replace it with:

```go
		case "ctrl+l":
			outlinePath := filepath.Join(m.files.dir, "outline.md")
			if m.currentFile == outlinePath {
				m.save()
				if m.outlineReturnFile != "" {
					m.loadFile(m.outlineReturnFile)
				}
			} else {
				m.save()
				m.outlineReturnFile = m.currentFile
				if _, err := os.Stat(outlinePath); err != nil {
					if werr := atomicWrite(outlinePath, []byte("- \n"), 0o644); werr != nil {
						m.status = "couldn't create outline: " + werr.Error()
						return m, nil
					}
					m.files.SetDir(m.files.dir) // surface outline.md in the sidebar
				}
				m.loadFile(outlinePath)
			}
			return m, nil
		case "ctrl+k":
			if m.files.view.ordered() {
				m.enterOutline()
			} else {
				m.status = "not a manuscript"
			}
			return m, nil
```

- [ ] **Step 5: Relabel user-facing "outline" → "Binder"**

Three status-hint strings in `main.go`:
- The writing hint (in `initialModel`): change `· ctrl+l outline ·` to `· ctrl+l outline · ctrl+k binder ·`.
- The binder-screen hint (currently `"outline · ↑↓ select · enter open · r rename · m read · ctrl+e export · esc back"`): change the leading `outline` to `binder` → `"binder · ↑↓ select · enter open · r rename · m read · ctrl+e export · esc back"`.
- The pager hint (currently `"manuscript · ↑↓ scroll · enter edit here · o outline · esc editor"`): change `o outline` to `o binder` → `"manuscript · ↑↓ scroll · enter edit here · o binder · esc editor"`.

- [ ] **Step 6: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestOutlineDocToggle|TestBinderOnCtrlK' -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "outline: ctrl+l edits outline.md (toggle); ctrl+k opens binder (renamed from outline)"
```

---

## Task 2: Inspector Outline tab + ctrl+y cycle (`inspector.go`, `main.go`)

**Files:**
- Modify: `inspector.go` (tabs, `cycle`, `View` + outline render), `main.go` (`readOutlineDoc`, View wiring, `ctrl+y`), `inspector_test.go` (update existing `View` calls)
- Test: `inspector_test.go`

**Interfaces:**
- Consumes: `inspectorModel`, `docStats`, `projStats`, `accent`, `subtle`, `breadcrumbStyle`, `selectedStyle` (existing).
- Produces: `const ( tabWords inspectorTab = iota; tabOutline )`; `func inspectorTabLabels() []string`; `func (in *inspectorModel) cycle()`; `func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string) string`; `func readOutlineDoc(dir string) string`.

- [ ] **Step 1: Write the failing tests**

Add to `inspector_test.go`:

```go
func TestInspectorCycle(t *testing.T) {
	in := inspectorModel{}
	in.cycle()
	if !in.visible || in.tab != tabWords {
		t.Fatalf("first cycle: visible=%v tab=%v, want visible Words", in.visible, in.tab)
	}
	in.cycle()
	if !in.visible || in.tab != tabOutline {
		t.Fatalf("second cycle: visible=%v tab=%v, want visible Outline", in.visible, in.tab)
	}
	in.cycle()
	if in.visible {
		t.Fatal("third cycle should close the inspector (past the last tab)")
	}
	if in.tab != tabWords {
		t.Fatalf("closed cycle should reset tab to Words, got %v", in.tab)
	}
}

func TestInspectorOutlineTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabOutline}
	out := in.View(28, docStats{}, projStats{}, "- Top\n  - sub")
	for _, want := range []string{"Outline", "Top", "sub"} {
		if !strings.Contains(out, want) {
			t.Fatalf("outline tab missing %q:\n%s", want, out)
		}
	}
	empty := in.View(28, docStats{}, projStats{}, "")
	if !strings.Contains(empty, "empty") {
		t.Fatal("empty outline should show an (empty …) hint")
	}
}
```

Also UPDATE the existing `TestInspectorViewRendersWords` calls to pass the new `outline` arg — add `, ""` to both `in.View(...)` calls.

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestInspectorCycle|TestInspectorOutlineTab' 2>&1 | tail`
Expected: build errors — `cycle`/`tabOutline` undefined; `View` arity mismatch.

- [ ] **Step 3: Update `inspector.go` — tabs, cycle, outline render, View arg**

Replace the tab const + add the labels helper:

```go
type inspectorTab int

const (
	tabWords inspectorTab = iota
	tabOutline
)

// inspectorTabLabels is the single source of the tab set — used by both the tab
// bar render and cycle() so they never diverge.
func inspectorTabLabels() []string { return []string{"Words", "Outline"} }

// cycle advances the inspector: hidden → Words → Outline → … → hidden.
func (in *inspectorModel) cycle() {
	if !in.visible {
		in.visible = true
		in.tab = tabWords
		return
	}
	in.tab++
	if int(in.tab) >= len(inspectorTabLabels()) {
		in.visible = false
		in.tab = tabWords
	}
}

// renderOutline shows the outline read-only: top-level bullets in accent, nested
// lines plain, each truncated to width. Empty → a hint.
func renderOutline(text string, width int) string {
	if strings.TrimSpace(text) == "" {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty — ctrl+l to edit)")
	}
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		shown := ansi.Truncate(line, width, "…")
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(shown))
		} else {
			b.WriteString(shown)
		}
	}
	return b.String()
}
```

Add `"github.com/charmbracelet/x/ansi"` to `inspector.go`'s imports.

Change the `View` signature and render the active tab. Replace the existing `View`'s tab-bar + body construction so it (a) iterates `inspectorTabLabels()` for the bar, and (b) switches on `in.tab`:

```go
func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string) string {
	var bar strings.Builder
	for i, t := range inspectorTabLabels() {
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
	switch in.tab {
	case tabOutline:
		b.WriteString(breadcrumbStyle.Render("Outline") + "\n\n")
		b.WriteString(renderOutline(outline, width))
	default: // tabWords
		b.WriteString(breadcrumbStyle.Render("Document") + "\n")
		b.WriteString(kvRow("Words", doc.words, width) + "\n")
		b.WriteString(kvRow("Characters", doc.chars, width) + "\n")
		b.WriteString(kvRow("Paragraphs", doc.paragraphs, width) + "\n\n")
		b.WriteString(breadcrumbStyle.Render("Project") + "\n")
		b.WriteString(kvRow("Words", proj.words, width))
		if proj.manuscript {
			b.WriteString("\n" + kvRow("Chapters", proj.chapters, width))
		}
	}
	return b.String()
}
```

(Keep `kvRow` as-is. If the old `View` had a different inner structure, preserve the Words body exactly — only the tab-bar source and the `switch` wrapper are new.)

- [ ] **Step 4: Wire `main.go` — `readOutlineDoc`, View call, ctrl+y cycle**

Add the helper (near other small helpers):

```go
// readOutlineDoc returns the project's outline.md content ("" if none) for the
// inspector's read-only Outline tab.
func readOutlineDoc(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "outline.md"))
	if err != nil {
		return ""
	}
	return string(data)
}
```

In `View()`, where the inspector column is built, pass the outline text:

```go
		doc := computeDocStats(m.editor.Value())
		proj := computeProjStats(m.files.dir, m.files.view, m.files.wc)
		insInner := m.inspector.View(inspectorWidth-3, doc, proj, readOutlineDoc(m.files.dir))
```

Change the `ctrl+y` case from toggle to cycle:

```go
		case "ctrl+y":
			m.inspector.cycle()
			m.layout()
			return m, nil
```

- [ ] **Step 5: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestInspector' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go main.go inspector_test.go
git commit -m "inspector: Outline tab (read-only outline.md) + ctrl+y cycles tabs"
```

---

## Self-Review

**Spec coverage:** outline-doc toggle + create-if-absent + return-file → Task 1; binder rebind to ctrl+k + relabel → Task 1; inspector Outline tab + read-only render + ctrl+y cycle → Task 2. Storage = visible outline.md (Resource, no order/export change) → Task 1 (created via atomicWrite). The existing-test `View`-arg update → Task 2 Step 1.

**Placeholder scan:** none — full code in every code step.

**Type consistency:** `outlineReturnFile string`; `cycle()` (pointer receiver), `inspectorTabLabels() []string`, `tabWords`/`tabOutline`, `View(width, docStats, projStats, string) string`, `readOutlineDoc(string) string`, `renderOutline(string, int) string` — defined and consumed consistently. `ctrl+y` handler updated to `cycle()`; the existing inspector `View` call sites (main.go + inspector_test.go) all updated to the new 4-arg signature.

**Integration note (executor):** the `ctrl+y` case currently does `m.inspector.visible = !m.inspector.visible` — replace that whole case body with the `cycle()` form. The inspector `View` is called only when visible (in the writing `View()`), so `readOutlineDoc` reads the tiny file only while the panel is up. Confirm the existing `TestInspectorToggleAndRender` still passes — first `ctrl+y` shows Words (cycle from hidden), so "Document" appears; a second+third `ctrl+y` would reach Outline then close (the existing test only presses twice — after the rebind, the 2nd press goes to Outline, NOT closed; update that test if it asserted closed-on-second-press). Read the existing test and adjust its expectations to the cycle semantics.
