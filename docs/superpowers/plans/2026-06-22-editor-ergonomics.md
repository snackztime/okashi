# Editor Ergonomics — Implementation Plan (Plan A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `esc` pane-switch, `Tab`/`Shift+Tab` indent/outdent, auto-continue Markdown lists, smart curly quotes, and a configurable column width.

**Architecture:** Editing primitives become small exported methods on the vendored `okashi/internal/textarea` (direct `value`/`row`/`col` manipulation). List-continuation and smart-quote logic are pure helpers in `main`, applied by intercepting keys in the editor-routing branch before forwarding to the editor.

**Tech Stack:** Go, Bubble Tea v1.1.0, vendored textarea.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- Indent unit = **2 spaces** (`const okashiIndentUnit = "  "`). Tab inserts spaces, never a tab char (the textarea sanitizes tabs out anyway).
- Column width default **65**, env `OKASHI_WIDTH` (valid integer in **[20,200]**, else default). Resolved once at startup.
- Smart quotes default **on**; `OKASHI_SMARTQUOTES` = `off`/`false`/`0` disables. Resolved once at startup. Curly glyphs by Unicode code point: `'`=U+2018, `'`=U+2019, `"`=U+201C, `"`=U+201D — write them as Go `\u` escapes so the bytes are unambiguous.
- Keys: `esc` toggles pane focus (and exits preview); `Tab`/`Shift+Tab` indent/outdent only when the editor pane is focused. The old `tab` focus-toggle is removed.
- Any buffer mutation done outside the normal forward path must set `m.dirty = true` and `m.lastEditAt = time.Now()` (autosave).
- Vendored textarea edits are additive and marked `okashi:editing`.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Vendored textarea editing helpers

**Files:**
- Modify: `internal/textarea/textarea.go`
- Test: `internal/textarea/editing_test.go`

**Interfaces:**
- Produces (all on `textarea.Model`): `Indent()`, `Outdent()`, `CurrentLine() string`, `AtLineEnd() bool`, `CharBeforeCursor() (rune, bool)`, `ClearLine()`.

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/editing_test.go`:

```go
package textarea

import "testing"

func TestIndentOutdent(t *testing.T) {
	m := New()
	m.SetValue("hello")
	m.SetCursor(5) // end of "hello"

	m.Indent()
	if got := m.Value(); got != "  hello" {
		t.Fatalf("after Indent: %q, want %q", got, "  hello")
	}
	if m.col != 7 {
		t.Fatalf("cursor col = %d, want 7", m.col)
	}

	m.Outdent()
	if got := m.Value(); got != "hello" {
		t.Fatalf("after Outdent: %q, want %q", got, "hello")
	}
	if m.col != 5 {
		t.Fatalf("cursor col = %d, want 5", m.col)
	}

	m.Outdent() // no leading spaces → no-op
	if got := m.Value(); got != "hello" {
		t.Fatalf("Outdent on unindented line changed it: %q", got)
	}
}

