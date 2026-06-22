# Focus Dimming (sentence-level, iA-style) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dim everything except the sentence under the cursor (part of "focus mode" with typewriter), toggleable.

**Architecture:** Pure sentence-span + run-splitting helpers in the vendored `internal/textarea` (fully unit-tested without color), then a per-character dim patch woven into the textarea render loop. okashi sets `editor.Dim` (= `typewriter && dimEnabled`) and the dim color, with a toggle key.

**Tech Stack:** Go, Bubble Tea v1.1.0, Lipgloss, vendored textarea.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go` (not on PATH).
- Dimming active when `typewriter && dimEnabled`; `dimEnabled` defaults **true**. Toggle key `ctrl+d` (app-intercepted; this overrides the editor's rarely-used delete-forward — document it). Status shows "dim on"/"dim off".
- Current sentence span: from after the previous sentence terminator (`.`/`!`/`?` + whitespace) — or paragraph start — to the next terminator (inclusive). A blank line (`\n\n`) is a hard boundary both ways; a single `\n` is NOT (a sentence may span soft/hard line breaks).
- Out-of-span characters render with the dim style (`subtle` foreground); in-span render normally. The cursor's character is always in-span.
- Vendored-textarea edits are additive and marked `okashi:dim`.
- Wrapped pieces reconstruct the source line (verified) → offset tracking through the render loop is valid; the only discrepancy is a synthetic trailing space past real content.
- Tests touching `writingDir()` set `t.Setenv("OKASHI_DIR", t.TempDir())`.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Pure sentence-span + run-splitting (`internal/textarea/dim.go`)

**Files:**
- Create: `internal/textarea/dim.go`, `internal/textarea/dim_test.go`

**Interfaces:**
- Produces (package `textarea`):
  - `func currentSentenceSpan(text string, cursorOffset int) (start, end int)`
  - `type dimRun struct { text string; dim bool }`
  - `func splitDimRuns(seg []rune, absStart, span0, span1 int) []dimRun`

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/dim_test.go`:

```go
package textarea

import "testing"

func TestCurrentSentenceSpan(t *testing.T) {
	cases := []struct {
		name             string
		text             string
		cursor           int
		wantStart, wantEnd int
	}{
		{"first sentence", "Hello world. Goodbye now.", 2, 0, 12},
		{"second sentence", "Hello world. Goodbye now.", 16, 13, 25},
		{"on terminator", "Hello world. Goodbye now.", 11, 0, 12},
		{"paragraph boundary", "One.\n\nTwo here.", 8, 6, 14},
		{"spans single newline", "A long\nsentence done.", 2, 0, 21},
		{"empty", "", 0, 0, 0},
		{"no terminator", "just some words", 5, 0, 15},
	}
	for _, c := range cases {
		gs, ge := currentSentenceSpan(c.text, c.cursor)
		if gs != c.wantStart || ge != c.wantEnd {
			t.Errorf("%s: currentSentenceSpan(%q,%d) = (%d,%d), want (%d,%d)",
				c.name, c.text, c.cursor, gs, ge, c.wantStart, c.wantEnd)
		}
	}
}

func TestSplitDimRuns(t *testing.T) {
	// seg "AB CD" starting at absolute offset 10; span [12,14) covers " C".
	runs := splitDimRuns([]rune("AB CD"), 10, 12, 14)
	// offsets: A=10 B=11 (dim) space=12 C=13 (bright) D=14 (dim)
	want := []dimRun{{"AB", true}, {" C", false}, {"D", true}}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs, want %d: %+v", len(runs), len(want), runs)
	}
	for i := range want {
		if runs[i] != want[i] {
			t.Fatalf("run %d = %+v, want %+v", i, runs[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestCurrentSentenceSpan|TestSplitDimRuns' -v 2>&1 | tail`
Expected: build error — undefined.

- [ ] **Step 3: Implement `dim.go`**

