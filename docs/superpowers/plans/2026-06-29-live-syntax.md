# Live Syntax Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

> **BYTE→RUNE TRAP:** `regexp` returns BYTE offsets; `textarea.Decoration` needs RUNE offsets (the editor indexes runes). okashi prose is full of multibyte chars (curly quotes, em-dashes), so you MUST convert byte→rune offsets, and TEST with a multibyte line. (This is the same class of bug as the dim-span rune/byte fix.)

**Goal:** Add live markdown syntax highlighting in the editor (a `syntaxDecorator` consuming the existing decoration hook), composing with spellcheck, toggled by the Analysis-tab Syntax checkbox.

**Architecture:** `syntaxDecorator(line)` tokenizes markdown per line into styled rune-range `Decoration`s (occupied-range overlap rule, bold before italic). `applyDecorator()` composes spell + syntax (spell first → wins overlaps). The Syntax checkbox toggles it.

**Tech Stack:** Go, `regexp`, lipgloss, `unicode/utf8`, the `textarea.Decoration` hook (cycle 3-1).

**Design spec:** `docs/superpowers/specs/2026-06-29-live-syntax-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Decoration offsets are RUNE indices; convert regex byte offsets → rune via a helper; test multibyte.
- Token set: heading, bold (`**`/`__`), italic (`*`/`_`), inline code, link, list marker. Per-line only (no fenced-block state). Overlap: build in priority order, skip already-covered runes.
- Composition (`applyDecorator`): spellcheck spans FIRST (it wins overlaps).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: syntaxDecorator (`syntax.go`)

**Files:** Create `syntax.go`, `syntax_test.go`

**Interfaces:**
- Consumes: `textarea.Decoration`, `accent`/`subtle`, `listItemRe` (main.go).
- Produces: `syntaxDecorator(line string) []textarea.Decoration`.

- [ ] **Step 1: Write the failing test** — create `syntax_test.go`:

```go
package main

import "testing"

// spanOf returns the first decoration whose rune-range matches [start,end), or -1,-1.
func hasSpan(decos []textarea.Decoration, start, end int) bool {
	for _, d := range decos {
		if d.Start == start && d.End == end {
			return true
		}
	}
	return false
}

func TestSyntaxDecorator(t *testing.T) {
	// Heading: text "Title" at runes 2..7.
	if d := syntaxDecorator("# Title"); !hasSpan(d, 2, 7) {
		t.Fatalf("heading text should be a span [2,7): %+v", d)
	}
	// Bold over the whole **bold** (runes 0..8), not italicized.
	bd := syntaxDecorator("**bold** x")
	if !hasSpan(bd, 0, 8) {
		t.Fatalf("bold span [0,8) missing: %+v", bd)
	}
	// Italic *it* → [0,4).
	if d := syntaxDecorator("*it* y"); !hasSpan(d, 0, 4) {
		t.Fatalf("italic span [0,4) missing: %+v", d)
	}
	// Inline code.
	if d := syntaxDecorator("a `c` b"); !hasSpan(d, 2, 5) {
		t.Fatalf("code span [2,5) missing: %+v", d)
	}
	// Plain prose → nothing.
	if d := syntaxDecorator("just some plain words"); len(d) != 0 {
		t.Fatalf("plain line should have no spans: %+v", d)
	}
}