func TestLineHelpers(t *testing.T) {
	m := New()
	m.SetValue("- item")
	m.SetCursor(6)
	if m.CurrentLine() != "- item" {
		t.Fatalf("CurrentLine = %q", m.CurrentLine())
	}
	if !m.AtLineEnd() {
		t.Fatal("AtLineEnd should be true at col 6")
	}
	if r, ok := m.CharBeforeCursor(); !ok || r != 'm' {
		t.Fatalf("CharBeforeCursor = %q,%v want 'm',true", r, ok)
	}
	m.SetCursor(0)
	if _, ok := m.CharBeforeCursor(); ok {
		t.Fatal("CharBeforeCursor at col 0 should be ok=false")
	}
	if m.AtLineEnd() {
		t.Fatal("AtLineEnd should be false at col 0 of a non-empty line")
	}

	m.ClearLine()
	if m.Value() != "" || m.col != 0 {
		t.Fatalf("after ClearLine: %q col=%d", m.Value(), m.col)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestIndentOutdent|TestLineHelpers' -v 2>&1 | tail`
Expected: build error — `m.Indent undefined` etc.

- [ ] **Step 3: Implement the helpers**

Append to `internal/textarea/textarea.go`:

```go
// --- okashi:editing helpers ---

const okashiIndentUnit = "  "

// Indent inserts the indent unit (two spaces) at the start of the current line.
func (m *Model) Indent() {
	unit := []rune(okashiIndentUnit)
	m.value[m.row] = append(unit, m.value[m.row]...)
	m.SetCursor(m.col + len(unit))
}

// Outdent removes up to one indent unit of leading spaces from the current line.
func (m *Model) Outdent() {
	line := m.value[m.row]
	removed := 0
	for removed < len(okashiIndentUnit) && len(line) > 0 && line[0] == ' ' {
		line = line[1:]
		removed++
	}
	m.value[m.row] = line
	m.SetCursor(m.col - removed)
}

// CurrentLine returns the text of the line the cursor is on.
func (m Model) CurrentLine() string {
	return string(m.value[m.row])
}

// AtLineEnd reports whether the cursor is at the end of the current line.
func (m Model) AtLineEnd() bool {
	return m.col >= len(m.value[m.row])
}

// CharBeforeCursor returns the rune immediately left of the cursor, or
// (0, false) at the start of a line.
func (m Model) CharBeforeCursor() (rune, bool) {
	if m.col == 0 {
		return 0, false
	}
	return m.value[m.row][m.col-1], true
}

// ClearLine empties the current line and moves the cursor to its start.
func (m *Model) ClearLine() {
	m.value[m.row] = m.value[m.row][:0]
	m.SetCursor(0)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestIndentOutdent|TestLineHelpers' -v 2>&1 | tail`
Expected: both PASS.

- [ ] **Step 5: gofmt, full suite, commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/editing_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea
git commit -m "textarea: add editing helpers (indent/outdent/line/clear)"
```

---

## Task 2: Configurable column width

**Files:**
- Modify: `main.go` (const, resolver, model field, `initialModel`, `layout`, `togglePreview`)
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `const defaultColumnWidth = 65`; `func resolveColumnWidth() int`; `model.colWidth int`. `layout` and `togglePreview` use `m.colWidth` instead of the old `columnWidth` const.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestResolveColumnWidth(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	if resolveColumnWidth() != 65 {
		t.Fatalf("default should be 65, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "72")
	if resolveColumnWidth() != 72 {
		t.Fatalf("env 72 should win, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "5") // out of range
	if resolveColumnWidth() != 65 {
		t.Fatalf("out-of-range should fall back to 65, got %d", resolveColumnWidth())
	}
	t.Setenv("OKASHI_WIDTH", "abc")
	if resolveColumnWidth() != 65 {
		t.Fatalf("garbage should fall back to 65, got %d", resolveColumnWidth())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestResolveColumnWidth -v 2>&1 | tail`
Expected: build error — `resolveColumnWidth undefined`.

- [ ] **Step 3: Replace the const and add the resolver**

In `main.go`, replace:

```go
// columnWidth is the target writing measure. 80 chars, left-justified inside
// the column, with the whole column centered in whatever space is available.
const columnWidth = 80
```

with:

```go
// defaultColumnWidth is the target writing measure (the readable "ideal
// measure" is ~66). Override with OKASHI_WIDTH.
const defaultColumnWidth = 65

// resolveColumnWidth reads OKASHI_WIDTH (a column count in [20,200]); otherwise
// defaultColumnWidth.
func resolveColumnWidth() int {
	if v := os.Getenv("OKASHI_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 200 {
			return n
		}
	}
	return defaultColumnWidth
}
```

- [ ] **Step 4: Add the field and replace usages**

In the model struct, after `mdStyle string`:

```go
	colWidth int
```

In `initialModel`, the viewport init line `vp := viewport.New(columnWidth, 1)` becomes:

```go
	vp := viewport.New(defaultColumnWidth, 1) // real size set in layout()
```

and add to the returned `model{...}`:

```go
		colWidth: resolveColumnWidth(),
```

In `layout`, replace both `columnWidth` references:

```go
	colWidth := min(columnWidth, m.width-2)
	...
		colWidth = min(columnWidth, m.width-sidebarWidth-2)
```

with `m.colWidth` (rename the local to avoid shadowing — call it `cw`):

```go
	cw := min(m.colWidth, m.width-2)
	if m.sidebarVisible {
		m.files.height = bodyH - 2
		m.files.width = sidebarWidth - 2
		cw = min(m.colWidth, m.width-sidebarWidth-2)
	}
	m.editor.SetWidth(cw)
	m.editor.SetHeight(bodyH)
	m.preview.Width = cw
	m.preview.Height = bodyH
```

In `togglePreview`, replace `wrap = columnWidth` with `wrap = m.colWidth`.

- [ ] **Step 5: Run the test, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestResolveColumnWidth -v 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Configurable column width (default 65, OKASHI_WIDTH)"
```

---

## Task 3: esc pane-switch + Tab/Shift+Tab indent

**Files:**
- Modify: `main.go` (`Update` KeyMsg switch)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `m.editor.Indent()`/`Outdent()` (Task 1).
- Produces: `esc` toggles focus (exits preview); `tab`/`shift+tab` indent/outdent the editor; the old `tab` focus-toggle is gone.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestEscTogglesFocusAndTabIndents(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("hi")
	m.editor.SetCursor(2)

	// Tab indents the editor.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	if m.editor.Value() != "  hi" {
		t.Fatalf("tab should indent, got %q", m.editor.Value())
	}
	if !m.dirty {
		t.Fatal("indent should mark dirty")
	}

	// Shift+Tab outdents.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = nm.(model)
	if m.editor.Value() != "hi" {
		t.Fatalf("shift+tab should outdent, got %q", m.editor.Value())
	}

	// esc moves focus to the sidebar.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.focus != focusSidebar {
		t.Fatal("esc should toggle focus to the sidebar")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestEscTogglesFocusAndTabIndents -v 2>&1 | tail`
Expected: FAIL — tab currently toggles focus (no indent); esc unhandled.

- [ ] **Step 3: Replace the `tab` case, add `esc`/`shift+tab`**

In `Update`'s `tea.KeyMsg` switch, replace the existing focus-toggle case:

```go
		case "tab":
			if m.sidebarVisible {
				if m.focus == focusSidebar {
					m.focus = focusEditor
					m.editor.Focus()
				} else {
					m.focus = focusSidebar
					m.editor.Blur()
				}
			}
			return m, nil
```

with these three cases:

```go
		case "esc":
			if m.previewing {
				m.togglePreview() // exit preview
				return m, nil
			}
			if m.sidebarVisible {
				if m.focus == focusSidebar {
					m.focus = focusEditor
					m.editor.Focus()
				} else {
					m.focus = focusSidebar
					m.editor.Blur()
				}
			}
			return m, nil
		case "tab":
			if m.focus == focusEditor {
				m.editor.Indent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
		case "shift+tab":
			if m.focus == focusEditor {
				m.editor.Outdent()
				m.dirty = true
				m.lastEditAt = time.Now()
			}
			return m, nil
```

- [ ] **Step 4: Run the test, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestEscTogglesFocusAndTabIndents -v 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "esc pane-switch; Tab/Shift+Tab indent/outdent"
```

---

## Task 4: Smart curly quotes

**Files:**
- Modify: `main.go` (resolver, helper, model field, `initialModel`, editor-routing branch)
- Test: `smoke_test.go`

**Interfaces:**
- Produces: `func resolveSmartQuotes() bool`; `func smartQuote(prev rune, hasPrev bool, q rune) rune`; `model.smartQuotes bool`. The editor-routing branch converts typed `'`/`"` when `smartQuotes` is on.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestSmartQuoteHelper(t *testing.T) {
	cases := []struct {
		prev    rune
		hasPrev bool
		q       rune
		want    rune
	}{
		{0, false, '\'', '‘'},   // start of line → opening '
		{' ', true, '"', '“'},   // after space → opening "
		{'n', true, '\'', '’'},  // contraction don't → closing '
		{'d', true, '"', '”'},   // after letter → closing "
		{'(', true, '\'', '‘'},  // after ( → opening
	}
	for _, c := range cases {
		if got := smartQuote(c.prev, c.hasPrev, c.q); got != c.want {
			t.Fatalf("smartQuote(%q,%v,%q) = %q, want %q", c.prev, c.hasPrev, c.q, got, c.want)
		}
	}
}

func TestResolveSmartQuotes(t *testing.T) {
	t.Setenv("OKASHI_SMARTQUOTES", "")
	if !resolveSmartQuotes() {
		t.Fatal("default should be on")
	}
	t.Setenv("OKASHI_SMARTQUOTES", "off")
	if resolveSmartQuotes() {
		t.Fatal("off should disable")
	}
}

func TestEditorSmartQuoteInsert(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true
	m.editor.SetValue("")
	m.editor.SetCursor(0)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'"'}})
	m = nm.(model)
	if m.editor.Value() != "“" {
		t.Fatalf("typing \" at start should insert a left double curly quote, got %q", m.editor.Value())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestSmartQuote|TestResolveSmartQuotes|TestEditorSmartQuote' -v 2>&1 | tail`
Expected: build error — `smartQuote undefined`.

- [ ] **Step 3: Add resolver, helper, field**

In `main.go`, near `resolveColumnWidth`, add:

```go
// resolveSmartQuotes reads OKASHI_SMARTQUOTES; off/false/0 disable, default on.
func resolveSmartQuotes() bool {
	switch strings.ToLower(os.Getenv("OKASHI_SMARTQUOTES")) {
	case "off", "false", "0":
		return false
	}
	return true
}

// smartQuote returns the curly form of a straight quote. It's an opening quote
// at the start of a line or after whitespace / an opening bracket; otherwise
// closing (which also yields the right apostrophe in contractions).
func smartQuote(prev rune, hasPrev bool, q rune) rune {
	opening := !hasPrev || prev == ' ' || prev == '\t' || prev == '\n' ||
		prev == '(' || prev == '[' || prev == '{'
	switch q {
	case '\'':
		if opening {
			return '‘'
		}
		return '’'
	case '"':
		if opening {
			return '“'
		}
		return '”'
	}
	return q
}
```

In the model struct, after `colWidth int`:

```go
	smartQuotes bool
```

In `initialModel`'s returned literal:

```go
		smartQuotes: resolveSmartQuotes(),
```

- [ ] **Step 4: Intercept quotes in the editor-routing branch**

In `Update`, the editor-routing `else` branch currently is:

```go
	} else {
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
		}
	}
```

Replace it with (adds the quote interception before the forward):

```go
	} else {
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '\'' || km.Runes[0] == '"') {
			prev, hasPrev := m.editor.CharBeforeCursor()
			m.editor.InsertString(string(smartQuote(prev, hasPrev, km.Runes[0])))
			m.dirty = true
			m.lastEditAt = time.Now()
			return m, nil
		}
		before := m.editor.Value()
		m.editor, cmd = m.editor.Update(msg)
		cmds = append(cmds, cmd)
		if m.editor.Value() != before {
			m.dirty = true
			m.lastEditAt = time.Now()
		}
	}
```

- [ ] **Step 5: Run the tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSmartQuote|TestResolveSmartQuotes|TestEditorSmartQuote' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Smart curly quotes (default on, OKASHI_SMARTQUOTES=off)"
```

---

## Task 5: Auto-continue Markdown lists

**Files:**
- Modify: `main.go` (helper, editor-routing branch)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `m.editor.AtLineEnd()`, `CurrentLine()`, `InsertString()`, `ClearLine()` (Task 1).
- Produces: `func listContinuation(line string) (prefix string, clear bool, ok bool)`; Enter auto-continues/ends a list when the editor is focused and the cursor is at line end.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestListContinuation(t *testing.T) {
	cases := []struct {
		line   string
		prefix string
		clear  bool
		ok     bool
	}{
		{"- item", "- ", false, true},
		{"  - nested", "  - ", false, true},
		{"3. third", "4. ", false, true},
		{"- ", "", true, true},   // empty bullet → end list
		{"1. ", "", true, true},  // empty number → end list
		{"plain text", "", false, false},
	}
	for _, c := range cases {
		p, cl, ok := listContinuation(c.line)
		if p != c.prefix || cl != c.clear || ok != c.ok {
			t.Fatalf("listContinuation(%q) = (%q,%v,%v), want (%q,%v,%v)",
				c.line, p, cl, ok, c.prefix, c.clear, c.ok)
		}
	}
}

func TestEnterContinuesList(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.editor.SetValue("- one")
	m.editor.SetCursor(5) // end of line

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if m.editor.Value() != "- one\n- " {
		t.Fatalf("Enter should continue the list, got %q", m.editor.Value())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestListContinuation|TestEnterContinuesList' -v 2>&1 | tail`
Expected: build error — `listContinuation undefined`.

- [ ] **Step 3: Add the helper**

In `main.go`, add (and ensure `regexp` is imported):

```go
var listItemRe = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+(.*)$`)

