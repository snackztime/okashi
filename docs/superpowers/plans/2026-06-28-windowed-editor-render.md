# Windowed Editor Render Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Make `internal/textarea` `View()` cost O(visible height) instead of O(buffer) so a 400-page single file edits as smoothly as a chapter, by rendering only the visible window. The `[][]rune` buffer and all edit/cursor logic stay untouched.

**Architecture:** Profiling shows the cost is per-line lipgloss styling multiplied across every buffer line. We window the *render*: compute the first visible display row, map it to a source line via cached wrap-heights, and style only the ~`height` visible rows — reusing the existing per-piece render block verbatim. Two secondary O(buffer) costs are also removed: the wrap cache stuck at capacity 99, and the dim sentence span calling `m.Value()` each frame.

**Tech Stack:** Go; `internal/textarea` (vendored). Bench/test via `/opt/homebrew/bin/go`.

**Design spec:** `docs/superpowers/specs/2026-06-28-windowed-editor-render-design.md`.

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt`.
- **Buffer model (`m.value [][]rune`) and ALL edit/cursor/word-motion ops are untouched.** Only `View()`, scroll-offset tracking, the wrap cache sizing, and the dim-span computation change.
- **The visible output must be byte-identical to today's** for the same scroll position — the existing tests are the gate. Off-screen lines simply stop being processed.
- Preserve: typewriter centering (cursor at window row `height/2`), non-typewriter scrolling (cursor kept visible), dim styling, smart quotes, `MoveToLine`, focus/blur, line-number/prompt handling.
- All existing `internal/textarea` tests pass unchanged: `dim_test.go`, `editing_test.go`, `moveline_test.go`, `typewriter_test.go`.
- **Acceptance gate:** `BenchmarkEditorViewWholeDraft` drops to ≈ `BenchmarkEditorViewChapter` and stays flat vs `…HalfDraft`.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit. Each task is its own commit (revertible).

## Reference: the per-piece render block

Tasks below reuse the existing inner block from `View()` (`textarea.go` ~1198–1262) **verbatim** — the wrapped-piece loop that computes `pieceLen`, prompt, line numbers, `widestLineNumber`, `strwidth`/`padding`/trailing-space trim, the cursor-segment branch (`m.row == l && lineInfo.RowOffset == wl`) using `renderSeg`, and the trailing `\n`. The ONLY change in Task 3 is the *bounds* it runs over (visible window) and where `displayLine`/`pieceStart`/`lineOffset` come from. Do not alter the per-piece rendering logic itself.

---

## Task 1: Decouple the wrap cache from MaxHeight

**Files:**
- Modify: `internal/textarea/textarea.go`
- Test: `internal/textarea/wrapcache_test.go`

**Why:** the cache is created at `defaultMaxHeight` (99) and only resized `if m.MaxHeight > 0` (~line 1047). With okashi's `MaxHeight = 0` it's stuck at 99, so a buffer over 99 distinct lines thrashes. Size it to the buffer.

**Interfaces:**
- Produces: cache capacity tracks `max(defaultMaxHeight, len(m.value))`.

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/wrapcache_test.go`:

```go
package textarea

import (
	"strings"
	"testing"
)

// The wrap cache must grow with the buffer, not stay pinned at the MaxHeight
// default (99). Regression for the large-file render thrash. Note: CharLimit
// and MaxHeight are both 0 (okashi's settings) so the buffer isn't truncated;
// the resize must fire on the SetValue (load) path, not only on a keystroke.
func TestWrapCacheGrowsWithBuffer(t *testing.T) {
	m := New()
	m.CharLimit = 0 // unlimited (default 400 would truncate the test buffer)
	m.MaxHeight = 0 // okashi's setting
	m.SetWidth(72)
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("a distinct line number ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteByte('\n')
	}
	m.SetValue(b.String()) // load path — no keystroke/Update
	if m.cache.Capacity() < 500 {
		t.Fatalf("cache capacity = %d, want >= 500 (buffer size) — cache thrashes on big files", m.cache.Capacity())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run TestWrapCacheGrowsWithBuffer 2>&1 | tail`
Expected: FAIL — capacity is 99.

