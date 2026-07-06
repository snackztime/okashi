# Outline mode Implementation Plan

> **For agentic workers:** executed inline, task-by-task with TDD, one commit per task on the `outline-mode` branch, adversarial review before merge. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A full-screen editor-first outline mode (`ctrl+l`) for drafting/reordering two-level beats+notes in `outline.md`, with a one-way `alt+↵` promote-beat→chapter bridge into the current manuscript's manifest.

**Architecture:** New `screenOutline` reuses the vendored `internal/textarea` editor (so autosave via the existing tick works for free — `currentFile` becomes `outline.md`). A pure parser (`outline_parse.go`) turns the buffer into beat blocks; structure ops (move, promote) act on those blocks. Promote reuses `uniqueChapterFile`/`writeManifest`/`saveSynopses`.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), Bubble Tea / lipgloss, `internal/textarea`.

## Global Constraints

- Invoke Go as `/opt/homebrew/bin/go`.
- Store stays plain `outline.md` (`- beat`, `  - note`). No new dependency.
- Promote writes **v1-shaped** `manifest.json` (append) + `.okashi-synopsis.json` — **no schema/shape change** (shared-contract gate untriggered). Chapter filenames stay birth-stable (`untitled[-N].md`); the beat text is the manifest **title**.
- Atomic writes only (`atomicWrite`/`writeManifest`/`saveSynopses`).
- `View()` stays O(visible) — reuse the editor's own windowed render.
- Structure-op keys are the **alt** family: `alt+↑`/`alt+↓` move a beat block, `alt+↵` promote. `esc` exits.
- No two-way sync; no nesting beyond two levels; no phantom cards.

---

### Task 1: The two-level parser (`outline_parse.go`)

**Files:**
- Create: `outline_parse.go`, `outline_parse_test.go`

**Interfaces (Produces):** `outlineBlock{start,end int}`, `isTopBeat(string) bool`, `beatBlocks([]string) []outlineBlock`, `blockAt([]string,int) (outlineBlock,bool)`, `beatTitle(string) string`, `beatIsPromoted(string) bool`, `beatNotes([]string,outlineBlock) []string`.

- [ ] **Step 1: Write failing tests** (`outline_parse_test.go`):
```go
package main

import (
	"reflect"
	"testing"
)

func TestBeatBlocksAndTitles(t *testing.T) {
	lines := []string{
		"preamble note",              // 0 — preamble, no block
		"- Act I",                    // 1
		"  - storm coming",           // 2
		"  - the letter",             // 3
		"- [x] Act II",               // 4
		"* Act III",                  // 5 (star marker)
	}
	got := beatBlocks(lines)
	want := []outlineBlock{{1, 4}, {4, 5}, {5, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("beatBlocks = %v, want %v", got, want)
	}
	if b, ok := blockAt(lines, 3); !ok || b != (outlineBlock{1, 4}) {
		t.Fatalf("blockAt(3) = %v,%v", b, ok)
	}
	if _, ok := blockAt(lines, 0); ok {
		t.Fatal("blockAt(0) should be preamble (ok=false)")
	}
	if beatTitle(lines[1]) != "Act I" {
		t.Fatalf("title I = %q", beatTitle(lines[1]))
	}
	if beatTitle(lines[4]) != "Act II" || !beatIsPromoted(lines[4]) {
		t.Fatalf("title II = %q promoted=%v", beatTitle(lines[4]), beatIsPromoted(lines[4]))
	}
	if beatIsPromoted(lines[1]) {
		t.Fatal("Act I is not promoted")
	}
	notes := beatNotes(lines, outlineBlock{1, 4})
	if !reflect.DeepEqual(notes, []string{"storm coming", "the letter"}) {
		t.Fatalf("notes = %v", notes)
	}
}
```

- [ ] **Step 2: Run → FAIL** (undefined). `/opt/homebrew/bin/go test . -run TestBeatBlocks`