// listContinuation inspects a list line for Enter handling. ok=false means it's
// not a list item (normal Enter). clear=true means the item is empty → end the
// list. Otherwise prefix is inserted after a newline to continue the list.
func listContinuation(line string) (prefix string, clear bool, ok bool) {
	mtch := listItemRe.FindStringSubmatch(line)
	if mtch == nil {
		return "", false, false
	}
	indent, marker, content := mtch[1], mtch[2], mtch[3]
	if strings.TrimSpace(content) == "" {
		return "", true, true
	}
	next := marker
	if n, err := strconv.Atoi(strings.TrimSuffix(marker, ".")); err == nil {
		next = strconv.Itoa(n+1) + "."
	}
	return indent + next + " ", false, true
}
```

- [ ] **Step 4: Intercept Enter in the editor-routing branch**

In the editor-routing `else` branch, add this BEFORE the smart-quote check (so it's the first interception):

```go
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEnter && m.editor.AtLineEnd() {
			if prefix, clear, ok := listContinuation(m.editor.CurrentLine()); ok {
				if clear {
					m.editor.ClearLine()
				} else {
					m.editor.InsertString("\n" + prefix)
				}
				m.dirty = true
				m.lastEditAt = time.Now()
				return m, nil
			}
		}
```

- [ ] **Step 5: Run the tests, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run 'TestListContinuation|TestEnterContinuesList' -v 2>&1 | tail -6
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Auto-continue Markdown lists on Enter"
```