- [ ] **Step 3: Size the cache to the buffer (on the load path AND the keystroke path)**

The cache resize must fire when the buffer is established (`SetValue → InsertString`), not only on a keystroke (`Update`). Add a pointer-receiver helper and call it from both. In `internal/textarea/textarea.go`:

Add the helper:

```go
// ensureWrapCacheCapacity grows the wrap-memoization cache to cover the whole
// buffer so large files don't thrash it. Never shrinks below the default.
func (m *Model) ensureWrapCacheCapacity() {
	if want := max(defaultMaxHeight, len(m.value)); m.cache.Capacity() < want {
		m.cache = memoization.NewMemoCache[line, [][]rune](want)
	}
}
```

Call it at the **end of `InsertString`** (covers `SetValue`/load and typed insertions) and at the **end of `Update`** just before `return m, tea.Batch(cmds...)` (covers line growth from `splitLine` on Enter, which doesn't go through `InsertString`). `Update` has a value receiver but `m` is addressable there, so `m.ensureWrapCacheCapacity()` works exactly like the existing `m.repositionView()` call.

**Remove** the old MaxHeight-gated resize block in `Update` (~line 1047):

```go
	// DELETE these lines:
	if m.MaxHeight > 0 && m.MaxHeight != m.cache.Capacity() {
		m.cache = memoization.NewMemoCache[line, [][]rune](m.MaxHeight)
	}
```

(`max` is already used in this file.)

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run TestWrapCacheGrowsWithBuffer -v 2>&1 | tail
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/wrapcache_test.go
/opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea/textarea.go internal/textarea/wrapcache_test.go
git commit -m "textarea: size wrap cache to the buffer, not the MaxHeight default"
```

---

## Task 2: Compute the dim sentence span cursor-locally (no m.Value() per frame)

**Files:**
- Modify: `internal/textarea/textarea.go`
- Test: `internal/textarea/dimspan_test.go`

**Why:** `View()` calls `currentSentenceSpan(m.Value(), m.cursorRuneOffset())` every frame; `m.Value()` joins the whole buffer (O(buffer)). The dim span only needs the sentence around the cursor.

**Interfaces:**
- Consumes: `currentSentenceSpan` (existing), `m.cursorRuneOffset()` (existing).
- Produces: `func (m Model) cursorSentenceSpan() (int, int)` returning the same absolute `[span0, span1)` as `currentSentenceSpan(m.Value(), m.cursorRuneOffset())`, computed from a cursor-local slice.

- [ ] **Step 1: Write the failing test**

Create `internal/textarea/dimspan_test.go`:

```go
package textarea

import (
	"strings"
	"testing"
)

// cursorSentenceSpan must equal the whole-buffer computation it replaces, for
// the cursor placed in various positions of a multi-line buffer.
func TestCursorSentenceSpanMatchesFullValue(t *testing.T) {
	m := New()
	m.SetWidth(72)
	m.SetValue("First sentence here. Second one follows! And a third?\n\nNew paragraph starts. It has two sentences.\n")
	for _, pos := range []struct{ row, col int }{
		{0, 3}, {0, 25}, {0, 50}, {2, 5}, {2, 30},
	} {
		m.row, m.col = pos.row, pos.col
		want0, want1 := currentSentenceSpan(m.Value(), m.cursorRuneOffset())
		got0, got1 := m.cursorSentenceSpan()
		if got0 != want0 || got1 != want1 {
			t.Fatalf("row %d col %d: got [%d,%d), want [%d,%d)", pos.row, pos.col, got0, got1, want0, want1)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run TestCursorSentenceSpanMatchesFullValue 2>&1 | tail`
Expected: FAIL — `cursorSentenceSpan` undefined.

- [ ] **Step 3: Implement `cursorSentenceSpan`**

Add to `internal/textarea/textarea.go`. Build a bounded local window around the cursor (a handful of source lines on each side is more than a sentence), run the existing `currentSentenceSpan` on it, and translate the returned offsets back to absolute buffer offsets:

```go
// cursorSentenceSpan returns the same absolute [span0,span1) as
// currentSentenceSpan(m.Value(), m.cursorRuneOffset()) but scans only a bounded
// window of source lines around the cursor, avoiding an O(buffer) m.Value() join
// every frame. okashi:dim
func (m Model) cursorSentenceSpan() (int, int) {
	const radius = 4 // sentences never span more than a few source lines here
	lo := max(0, m.row-radius)
	hi := min(len(m.value), m.row+radius+1)

	// Absolute rune offset of the first rune of line `lo`.
	base := 0
	for i := 0; i < lo; i++ {
		base += len(m.value[i]) + 1
	}
	// Join the window exactly as Value() would (newline between lines).
	var b strings.Builder
	for i := lo; i < hi; i++ {
		if i > lo {
			b.WriteByte('\n')
		}
		b.WriteString(string(m.value[i]))
	}
	local := m.cursorRuneOffset() - base
	s0, s1 := currentSentenceSpan(b.String(), local)
	return s0 + base, s1 + base
}
```

Then in `View()` replace the dim-span computation (~line 1183):

```go
	var dimSpan0, dimSpan1 int
	if m.Dim {
		dimSpan0, dimSpan1 = m.cursorSentenceSpan()
	}
```

- [ ] **Step 4: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestCursorSentenceSpan|TestDim' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/dimspan_test.go
/opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add internal/textarea/textarea.go internal/textarea/dimspan_test.go
git commit -m "textarea: compute dim sentence span cursor-locally (drop per-frame m.Value())"
```

---

## Task 3: Window the render in View() (the core change)

**Files:**
- Modify: `internal/textarea/textarea.go` (`View`, `repositionView`, struct, `New`, drop `renderViewport`/viewport coupling as needed)
- Test: `internal/textarea/window_test.go`

**Why:** the dominant O(buffer) cost — styling every line every frame. Render only the visible window.

**Interfaces:**
- Consumes: `cursorLineNumber()` (cursor's display row), `LineInfo()`, `memoizedWrap`, `renderSeg`, `cursorSentenceSpan` (Task 2), the per-piece block (see Reference).
- Produces: a windowed `View()`; an explicit scroll offset field `offset int` (first visible display row in non-typewriter mode); helpers `displayHeight() int`, `locateRow(row int) (l, wl, lineOffset int)`.

### Window math (derived from the current behavior)

- `cln := cursorLineNumber()` is the cursor's absolute display row.
- **Typewriter** (today: `renderViewport` prepends `height/2` blanks and sets `YOffset = cln`): the first visible display row is `top = cln - height/2`. `top` may be negative near the start → emit blank rows above row 0 (matching today's prepended blanks). The cursor lands at window row `height/2`.
- **Non-typewriter** (today: `repositionView` keeps the cursor within `[YOffset, YOffset+height)` via LineUp/LineDown): track the offset in a new `offset int` field; `top = offset`. `repositionView` updates `offset` to keep `cln` visible, clamped to `[0, max(0, displayHeight()-height)]`.
- Rows at/after `displayHeight()` render the end-of-buffer gutter (today's bottom padding loop).

### Steps

- [ ] **Step 1: Write the failing tests**

Create `internal/textarea/window_test.go`. The key invariant: **windowed output equals the visible slice of a full reference render**, plus structural checks. Build a reference by reconstructing the pre-windowing behavior is impractical, so assert observable properties instead:

```go
package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func bigEditor(lines int) Model {
	m := New()
	m.Prompt = ""
	m.ShowLineNumbers = false
	m.CharLimit = 0
	m.MaxHeight = 0
	m.FocusedStyle.Base = lipgloss.NewStyle()
	m.SetWidth(40)
	m.SetHeight(10)
	m.Focus()
	var b strings.Builder
	for i := 0; i < lines; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("w", 3))
		b.WriteByte('\n')
	}
	m.SetValue(b.String())
	return m
}

// The view is exactly `height` rows regardless of buffer size or scroll spot.
func TestViewEmitsHeightRows(t *testing.T) {
	m := bigEditor(500)
	for _, row := range []int{0, 5, 250, 499} {
		m.row, m.col = row, 0
		m.repositionView()
		got := strings.Count(m.View(), "\n")
		if got < m.height {
			t.Fatalf("cursor row %d: view has %d newlines, want >= height %d", row, got, m.height)
		}
	}
}

// The cursor's line content is present in the rendered window wherever the
// cursor is — i.e. the window actually follows the cursor.
func TestCursorLineVisible(t *testing.T) {
	m := bigEditor(500)
	m.SetValue(strings.Repeat("filler\n", 250) + "UNIQUEMARKER here\n" + strings.Repeat("filler\n", 250))
	m.row = 250 // the UNIQUEMARKER line
	m.col = 0
	m.repositionView()
	if !strings.Contains(m.View(), "UNIQUEMARKER") {
		t.Fatal("cursor line not visible in the windowed view")
	}
}

// A far-away line must NOT be rendered (proves we don't style the whole buffer).
func TestOffscreenLineNotRendered(t *testing.T) {
	m := bigEditor(0)
	m.SetValue("TOPMARKER\n" + strings.Repeat("filler\n", 500) + "BOTTOMMARKER\n")
	m.moveToEnd() // cursor at the bottom
	m.repositionView()
	v := m.View()
	if strings.Contains(v, "TOPMARKER") {
		t.Fatal("top line rendered while scrolled to the bottom — not windowed")
	}
	if !strings.Contains(v, "BOTTOMMARKER") {
		t.Fatal("bottom (cursor) line not visible")
	}
}
```

- [ ] **Step 2: Run to verify they fail (or that current behavior differs)**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestViewEmitsHeightRows|TestCursorLineVisible|TestOffscreenLineNotRendered' 2>&1 | tail`
Expected: `TestOffscreenLineNotRendered` FAILS today (the full buffer is rendered, so TOPMARKER is present); the others may pass via the old viewport path. After Step 3 all three pass.

- [ ] **Step 3: Implement the windowed View()**

Add the offset field and helpers, rewrite `View()`, and update `repositionView`. Concretely:

1. **Struct + New:** add `offset int` to `Model`. Keep the existing `viewport` field only if other code still needs it; this plan stops feeding it full content, so prefer removing it along with `renderViewport`. If removing the viewport, also remove its `Update` call (`m.viewport, cmd = …`) and replace any mouse-wheel handling by adjusting `m.offset` (clamped). If that widens scope unexpectedly, keep the viewport struct but never `SetContent` the full buffer — set `m.offset` as the source of truth and ignore the viewport's view.

2. **Helpers:**

```go
// displayHeight is the total number of wrapped display rows in the buffer.
func (m Model) displayHeight() int {
	n := 0
	for _, line := range m.value {
		n += len(m.memoizedWrap(line, m.width))
	}
	return n
}

// locateRow returns the source line index and wrap-piece index containing
// absolute display row `row`, plus the absolute rune offset of that source
// line's first rune. row <= 0 maps to (0, 0, 0). row past the end returns
// (len(m.value), 0, <offset past end>).
func (m Model) locateRow(row int) (l, wl, lineOffset int) {
	if row < 0 {
		row = 0
	}
	acc := 0
	off := 0
	for i, line := range m.value {
		h := len(m.memoizedWrap(line, m.width))
		if acc+h > row {
			return i, row - acc, off
		}
		acc += h
		off += len(line) + 1
	}
	return len(m.value), 0, off
}
```

3. **`View()`:** replace the body after the dim-span block with a windowed emit. Compute `h := m.height` (min 1) and `top`:

```go
	top := m.offset
	if m.Typewriter && m.height > 0 {
		top = m.cursorLineNumber() - m.height/2
	}
```

   Then emit exactly `h` rows into `s`:
   - while the current absolute row `< 0` and rows emitted `< h`: write `"\n"` (the typewriter top pad), advance.
   - `l, wl, lineOffset := m.locateRow(max(0, top))`; iterate source lines from `l`, wrapping each, **starting at piece `wl` for the first line** (compute `pieceStart` by summing the rune-lengths of the skipped pieces `0..wl-1`), running **the existing per-piece block verbatim** for each visible piece until `h` rows are emitted. `displayLine` for `getPromptString` is the running emitted-row index. Advance `lineOffset += len(line) + 1` after each source line; reset the per-line `wl` start to 0 after the first line.
   - after the buffer is exhausted, run **the existing end-of-buffer row block verbatim** until `h` rows total are emitted.
   - `return m.style.Base.Render(s.String())` (no viewport).

   The per-piece and EOB blocks are unchanged from the current `View()` (Reference section) — only their iteration bounds change.

4. **`repositionView()`** (non-typewriter branch): replace the viewport LineUp/LineDown with offset math:

```go
func (m *Model) repositionView() {
	if m.Typewriter {
		return // typewriter centering is computed in View
	}
	row := m.cursorLineNumber()
	if row < m.offset {
		m.offset = row
	} else if row >= m.offset+m.height {
		m.offset = row - m.height + 1
	}
	if maxOff := max(0, m.displayHeight()-m.height); m.offset > maxOff {
		m.offset = maxOff
	}
	if m.offset < 0 {
		m.offset = 0
	}
}
```

- [ ] **Step 4: Run the windowing tests + ALL existing textarea tests**

Run: `/opt/homebrew/bin/go test ./internal/textarea/ -v 2>&1 | tail -40`
Expected: the new windowing tests PASS **and** every existing test (`dim_test`, `editing_test`, `moveline_test`, `typewriter_test`) still PASSES. If any existing test fails, the windowed output diverged from the old behavior — fix the window math, do not edit the tests.

- [ ] **Step 5: gofmt; full suite; build; commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/window_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add internal/textarea/textarea.go internal/textarea/window_test.go
git commit -m "textarea: window View() to O(visible) — render only the visible rows"
```

---

## Task 4: Verify the performance gate + okashi end-to-end

**Files:** none (verification only); if a tweak is needed it lands in `internal/textarea/textarea.go`.

- [ ] **Step 1: Editor benchmark — whole-draft ≈ chapter, flat vs size**

Run: `/opt/homebrew/bin/go test . -run '^$' -bench 'BenchmarkEditorView' -benchmem -benchtime=50x 2>&1 | grep -E "Benchmark|ns/op"`
Expected: `BenchmarkEditorViewWholeDraft` is within ~2× of `BenchmarkEditorViewChapter` (was ~34×: 51ms vs 1.5ms) and roughly equal to `…HalfDraft` (flat, not linear). Record the numbers in the task report.

- [ ] **Step 2: okashi smoke — the editor still works**

Run: `/opt/homebrew/bin/go test . 2>&1 | tail -3`
Expected: PASS — `smoke_test.go` (preview toggle etc.) and `editor_height_test.go` still green, confirming SetValue/View integrate.

- [ ] **Step 3: Manual sanity note**

Document in the report that `go run .` opens, a large file loads, and arrow/typewriter scrolling renders the correct window (cursor centered in typewriter mode; cursor kept visible with `ctrl+t` off). (No code; a checklist for the reviewer / user.)

---

## Self-Review

**Spec coverage:** windowed render (dominant cost) → Task 3; wrap-cache decouple → Task 1; dim-span without `m.Value()` → Task 2; performance gate + buffer-untouched verification → Task 4. Buffer/edit ops untouched across all tasks (only View/repositionView/cache/dim-span change).

**Placeholder scan:** Tasks 1–2 carry full code. Task 3's per-piece and EOB blocks are reused verbatim from the existing `View()` (quoted by reference, not re-typed, to avoid transcription drift) — the implementer moves them under the windowed bounds; the new helpers (`displayHeight`, `locateRow`, offset `top`/`repositionView`) are given in full.

**Type consistency:** `cursorSentenceSpan() (int,int)` (Task 2) consumed by `View` (Task 3); `offset int`, `displayHeight()`, `locateRow()` defined and used in Task 3; `cursorLineNumber()`, `LineInfo()`, `renderSeg`, `memoizedWrap` reused as-is.

**Risk note (for the executor):** Task 3 is coupled render+scroll surgery on the editing surface. The existing behavioral tests (`typewriter_test`, `moveline_test`, `editing_test`, `dim_test`) are the hard correctness gate — they assert the *visible* output and cursor behavior, which must not change. Treat any existing-test failure as a window-math bug, never edit those tests to pass. This task warrants the most capable model and careful review.