- [ ] **Step 3: Implement `outline_parse.go`:**
```go
package main

import "strings"

// outlineBlock is the [start,end) line range of a beat and its notes.
type outlineBlock struct{ start, end int }

func isBulletMarker(b byte) bool { return b == '-' || b == '*' || b == '+' }

// isTopBeat reports whether line is a top-level list item (marker + space, at indent 0).
func isTopBeat(line string) bool {
	return len(line) >= 2 && isBulletMarker(line[0]) && line[1] == ' '
}

// beatBlocks returns each beat's block range in order; lines before the first beat (preamble) belong
// to no block.
func beatBlocks(lines []string) []outlineBlock {
	var blocks []outlineBlock
	start := -1
	for i, ln := range lines {
		if isTopBeat(ln) {
			if start >= 0 {
				blocks = append(blocks, outlineBlock{start, i})
			}
			start = i
		}
	}
	if start >= 0 {
		blocks = append(blocks, outlineBlock{start, len(lines)})
	}
	return blocks
}

// blockAt returns the beat block containing line, or ok=false when line is in the preamble.
func blockAt(lines []string, line int) (outlineBlock, bool) {
	for _, b := range beatBlocks(lines) {
		if line >= b.start && line < b.end {
			return b, true
		}
	}
	return outlineBlock{}, false
}

// stripMarker drops a leading bullet marker + space from a trimmed string.
func stripMarker(s string) string {
	if len(s) >= 2 && isBulletMarker(s[0]) && s[1] == ' ' {
		return strings.TrimSpace(s[2:])
	}
	return s
}

// beatTitle strips a beat line's marker + an optional [ ]/[x] task box + surrounding spaces.
func beatTitle(line string) string {
	s := stripMarker(strings.TrimSpace(line))
	switch {
	case strings.HasPrefix(s, "[ ] "), strings.HasPrefix(s, "[x] "), strings.HasPrefix(s, "[X] "):
		return strings.TrimSpace(s[4:])
	case s == "[ ]" || s == "[x]" || s == "[X]":
		return ""
	}
	return s
}

// beatIsPromoted reports whether a beat line carries a checked task box.
func beatIsPromoted(line string) bool {
	s := stripMarker(strings.TrimSpace(line))
	return strings.HasPrefix(s, "[x]") || strings.HasPrefix(s, "[X]")
}

// beatNotes returns a block's note lines (after the beat line), each trimmed of indent + marker,
// blanks dropped.
func beatNotes(lines []string, b outlineBlock) []string {
	var notes []string
	for i := b.start + 1; i < b.end; i++ {
		s := stripMarker(strings.TrimSpace(lines[i]))
		if s != "" {
			notes = append(notes, s)
		}
	}
	return notes
}
```

- [ ] **Step 4: Run → PASS.** `/opt/homebrew/bin/go test . -run TestBeatBlocks`
- [ ] **Step 5: Commit** — `git commit -am "outline: two-level beat parser"`

---

### Task 2: Pure move-beat op

**Files:**
- Modify: `outline_parse.go` (add `moveBeat`)
- Modify: `outline_parse_test.go`

**Interfaces (Produces):** `moveBeat(lines []string, cursorLine, dir int) (out []string, newCursor int, ok bool)`.

- [ ] **Step 1: Write failing test:**
```go
func TestMoveBeat(t *testing.T) {
	lines := []string{"- A", "  - a1", "- B", "  - b1", "  - b2"}
	// Move block B (cursor on its note, line 3) UP past A.
	out, nc, ok := moveBeat(lines, 3, -1)
	if !ok {
		t.Fatal("move up should apply")
	}
	want := []string{"- B", "  - b1", "  - b2", "- A", "  - a1"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v", out)
	}
	if nc != 1 { // cursor kept its offset (was b.start+1) on the moved block, now at 0+1
		t.Fatalf("newCursor = %d, want 1", nc)
	}
	// No neighbor above A → no-op.
	if _, _, ok := moveBeat(lines, 0, -1); ok {
		t.Fatal("A has no block above → no-op")
	}
	// Preamble cursor → no-op.
	if _, _, ok := moveBeat([]string{"note", "- A"}, 0, 1); ok {
		t.Fatal("preamble → no-op")
	}
}
```

- [ ] **Step 2: Run → FAIL.** `/opt/homebrew/bin/go test . -run TestMoveBeat`