```go
package textarea

// currentSentenceSpan returns the [start,end) rune range of the sentence under
// cursorOffset. A sentence runs from after the previous terminator (.!?) +
// whitespace (or paragraph start) to the next terminator (inclusive). A blank
// line ("\n\n") is a hard boundary on both sides.
func currentSentenceSpan(text string, cursorOffset int) (int, int) {
	r := []rune(text)
	n := len(r)
	if n == 0 {
		return 0, 0
	}
	if cursorOffset < 0 {
		cursorOffset = 0
	}
	if cursorOffset > n {
		cursorOffset = n
	}

	isTerm := func(c rune) bool { return c == '.' || c == '!' || c == '?' }
	isWS := func(c rune) bool { return c == ' ' || c == '\t' || c == '\n' }
	paraBreak := func(i int) bool {
		return r[i] == '\n' && ((i > 0 && r[i-1] == '\n') || (i+1 < n && r[i+1] == '\n'))
	}

	start := 0
	for i := cursorOffset - 1; i >= 1; i-- {
		if paraBreak(i) {
			start = i + 1
			break
		}
		if isTerm(r[i-1]) && isWS(r[i]) {
			j := i
			for j < n && isWS(r[j]) {
				j++
			}
			start = j
			break
		}
	}

	end := n
	for i := cursorOffset; i < n; i++ {
		if paraBreak(i) {
			end = i
			break
		}
		if isTerm(r[i]) {
			end = i + 1
			break
		}
	}
	if start > end {
		start = end
	}
	return start, end
}

// dimRun is a maximal run of characters that are all in- or out-of-span.
type dimRun struct {
	text string
	dim  bool
}

// splitDimRuns groups seg (whose first rune is at absolute offset absStart) into
// runs marked dim when outside [span0,span1).
func splitDimRuns(seg []rune, absStart, span0, span1 int) []dimRun {
	var runs []dimRun
	i := 0
	for i < len(seg) {
		off := absStart + i
		dim := off < span0 || off >= span1
		j := i + 1
		for j < len(seg) {
			o := absStart + j
			if (o < span0 || o >= span1) != dim {
				break
			}
			j++
		}
		runs = append(runs, dimRun{text: string(seg[i:j]), dim: dim})
		i = j
	}
	return runs
}
```

- [ ] **Step 4: Run the tests; iterate `currentSentenceSpan` until the table passes**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestCurrentSentenceSpan|TestSplitDimRuns' -v 2>&1 | tail`
Expected: PASS. (If a span case is off, adjust the boundary logic — the test table is the spec.)

- [ ] **Step 5: gofmt, commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/dim.go internal/textarea/dim_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea/dim.go internal/textarea/dim_test.go
git commit -m "textarea: pure sentence-span + dim-run helpers"
```

---

## Task 2: Dim render patch in the vendored textarea

**Files:**
- Modify: `internal/textarea/textarea.go`
- Test: `internal/textarea/dim_test.go`

**Interfaces:**
- Consumes: `currentSentenceSpan`, `splitDimRuns` (Task 1).
- Produces: exported `Dim bool`, `DimStyle lipgloss.Style`; a `cursorByteOffset()`-style rune offset; `renderSeg(seg []rune, absStart, span0, span1 int, style lipgloss.Style) string`. When `Dim`, the render styles out-of-span characters with `DimStyle`.

- [ ] **Step 1: Write the failing integration test**

Add to `internal/textarea/dim_test.go` (add imports `strings`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/x/exp/teatest`? no — use lipgloss only):

```go
import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDimAppliesOutOfSpan(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(2) // force ANSI256 so styles emit codes in tests
	defer lipgloss.SetColorProfile(old)

	ta := New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(60)
	ta.SetHeight(5)
	ta.DimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ta.SetValue("First one. Second two.")
	ta.SetCursor(2) // inside "First one."

	dimSeq := ta.DimStyle.Render("x")
	dimCode := dimSeq[:strings.Index(dimSeq, "x")] // the opening SGR of DimStyle

	ta.Dim = false
	if strings.Contains(ta.View(), dimCode) {
		t.Fatal("no dim styling expected when Dim is off")
	}
	ta.Dim = true
	if !strings.Contains(ta.View(), dimCode) {
		t.Fatal("expected the out-of-span text to carry the dim style when Dim is on")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run TestDimAppliesOutOfSpan -v 2>&1 | tail`
Expected: build error — `ta.Dim`/`ta.DimStyle` undefined.

- [ ] **Step 3: Add fields, the rune-offset helper, and `renderSeg`**

In `internal/textarea/textarea.go`, add to the `Model` struct (near `Typewriter bool`):

```go
	// okashi:dim — when Dim is true, characters outside the cursor's sentence
	// render with DimStyle.
	Dim      bool
	DimStyle lipgloss.Style
```

Add these methods (near `renderViewport`, marked `okashi:dim`):