---

## Task 6: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the keys table**

Replace the `tab` row in `README.md`'s keys table:

```
| `tab`    | Switch focus between sidebar and editor |
```

with:

```
| `esc`    | Switch focus between sidebar and editor (exit preview) |
| `tab`    | Indent (Shift+Tab to outdent) in the editor             |
```

- [ ] **Step 2: Add a "Writing ergonomics" section**

After the "Markdown preview" section, add:

```markdown
## Writing ergonomics

- **Tab / Shift+Tab** indent and outdent (two spaces).
- **Enter** on a Markdown list line (`- `, `* `, `+ `, `1.`) continues the list;
  Enter on an empty item ends it.
- **Smart quotes** turn `'`/`"` into curly quotes as you type (on by default;
  set `OKASHI_SMARTQUOTES=off` for code-heavy writing).
- **Column width** defaults to 65; set `OKASHI_WIDTH=<n>` (20–200) to taste.
```

- [ ] **Step 3: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "Docs: writing ergonomics (indent, lists, smart quotes, width)"
```

- [ ] **Step 5: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` → open a file, then: Tab/Shift+Tab indent/outdent; type a `- ` list and press Enter (continues; empty item ends it); type quotes (curly); `esc` toggles to the sidebar and back; the column looks narrower (65). Try `OKASHI_WIDTH=80` and `OKASHI_SMARTQUOTES=off`. (Editing feel, wrap interaction, and quote behavior are only fully confirmed here.)

---

## Self-Review

**Spec coverage (Plan A scope — spec Sections 1 + 2):**
- esc pane-switch (+ exit preview) → Task 3. Tab/Shift+Tab indent/outdent → Tasks 1 + 3. Auto-continue lists → Tasks 1 + 5. Smart quotes → Task 4. Configurable width → Task 2. dirty-tracking on the special handlers → Tasks 3/4/5 (each sets dirty+lastEditAt). Docs → Task 6.
- **Deferred:** spec Section 3 (file pane breadcrumb/confinement) is Plan B.

**Placeholder scan:** none — every code step shows full code; curly quotes use `\u` escapes (unambiguous bytes).

**Type consistency:** `Indent`/`Outdent`/`CurrentLine`/`AtLineEnd`/`CharBeforeCursor`/`ClearLine` (Task 1) are consumed in Tasks 3/4/5. `resolveColumnWidth`/`defaultColumnWidth`/`m.colWidth` (Task 2), `resolveSmartQuotes`/`smartQuote`/`m.smartQuotes` (Task 4), `listContinuation` (Task 5) all consistent. The editor-routing branch is edited by Task 4 (quotes) then Task 5 (Enter, inserted before the quote check) — both land in the same `else` block; the Enter interception is first.

**Ordering note:** Task 5 inserts its Enter-handling block *before* the smart-quote block added in Task 4. If implemented out of order, Task 5's "before the smart-quote check" instruction still applies once Task 4's block exists.