- [ ] **Step 3: Implement `moveBeat`:**
```go
// moveBeat swaps the beat block containing cursorLine with its neighbor (dir -1 up / +1 down),
// keeping the cursor on the same line within the moved block. ok=false (no change) when the cursor is
// in the preamble or there is no neighbor that way. Adjacent beat blocks are contiguous.
func moveBeat(lines []string, cursorLine, dir int) ([]string, int, bool) {
	blocks := beatBlocks(lines)
	idx := -1
	for i, b := range blocks {
		if cursorLine >= b.start && cursorLine < b.end {
			idx = i
			break
		}
	}
	if idx < 0 {
		return lines, cursorLine, false
	}
	j := idx + dir
	if j < 0 || j >= len(blocks) {
		return lines, cursorLine, false
	}
	lo, hi := idx, j
	if lo > hi {
		lo, hi = hi, lo
	}
	A, B := blocks[lo], blocks[hi] // A.end == B.start (contiguous)
	out := make([]string, 0, len(lines))
	out = append(out, lines[:A.start]...)
	out = append(out, lines[B.start:B.end]...) // B moves before A
	out = append(out, lines[A.start:A.end]...)
	out = append(out, lines[B.end:]...)
	off := cursorLine - blocks[idx].start
	newStart := A.start // moving B up → B now starts where A did
	if idx == lo {      // moving A down → A now sits after B
		newStart = A.start + (B.end - B.start)
	}
	return out, newStart + off, true
}
```

- [ ] **Step 4: Run → PASS.** `/opt/homebrew/bin/go test . -run TestMoveBeat`
- [ ] **Step 5: Commit** — `git commit -am "outline: pure move-beat op"`

---

### Task 3: Screen scaffolding — entry/exit, typing, view, `ctrl+l`

**Files:**
- Modify: `main.go` (`screenOutline` const; Update dispatch ~926; View dispatch ~1658; repoint `ctrl+l` case ~1399)
- Modify: `outline.go` (`enterOutline`, `exitOutline`, `updateOutline`, `outlineView`)
- Modify: `smoke_test.go` (the existing ctrl+l toggle test)

**Interfaces:**
- Consumes: `m.editor` (textarea), `m.outlineReturnFile`, `loadFile`, `save`, `listContinuation`, `filelist.SetDir`.
- Produces: `enterOutline`, `exitOutline`, `updateOutline`, `outlineView`; `screenOutline`.

- [ ] **Step 1: Update the existing ctrl+l test** in `smoke_test.go` to the new behavior (this is the failing test). Find `TestCtrlL…` (asserts inline toggle) and replace its body so: `ctrl+l` sets `m.screen == screenOutline` and creates `outline.md`; a subsequent `esc` returns to the original chapter (`filepath.Base(m.currentFile) == "01-a.md"`) with `m.screen == screenWriting`. Keep the same fixture setup.

- [ ] **Step 2: Run → FAIL.** `/opt/homebrew/bin/go test . -run TestCtrlL`

- [ ] **Step 3: Add `screenOutline`** to the screen const block (after `screenNotes`). Add Update dispatch after the `screenNotes` block (~930):
```go
	if m.screen == screenOutline {
		return m.updateOutline(msg)
	}
```
Add View dispatch after the `screenNotes` view (~1663):
```go
	if m.screen == screenOutline {
		return m.outlineView()
	}
```
Repoint the `ctrl+l` case (~1399) — replace the whole inline-toggle body with:
```go
		case "ctrl+l":
			m.enterOutline()
			return m, nil
```