func TestSyntaxMultibyteOffsets(t *testing.T) {
	// An em-dash (multibyte) before bold: rune offsets must not be byte offsets.
	// "x — **b**"  → runes: x(0) space(1) —(2) space(3) *(4)*(5) b(6) *(7)*(8)
	d := syntaxDecorator("x — **b**")
	if !hasSpan(d, 4, 9) {
		t.Fatalf("bold span after em-dash should be rune [4,9), got %+v", d)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestSyntax 2>&1 | tail` → `syntaxDecorator` undefined.

- [ ] **Step 3: Create `syntax.go`:**

```go
package main

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

var (
	synHeadingStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	synMarkerStyle  = lipgloss.NewStyle().Foreground(subtle)
	synBoldStyle    = lipgloss.NewStyle().Bold(true)
	synItalicStyle  = lipgloss.NewStyle().Italic(true)
	synCodeStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b"))
	synLinkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd"))
)

var (
	synHeadingRe = regexp.MustCompile(`^#{1,6}\s+(\S.*)$`)
	synBoldRe    = regexp.MustCompile(`\*\*[^*]+\*\*|__[^_]+__`)
	synItalicRe  = regexp.MustCompile(`\*[^*]+\*|_[^_]+_`)
	synCodeRe    = regexp.MustCompile("`[^`]+`")
	synLinkRe    = regexp.MustCompile(`\[[^\]]+\]\([^)]+\)`)
)

// syntaxDecorator styles markdown tokens on one line. Rune-range decorations,
// built in priority order; a rune already covered is not styled again.
func syntaxDecorator(line string) []textarea.Decoration {
	runes := []rune(line)
	occupied := make([]bool, len(runes))
	var decos []textarea.Decoration

	// byteToRune maps a byte offset in `line` to its rune index.
	byteToRune := func(b int) int {
		return len([]rune(line[:b]))
	}
	add := func(rs, re int, style lipgloss.Style) {
		if rs < 0 || re > len(runes) || rs >= re {
			return
		}
		for i := rs; i < re; i++ {
			if occupied[i] {
				return
			}
		}
		for i := rs; i < re; i++ {
			occupied[i] = true
		}
		decos = append(decos, textarea.Decoration{Start: rs, End: re, Style: style})
	}
	addByte := func(bs, be int, style lipgloss.Style) { add(byteToRune(bs), byteToRune(be), style) }

	// Heading: style the text (submatch 1).
	if m := synHeadingRe.FindStringSubmatchIndex(line); m != nil {
		addByte(m[2], m[3], synHeadingStyle)
	}
	// List marker (leading marker run).
	if m := listItemRe.FindStringSubmatchIndex(line); m != nil {
		addByte(m[4], m[5], synMarkerStyle) // group 2 = the marker
	}
	for _, m := range synCodeRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synCodeStyle)
	}
	for _, m := range synLinkRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synLinkStyle)
	}
	for _, m := range synBoldRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synBoldStyle)
	}
	for _, m := range synItalicRe.FindAllStringIndex(line, -1) {
		addByte(m[0], m[1], synItalicStyle)
	}
	return decos
}
```

(Note: `listItemRe` group indices — `FindStringSubmatchIndex` returns pairs; group 2 is at `m[4]:m[5]`. The plan's `synHeadingRe` captures the text in group 1 → `m[2]:m[3]`. The implementer should verify the group offsets with a quick check and adjust if the existing `listItemRe` shape differs.)

- [ ] **Step 4: Run tests; gofmt; commit:**

```bash
/opt/homebrew/bin/go test . -run TestSyntax -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w syntax.go syntax_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add syntax.go syntax_test.go
git commit -m "syntax: per-line markdown syntaxDecorator (headings/emphasis/code/links/markers)"
```

---

## Task 2: Compose decorators + Syntax toggle (`main.go`, `inspector.go`)

**Files:** Modify `main.go` (`applyDecorator`, Analysis click), `inspector.go` (un-dim Syntax row); Test `smoke_test.go`

**Interfaces:** Consumes `syntaxDecorator` (Task 1), `m.analysis`, `inspectorAnalysisRowAtY`, `textarea.Decoration`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestSyntaxToggleComposesWithSpell(t *testing.T) {
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

	// Click the Syntax row (spellRowY+1) → syntax on, Decorator set.
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: spellRowY + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.syntax {
		t.Fatal("clicking the Syntax row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("syntax on should set the editor Decorator")
	}
	// Now enable spell too → combined decorator, spell spans first.
	m.analysis.spell = true
	m.applyDecorator()
	got := m.editor.Decorator("teh **word**")
	// spell flags "teh" (a span starting at 0); a syntax bold span starts at 4.
	if len(got) < 2 || got[0].Start != 0 {
		t.Fatalf("combined decorator should put the spell span (teh @0) first: %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestSyntaxToggle 2>&1 | tail` → syntax row inert (no toggle).

- [ ] **Step 3: Compose in `applyDecorator` (`main.go`)** — replace the helper body:

```go
func (m *model) applyDecorator() {
	switch {
	case m.analysis.spell && m.analysis.syntax:
		m.editor.Decorator = func(line string) []textarea.Decoration {
			return append(spellDecorator(line), syntaxDecorator(line)...)
		}
	case m.analysis.spell:
		m.editor.Decorator = spellDecorator
	case m.analysis.syntax:
		m.editor.Decorator = syntaxDecorator
	default:
		m.editor.Decorator = nil
	}
}
```

- [ ] **Step 4: Wire the Syntax checkbox click (`main.go`)** — in the Analysis-tab click block, handle row 1:

```go
				if row == 0 {
					m.analysis.spell = !m.analysis.spell
					m.applyDecorator()
				} else if row == 1 {
					m.analysis.syntax = !m.analysis.syntax
					m.applyDecorator()
				}
```

(Replace the existing "row 1 (Syntax) is wired next cycle" comment/branch.)

- [ ] **Step 5: Un-dim the Syntax row (`inspector.go`)** — in the `tabAnalysis` body, render the Syntax line like the Spellcheck line (a normal checkbox, not `subtle`):

```go
		b.WriteString(checkbox(analysis.spell) + "Spellcheck\n")
		b.WriteString(checkbox(analysis.syntax) + "Syntax")
```

- [ ] **Step 6: Run tests; gofmt; build; commit:**

```bash
/opt/homebrew/bin/go test . -run 'TestSyntaxToggle|TestSpellcheckToggle|TestInspectorAnalysis' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go inspector.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go inspector.go smoke_test.go
git commit -m "syntax: compose with spellcheck + Analysis-tab Syntax checkbox toggle"
```

---

## Self-Review

**Spec coverage:** token set (heading/bold/italic/code/link/marker) + rune offsets + occupied-range overlap + byte→rune → Task 1; compose-spell-first + Syntax checkbox click + un-dim row → Task 2.

**Placeholder scan:** none — full code. The `listItemRe`/`synHeadingRe` group-index note tells the implementer to verify the submatch offsets (a correctness check, not a placeholder).

**Type consistency:** `syntaxDecorator(string) []textarea.Decoration` (Task 1) consumed by `applyDecorator` (Task 2); `m.analysis.syntax`, `inspectorAnalysisRowAtY`, `spellRowY`, `checkbox` reused from cycle 3-1; the combined decorator returns spell spans first (composition contract).

**Risk note (executor):** the byte→rune conversion is the trap — `addByte` maps regex byte offsets to rune indices via `len([]rune(line[:b]))`; the multibyte test (`TestSyntaxMultibyteOffsets`) is the gate. The occupied-range rule makes bold-before-italic deterministic. Spellcheck spans must precede syntax spans in the combined slice (Task 2 test asserts it).