```go
// cursorRuneOffset returns the cursor's absolute rune offset in Value().
func (m Model) cursorRuneOffset() int {
	off := 0
	for i := 0; i < m.row; i++ {
		off += len(m.value[i]) + 1 // +1 for the joining newline
	}
	return off + m.col
}

// renderSeg renders seg (first rune at absolute offset absStart). When Dim is
// off it's just style.Render; when on, out-of-[span0,span1) runs use DimStyle.
func (m Model) renderSeg(seg []rune, absStart, span0, span1 int, style lipgloss.Style) string {
	if !m.Dim {
		return style.Render(string(seg))
	}
	var b strings.Builder
	for _, run := range splitDimRuns(seg, absStart, span0, span1) {
		if run.dim {
			b.WriteString(m.DimStyle.Render(run.text))
		} else {
			b.WriteString(style.Render(run.text))
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Compute the span once in `View` and track per-piece offsets**

In `View`, just after the variable block (where `lineInfo = m.LineInfo()` is set), add:

```go
	var dimSpan0, dimSpan1 int
	if m.Dim {
		dimSpan0, dimSpan1 = currentSentenceSpan(m.Value(), m.cursorRuneOffset())
	}
	lineOffset := 0
```

In the `for l, line := range m.value {` loop, immediately inside the loop add a per-line piece-offset start, and at the END of the loop body advance `lineOffset`:

```go
	for l, line := range m.value {
		wrappedLines := m.memoizedWrap(line, m.width)
		pieceStart := 0 // rune offset of the current wrapped piece within this line
		// ... existing style selection ...
```

and at the very end of the `for l` loop body (after the inner piece loop closes), add:

```go
		lineOffset += len(line) + 1
	}
```

Inside the `for wl, wrappedLine := range wrappedLines {` loop, capture the piece length BEFORE the `TrimSuffix` line, and advance `pieceStart` after rendering:

```go
		for wl, wrappedLine := range wrappedLines {
			pieceLen := len(wrappedLine) // source rune count of this piece
			// ... existing prompt / line-number / strwidth / TrimSuffix code ...
```

Replace the three `style.Render(string(...))` content sites with `m.renderSeg(...)`, computing each segment's absolute start as `lineOffset + pieceStart + <local column>`:

- `s.WriteString(style.Render(string(wrappedLine[:lineInfo.ColumnOffset])))` →
  `s.WriteString(m.renderSeg(wrappedLine[:lineInfo.ColumnOffset], lineOffset+pieceStart, dimSpan0, dimSpan1, style))`
- `s.WriteString(style.Render(string(wrappedLine[lineInfo.ColumnOffset+1:])))` →
  `s.WriteString(m.renderSeg(wrappedLine[lineInfo.ColumnOffset+1:], lineOffset+pieceStart+lineInfo.ColumnOffset+1, dimSpan0, dimSpan1, style))`
- `s.WriteString(style.Render(string(wrappedLine)))` →
  `s.WriteString(m.renderSeg(wrappedLine, lineOffset+pieceStart, dimSpan0, dimSpan1, style))`

(Leave the cursor character (`m.Cursor.View()`) and the trailing `padding` render as-is — the cursor is always in-span and padding is whitespace.)

At the END of the `for wl` loop body (after `newLines++`), advance the piece offset:

```go
			pieceStart += pieceLen
		}
```

- [ ] **Step 5: Run the integration test; full suite**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run TestDim -v 2>&1 | tail`
Expected: PASS. Then `/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./...` clean.

- [ ] **Step 6: gofmt, commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/dim_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea
git commit -m "textarea: sentence-level dim render patch"
```

---

## Task 3: Wire dimming into okashi (toggle + focus mode)

**Files:**
- Modify: `main.go`
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `editor.Dim`, `editor.DimStyle` (Task 2).
- Produces: `model.dimEnabled bool` (default true); a `syncDim()` that sets `m.editor.Dim = m.typewriter && m.dimEnabled` and `m.editor.DimStyle`; `ctrl+d` toggles `dimEnabled`; `ctrl+t` also re-syncs.

- [ ] **Step 1: Write the failing test**

Add to `smoke_test.go`:

```go
func TestDimFollowsTypewriterAndToggle(t *testing.T) {
	m := initialModel()
	// default: typewriter on, dimEnabled on → editor.Dim on
	if !m.dimEnabled || !m.editor.Dim {
		t.Fatal("dim should default on (typewriter on, dimEnabled on)")
	}
	// ctrl+d turns dimming off but keeps typewriter
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = nm.(model)
	if m.dimEnabled || m.editor.Dim {
		t.Fatal("ctrl+d should turn dimming off")
	}
	if !m.typewriter {
		t.Fatal("ctrl+d must not affect typewriter")
	}
	// ctrl+t off → dim off regardless of dimEnabled
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD}) // dim back on
	m = nm.(model)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlT}) // typewriter off
	m = nm.(model)
	if m.editor.Dim {
		t.Fatal("editor.Dim must be off when typewriter is off")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestDimFollowsTypewriterAndToggle -v 2>&1 | tail`
Expected: build error — `m.dimEnabled` undefined.

- [ ] **Step 3: Add the field, `syncDim`, and key handling**

In the model struct, after `typewriter bool`:

```go
	dimEnabled bool
```

In `initialModel`, after `ta.Typewriter = true`:

```go
	ta.Dim = true
	ta.DimStyle = lipgloss.NewStyle().Foreground(subtle)
```

and add `dimEnabled: true,` to the returned `model{...}` literal. Update the status hint to include `ctrl+d dim`.

Add a helper:

```go
// syncDim keeps the editor's dim state in step with focus mode: dim only when
// typewriter AND dimEnabled.
func (m *model) syncDim() {
	m.editor.Dim = m.typewriter && m.dimEnabled
}
```

In `Update`'s `tea.KeyMsg` switch, update the `ctrl+t` case to re-sync, and add `ctrl+d`:

```go
		case "ctrl+t":
			m.typewriter = !m.typewriter
			m.editor.Typewriter = m.typewriter
			m.syncDim()
			if m.typewriter {
				m.status = "typewriter on"
			} else {
				m.status = "typewriter off"
			}
			return m, nil
		case "ctrl+d":
			m.dimEnabled = !m.dimEnabled
			m.syncDim()
			if m.editor.Dim {
				m.status = "dim on"
			} else {
				m.status = "dim off"
			}
			return m, nil
```

- [ ] **Step 4: Run the test, full suite, commit**

```bash
/opt/homebrew/bin/go test . -run TestDimFollowsTypewriterAndToggle -v 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add main.go smoke_test.go
git commit -m "Wire focus dimming (ctrl+d toggle, follows typewriter)"
```

---

## Task 4: Docs + final verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the docs**

Add a `ctrl+d` row to the keys table:

```
| `ctrl+d` | Toggle focus dimming (dim all but the current sentence) |
```

After the "Writing ergonomics" (or typewriter) section, add:

```markdown
## Focus mode

With typewriter on (`ctrl+t`), okashi also dims everything except the sentence
you're in — your current sentence stays bright and centered, the rest fades.
`ctrl+d` toggles just the dimming (keeping centered scrolling). Turning
typewriter off turns both off. (`ctrl+d` takes over the editor's delete-forward;
use Delete/Backspace instead.)
```

- [ ] **Step 2: Full verification**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/gofmt -l .            # expect no output
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -4
/opt/homebrew/bin/go build ./... && echo "ALL CLEAN"
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Docs: focus dimming"
```

- [ ] **Step 4: Manual smoke (real terminal — author to run)**

`/opt/homebrew/bin/go run .` → open a multi-sentence document; the sentence under the cursor is bright, the rest dimmed; move the cursor across sentences and watch the bright span follow; `ctrl+d` toggles dimming off/on; `ctrl+t` off removes both centering and dimming. Check the dim boundary lands exactly at sentence edges (offset tracking) and behaves on soft-wrapped long sentences and across blank lines.

---

## Self-Review

**Spec coverage (Section 5):**
- `dimEnabled` default-on, active when `typewriter && dimEnabled` → Task 3. `ctrl+d` toggle + status → Task 3. Sentence span (terminators, paragraph hard-boundary, spans single `\n`) → Task 1. Per-character dim render → Task 2. Testability (pure span + run split; integration with forced color) → Tasks 1, 2.

**Placeholder scan:** none — full code in every step. Task 1 Step 4 explicitly allows iterating `currentSentenceSpan` against its test table (the table is the spec).

**Type consistency:** `currentSentenceSpan`, `splitDimRuns`/`dimRun`, `Model.Dim`/`DimStyle`, `cursorRuneOffset`, `renderSeg`, `model.dimEnabled`, `syncDim` are used consistently across tasks. The render patch advances `pieceStart` by the pre-`TrimSuffix` piece length and `lineOffset` by `len(line)+1`, matching `Value()`'s newline joins and the verified wrapped-piece reconstruction.

**Risk note:** Task 2's offset tracking is the intricate part; the integration test (forced color profile) guards that dimming is applied and conditional, and the manual smoke confirms the boundary lands exactly. If the boundary is off by a character at wrapped-line ends, revisit `pieceStart`/`pieceLen` vs the `TrimSuffix` of overflow spaces.