- [ ] **Step 4: Implement in `outline.go`** (`enterOutline`/`exitOutline`/`updateOutline`/`outlineView`):
```go
func (m *model) enterOutline() {
	m.save() // flush the current chapter before loadFile reloads over it
	outlinePath := filepath.Join(m.files.dir, "outline.md")
	if _, err := os.Stat(outlinePath); err != nil {
		if werr := atomicWrite(outlinePath, []byte("- \n"), 0o644); werr != nil {
			m.status = "couldn't create outline: " + werr.Error()
			return
		}
		m.files.SetDir(m.files.dir)
	}
	m.outlineReturnFile = m.currentFile
	m.loadFile(outlinePath)
	m.editor.Dim = false // no sentence-dim in the outline
	m.screen = screenOutline
	m.focus = focusEditor
	m.editor.Focus()
}

func (m *model) exitOutline() {
	m.save()                    // flush outline.md
	m.files.SetDir(m.files.dir) // reflect any promoted chapters in the pane/manifest
	if m.outlineReturnFile != "" {
		m.loadFile(m.outlineReturnFile)
	}
	m.syncDim() // restore the writing-screen dim setting
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}

func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		m.layout()
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.exitOutline()
		return m, nil
	case "alt+up":
		m.moveOutlineBeat(-1)
		return m, nil
	case "alt+down":
		m.moveOutlineBeat(1)
		return m, nil
	case "alt+enter", "ctrl+enter":
		m.promoteOutlineBeat()
		return m, nil
	case "enter":
		if m.editor.AtLineEnd() {
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
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	m.dirty = true
	m.lastEditAt = time.Now()
	return m, cmd
}

func (m *model) moveOutlineBeat(dir int) {
	out, nc, ok := moveBeat(strings.Split(m.editor.Value(), "\n"), m.editor.Line(), dir)
	if !ok {
		m.status = "move: put the cursor on a beat"
		return
	}
	m.editor.SetValue(strings.Join(out, "\n"))
	m.editor.MoveToLine(nc)
	m.dirty = true
	m.save()
}

func (m model) outlineView() string {
	title := projectTitle(filepath.Base(m.files.dir))
	header := sectionHeader("OUTLINE · "+title, m.width)
	foot := lipgloss.NewStyle().Foreground(subtle).Render("alt+↑/↓ move beat · alt+↵ promote · esc done")
	body := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Top, m.editor.View())
	return lipgloss.JoinVertical(lipgloss.Left, header, body,
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
}
```
(If `m.layout()` doesn't size the editor without the sidebar, size it in `outlineView` the way the writing view does — `m.editor.SetWidth(colWidth)`/`SetHeight(m.height-3)` — matching lines ~1787. `promoteOutlineBeat` is a stub for now: `func (m *model) promoteOutlineBeat() { m.status = "promote coming in the next task" }` — replaced in Task 5.)

- [ ] **Step 5: Build + run → PASS.** `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test . -run TestCtrlL`
- [ ] **Step 6: Full suite green.** `/opt/homebrew/bin/go test .`
- [ ] **Step 7: Commit** — `git commit -am "outline: full-screen screen scaffolding + ctrl+l entry"`

---

### Task 4: Wire move-beat keys (integration test)

**Files:**
- Modify: `outline_test.go` (create)

**Interfaces:** none new (move already wired in Task 3).

- [ ] **Step 1: Write failing integration test** (`outline_test.go`) — enter the outline on a manuscript, set the editor value to two beats, put the cursor on the second beat, dispatch `alt+down`/`alt+up` via `updateOutline`, and assert the editor `Value()` reordered and the cursor followed. Build the model with the corkboard/manuscript test fixtures (`seedCorkManuscript` is in-package).

- [ ] **Step 2: Run → FAIL** (until Task 3's wiring is confirmed end-to-end; if it already passes because Task 3 wired it, note that and proceed — this task locks the behavior).
Run: `/opt/homebrew/bin/go test . -run TestOutlineMove`

- [ ] **Step 3: Fix any wiring gaps** surfaced (e.g., `alt+up`/`alt+down` key strings, cursor mapping).

- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** — `git commit -am "outline: move-beat keys (alt+up/down)"`

---

### Task 5: Promote bridge

**Files:**
- Modify: `outline.go` (`promoteOutlineBeat`, `markBeatPromoted`)
- Modify: `outline_test.go`

**Interfaces (Produces):** `promoteOutlineBeat()`, `markBeatPromoted(string) string`.

- [ ] **Step 1: Write failing tests** — on a `seedCorkManuscript` dir: set the editor to `"- New Chapter\n  - a note\n  - two"`, cursor on line 0, `enterOutline`, then dispatch `alt+enter`. Assert: (a) manifest now has 4 items and the last is titled "New Chapter"; (b) that chapter's synopsis in `.okashi-synopsis.json` == "a note\ntwo"; (c) the editor line 0 is now `- [x] New Chapter`; (d) a second `alt+enter` on the same line is a no-op (`beatIsPromoted` guard) and the manifest stays at 4 items. Add a guard test: promote in a non-manifest temp dir sets a status and does not create a file.

- [ ] **Step 2: Run → FAIL** (stub). `/opt/homebrew/bin/go test . -run TestOutlinePromote`

- [ ] **Step 3: Implement:**
```go
func (m *model) promoteOutlineBeat() {
	lines := strings.Split(m.editor.Value(), "\n")
	b, ok := blockAt(lines, m.editor.Line())
	if !ok {
		m.status = "promote: put the cursor on a beat"
		return
	}
	if beatIsPromoted(lines[b.start]) {
		m.status = "already promoted"
		return
	}
	title := beatTitle(lines[b.start])
	if title == "" {
		m.status = "promote: the beat has no title"
		return
	}
	dir := m.files.dir
	mani, present, err := readManifest(dir)
	if err != nil || !present {
		m.status = "promote needs a manuscript (no manifest here)"
		return
	}
	taken := map[string]bool{}
	for _, it := range mani.Items {
		taken[it.File] = true
	}
	file := uniqueChapterFile(dir, taken)
	if werr := atomicWrite(filepath.Join(dir, file), []byte(""), 0o644); werr != nil {
		m.status = "promote failed: " + werr.Error()
		return
	}
	mani.Items = append(mani.Items, manifestItem{File: file, Title: title})
	if werr := writeManifest(dir, mani); werr != nil {
		m.status = "promote failed: " + werr.Error()
		return
	}
	if notes := beatNotes(lines, b); len(notes) > 0 {
		syn := loadSynopses(dir)
		if syn == nil {
			syn = map[string]string{}
		}
		syn[file] = strings.Join(notes, "\n")
		chapters := map[string]bool{}
		for _, it := range mani.Items {
			chapters[it.File] = true
		}
		_ = saveSynopses(dir, syn, chapters)
	}
	lines[b.start] = markBeatPromoted(lines[b.start])
	m.editor.SetValue(strings.Join(lines, "\n"))
	m.dirty = true
	m.save()
	m.status = "promoted “" + title + "”"
}

// markBeatPromoted rewrites a top-level beat line to a checked task item, preserving its marker.
func markBeatPromoted(line string) string {
	return string(line[0]) + " [x] " + beatTitle(line)
}
```

- [ ] **Step 4: Run → PASS.** `/opt/homebrew/bin/go test . -run TestOutlinePromote`
- [ ] **Step 5: Full suite green.** `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go vet . && /opt/homebrew/bin/go test .`
- [ ] **Step 6: Commit** — `git commit -am "outline: promote beat → chapter (seeds synopsis, marks [x])"`

---

### Task 6: Docs + help

**Files:**
- Modify: `main.go` (`helpText` — the `ctrl+l outline` line + a MANUSCRIPT/OUTLINE note)
- Modify: `README.md` (shortcut row + a short "Outline" section)

- [ ] **Step 1: Update `helpText`.** Keep `ctrl+l outline` in NAVIGATE; add an OUTLINE note: `alt+↑/↓ move beat · alt+↵ promote → chapter`. If `help_test.go` asserts substrings, keep them satisfied or update them.
- [ ] **Step 2: Update `README.md`.** Change the `ctrl+l` row to "Outline (full-screen brainstorming — beats & notes)"; add a short **Outline** section: draft two-level beats+notes in `outline.md`; `alt+↑/↓` reorder a beat; `alt+↵` promote a beat into a chapter (seeds its synopsis from the notes, marks the beat done); it's a separate surface from the corkboard, with a one-way promote and no back-sync.
- [ ] **Step 3: Build + full suite.** `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test .`
- [ ] **Step 4: Commit** — `git commit -am "docs: outline mode"`

---

## After all tasks
- Adversarial whole-branch review (parser edge cases, editor-reuse side effects — autosave/grammar/undo while `currentFile==outline.md`; the `ctrl+l`→`esc` round-trip; promote atomicity + guards; help/README drift).
- `finishing-a-development-branch`: verify `/opt/homebrew/bin/go test ./...`, present merge options.
