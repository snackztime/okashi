# Tufte sidenote preview · README · scrub — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add margin **sidenotes** to okashi's Tufte preview, rewrite the README as a standalone product doc with a shortcuts table, and scrub the companion project's name from user-facing strings and code comments.

**Architecture:** The sidenote preview keeps glamour for body rendering and adds a pure layout pass that floats footnotes into a right gutter (Approach B from the spec). The scrub genericizes names without deleting rationale. The README is transcribed from the live `helpText` and env knobs.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea, lipgloss, glamour, `github.com/charmbracelet/x/ansi` (already a dep, for visible-width measurement).

## Global Constraints

- `go` is invoked as `/opt/homebrew/bin/go`; `gofmt` as `/opt/homebrew/bin/gofmt`. Module is `okashi`, flat `package main` + `internal/textarea`.
- `View()` stays O(visible); the preview is a viewport that already windows — do not stringify differently per frame. `layoutSidenotes` runs once per `renderPreview()` call (on open / toggle / resize), not per frame.
- Default preview mode and narrow-terminal Tufte mode MUST remain byte-for-byte unchanged. Sidenotes engage ONLY when `previewTufte && preview.Width >= 90 && the doc has referenced footnotes`.
- No new markdown syntax; only GFM footnotes become sidenotes (markdown-flavor contract).
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.
- Constants: `sidenoteMinWidth = 90`; gutter = clamp(total/3, 18, 30); measure = total − gutter − 3 (the `" ┆ "` gap is 3 columns).
- Superscript markers come from the existing `superscript(n int) string` in preview.go. Match a note's marker against a WHOLE superscript run, never a substring (`¹` ≠ `¹²`).

---

## Task 1: Companion-name scrub (strings + comments + test rename)

**Files:**
- Modify: `main.go` (6 status-string sites), `manifest.go` (comments), `manuscript.go` (comment), `source.go` (comment)
- Modify: `manifest_writers_test.go` (rename test + its comment)

**Interfaces:**
- Consumes: nothing.
- Produces: nothing new; a grep-clean tree (no companion-app names remain in `*.go`).

- [ ] **Step 1: Rewrite the 6 user-facing status strings in main.go**

Find each status string that names the companion app (grep for the companion-app name in main.go)
and replace with a neutral phrasing:
- `"manifest.json is managed by <companion-app>"` → `"manifest.json is read-only (managed externally)"` (both sites)
- `"manifest unreadable — structure is managed by <companion-app>"` → `"manifest unreadable — structure is read-only (external manifest)"` (both sites)
- `"chapter files are managed by <companion-app>"` → `"chapter files are read-only (external manifest)"`

Use `grep -n 'companion-app-name' main.go` (substituting the actual name) to find all sites; there are 6. Replace each string literal; keep the surrounding code identical.

- [ ] **Step 2: Reword code comments in manifest.go, manuscript.go, source.go**

Replace companion-app names in comments with "the companion app" or "the external owner of the manifest". Keep every technical claim (schema v1, sorted keys, no trailing newline, `[]`-not-null, id/name/kind/path). Example: `// <companion-app>'s manuscript folder` → `// the companion app's manuscript folder`; `matches <companion-app>'s JSONEncoder(...)` → `matches the companion app's JSONEncoder(...)`.

- [ ] **Step 3: Rename the serialization test**

In `manifest_writers_test.go`: rename the serialization test (which was named after the companion app) → `TestWriteManifestSortedKeys`, and reword its doc comment to reference "the companion app's" serialization instead of the old name. Leave the assertions and the `t.Fatalf` message wording that references the JSONEncoder behavior; genericize it if it names the companion app. Keep the `null`-vs-`[]` comment's meaning.

- [ ] **Step 4: Verify grep-clean + suite green**

Run:
```
grep -rniE '<companion-app-name>' --include='*.go' .
/opt/homebrew/bin/gofmt -w main.go manifest.go manuscript.go source.go manifest_writers_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: the grep prints nothing; build/test/vet all pass; `TestWriteManifestSortedKeys` runs.

- [ ] **Step 5: Commit**

```
git add main.go manifest.go manuscript.go source.go manifest_writers_test.go
git commit -m "scrub: genericize companion-app name in status strings + comments"
```

---

## Task 2: `footnotesToSidenotes` — split footnotes from body

**Files:**
- Modify: `preview.go` (add the function + a shared code-mask helper)
- Test: `preview_test.go` (create if absent)

**Interfaces:**
- Consumes: the existing `fnDef`, `fnRef`, `codeMask` regexes and `superscript(n int) string` in preview.go.
- Produces: `func footnotesToSidenotes(orig string) (body string, notes []string)` — body has superscript refs in place and NO appended Notes section; `notes[i]` is the text for marker `superscript(i+1)`, in first-reference order. Empty `notes` when the doc has no referenced footnotes.

- [ ] **Step 1: Write the failing test**

Add to `preview_test.go`:
```go
package main

import (
	"strings"
	"testing"
)

func TestFootnotesToSidenotesSplitsNotes(t *testing.T) {
	src := "Alpha[^a] and beta[^b].\n\n[^a]: first note\n[^b]: second note\n"
	body, notes := footnotesToSidenotes(src)
	if len(notes) != 2 {
		t.Fatalf("want 2 notes, got %d: %v", len(notes), notes)
	}
	if notes[0] != "first note" || notes[1] != "second note" {
		t.Fatalf("notes out of order: %v", notes)
	}
	if strings.Contains(body, "Notes") || strings.Contains(body, "[^a]") {
		t.Fatalf("body should have no Notes section and no raw refs: %q", body)
	}
	if !strings.Contains(body, superscript(1)) || !strings.Contains(body, superscript(2)) {
		t.Fatalf("body missing superscript refs: %q", body)
	}
}

func TestFootnotesToSidenotesNoFootnotes(t *testing.T) {
	body, notes := footnotesToSidenotes("Just prose, no notes.\n")
	if len(notes) != 0 {
		t.Fatalf("want 0 notes, got %v", notes)
	}
	if !strings.Contains(body, "Just prose") {
		t.Fatalf("body mangled: %q", body)
	}
}

func TestFootnotesToSidenotesIgnoresCodeAndOrphans(t *testing.T) {
	src := "See `arr[^1]` and real[^r].\n\n[^r]: real note\n"
	body, notes := footnotesToSidenotes(src)
	if len(notes) != 1 || notes[0] != "real note" {
		t.Fatalf("want 1 real note, got %v", notes)
	}
	if !strings.Contains(body, "arr[^1]") {
		t.Fatalf("code span footnote must stay literal: %q", body)
	}
}
```

- [ ] **Step 2: Run it to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestFootnotesToSidenotes -v`
Expected: FAIL (`undefined: footnotesToSidenotes`).

- [ ] **Step 3: Extract the shared mask helper + implement `footnotesToSidenotes`**

Refactor the code-masking block out of `footnotesToEndnotes` into a helper so both functions share it:
```go
// maskCode replaces fenced/inline code with sentinels so the footnote regexes never see
// inside it, and returns a restore func. Shared by footnotesToEndnotes and footnotesToSidenotes.
func maskCode(orig string) (masked string, restore func(string) string) {
	var code []string
	masked = codeMask.ReplaceAllStringFunc(orig, func(c string) string {
		code = append(code, c)
		return fmt.Sprintf("\x00CODE%d\x00", len(code)-1)
	})
	restore = func(s string) string {
		for i, c := range code {
			s = strings.Replace(s, fmt.Sprintf("\x00CODE%d\x00", i), c, 1)
		}
		return s
	}
	return masked, restore
}
```
Update `footnotesToEndnotes` to call `md, restore := maskCode(orig)` in place of its inline block (behavior identical — its existing tests must still pass). Then add:
```go
// footnotesToSidenotes splits GFM footnotes out of the body for margin rendering: it rewrites
// referenced [^id] to superscript markers in place (no endnote section) and returns the note
// texts in first-reference order. Empty notes slice when nothing is referenced. Code is masked.
func footnotesToSidenotes(orig string) (body string, notes []string) {
	md, restore := maskCode(orig)
	defMatches := fnDef.FindAllStringSubmatch(md, -1)
	if len(defMatches) == 0 {
		return restore(md), nil
	}
	defs := map[string]string{}
	for _, m := range defMatches {
		defs[m[1]] = strings.TrimSpace(m[2])
	}
	b := fnDef.ReplaceAllString(md, "") // drop definition lines
	var order []string
	num := map[string]int{}
	b = fnRef.ReplaceAllStringFunc(b, func(ref string) string {
		id := fnRef.FindStringSubmatch(ref)[1]
		if _, ok := defs[id]; !ok {
			return ref // orphan reference: keep literal
		}
		if _, seen := num[id]; !seen {
			order = append(order, id)
			num[id] = len(order)
		}
		return superscript(num[id])
	})
	b = strings.TrimRight(b, "\n")
	for _, id := range order {
		notes = append(notes, defs[id])
	}
	return restore(b), notes
}
```

- [ ] **Step 4: Run the tests (including the existing endnote tests)**

Run: `/opt/homebrew/bin/go test . -run 'Footnote|Endnote' -v` (and the full `preview.go` coverage).
Expected: PASS, and any pre-existing `footnotesToEndnotes` tests still PASS (the refactor is behavior-preserving).

- [ ] **Step 5: gofmt + build + commit**

```
/opt/homebrew/bin/gofmt -w preview.go preview_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
git add preview.go preview_test.go
git commit -m "feat(preview): footnotesToSidenotes + shared maskCode helper"
```

---

## Task 3: `layoutSidenotes` — float notes into the right gutter

**Files:**
- Modify: `preview.go` (add `layoutSidenotes` + small helpers)
- Test: `preview_test.go`

**Interfaces:**
- Consumes: `superscript(n int) string`; `github.com/charmbracelet/x/ansi` `StringWidth`.
- Produces: `func layoutSidenotes(body string, notes []string, measure, gutter int) string` — composes `padTo(bodyLine, measure) + " ┆ " + gutterLine` per row; a note anchors on the first body row whose superscript run equals its marker; notes cascade (no overlap); output extends past the body while gutter content remains.

- [ ] **Step 1: Write the failing test**

Add to `preview_test.go`:
```go
func TestLayoutSidenotesAnchorsOnRefRow(t *testing.T) {
	body := "line zero\nalpha " + superscript(1) + " here\nline two\n"
	out := layoutSidenotes(body, []string{"the note"}, 20, 12)
	lines := strings.Split(out, "\n")
	// The note must appear on the same row as the ¹ marker (row index 1), in the gutter.
	if !strings.Contains(lines[1], "┆") || !strings.Contains(lines[1], "the note") {
		t.Fatalf("note not on ref row:\n%s", out)
	}
	// Row 0 has a gutter divider but no note text.
	if !strings.Contains(lines[0], "┆") || strings.Contains(lines[0], "the note") {
		t.Fatalf("row 0 should be divider-only:\n%s", out)
	}
}

func TestLayoutSidenotesCascadeNoOverlap(t *testing.T) {
	// Two markers on adjacent rows; notes must not land on the same gutter row.
	body := "a " + superscript(1) + "\nb " + superscript(2) + "\n"
	out := layoutSidenotes(body, []string{"note one", "note two"}, 10, 12)
	lines := strings.Split(out, "\n")
	row1 := -1
	row2 := -1
	for i, ln := range lines {
		if strings.Contains(ln, "note one") {
			row1 = i
		}
		if strings.Contains(ln, "note two") {
			row2 = i
		}
	}
	if row1 == -1 || row2 == -1 || row1 == row2 {
		t.Fatalf("notes overlap or missing (row1=%d row2=%d):\n%s", row1, row2, out)
	}
}

func TestLayoutSidenotesMarkerIsWholeRun(t *testing.T) {
	// ¹ must not anchor inside ¹² (note 12's marker).
	body := "x " + superscript(12) + "\ny " + superscript(1) + "\n"
	notes := make([]string, 12)
	for i := range notes {
		notes[i] = "n" + superscript(i+1)
	}
	out := layoutSidenotes(body, notes, 10, 14)
	lines := strings.Split(out, "\n")
	// note 1 (n¹) should anchor on row 1 (the y-line), not row 0 (the ¹² line).
	for i, ln := range lines {
		if strings.Contains(ln, "n"+superscript(1)) && !strings.Contains(ln, "n"+superscript(12)) {
			if i == 0 {
				t.Fatalf("note 1 mis-anchored onto the ¹² row:\n%s", out)
			}
			break
		}
	}
}
```

- [ ] **Step 2: Run it to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestLayoutSidenotes -v`
Expected: FAIL (`undefined: layoutSidenotes`).

- [ ] **Step 3: Implement `layoutSidenotes`**

```go
import "github.com/charmbracelet/x/ansi" // add to preview.go imports

var sidenoteDivider = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a6f63")).Render("┆")
var sidenoteText = lipgloss.NewStyle().Foreground(lipgloss.Color("#704214"))

// padTo pads s with spaces to visible width w (ANSI-aware). Longer strings are returned as-is.
func padTo(s string, w int) string {
	n := w - ansi.StringWidth(s)
	if n <= 0 {
		return s
	}
	return s + strings.Repeat(" ", n)
}

// superscriptRuns returns the distinct maximal superscript runs on a line (as strings).
func superscriptRuns(line string) []string {
	isSup := func(r rune) bool {
		switch r {
		case '⁰', '¹', '²', '³', '⁴', '⁵', '⁶', '⁷', '⁸', '⁹':
			return true
		}
		return false
	}
	var runs []string
	var cur strings.Builder
	for _, r := range line {
		if isSup(r) {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() > 0 {
			runs = append(runs, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		runs = append(runs, cur.String())
	}
	return runs
}

// wrapPlain wraps s to width w on spaces (plain text — note bodies carry no ANSI).
func wrapPlain(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if len(line)+1+len(word) > w {
				out = append(out, line)
				line = word
			} else {
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return out
}

// layoutSidenotes composes body (glamour output, wrapped to `measure`) with `notes` floated into
// a right gutter of `gutter` columns, each anchored to the first row bearing its superscript
// marker and cascading downward so notes never overlap.
func layoutSidenotes(body string, notes []string, measure, gutter int) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	// gutterRows[i] is the note text (already numbered/wrapped) for output row i.
	gutterRows := map[int]string{}
	nextFree := 0
	anchorOf := func(marker string) int {
		for i, ln := range lines {
			for _, run := range superscriptRuns(ln) {
				if run == marker {
					return i
				}
			}
		}
		return -1
	}
	for n, text := range notes {
		marker := superscript(n + 1)
		anchor := anchorOf(marker)
		if anchor < 0 {
			anchor = nextFree
		}
		start := anchor
		if start < nextFree {
			start = nextFree
		}
		wrapped := wrapPlain(marker+" "+text, gutter)
		for j, wl := range wrapped {
			gutterRows[start+j] = wl
		}
		nextFree = start + len(wrapped) + 1 // blank row between notes
	}
	// Determine how many rows we render (body rows or further, if a note overflows).
	maxRow := len(lines) - 1
	for r := range gutterRows {
		if r > maxRow {
			maxRow = r
		}
	}
	var b strings.Builder
	for i := 0; i <= maxRow; i++ {
		bodyLine := ""
		if i < len(lines) {
			bodyLine = lines[i]
		}
		b.WriteString(padTo(bodyLine, measure))
		b.WriteString(" ")
		b.WriteString(sidenoteDivider)
		b.WriteString(" ")
		if g, ok := gutterRows[i]; ok {
			b.WriteString(sidenoteText.Render(g))
		}
		if i < maxRow {
			b.WriteString("\n")
		}
	}
	return b.String()
}
```
Confirm `lipgloss` is already imported in preview.go; if not, add it. (`fmt`, `strings` already are.)

- [ ] **Step 4: Run the tests**

Run: `/opt/homebrew/bin/go test . -run TestLayoutSidenotes -v`
Expected: PASS (all three).

- [ ] **Step 5: gofmt + build + commit**

```
/opt/homebrew/bin/gofmt -w preview.go preview_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
git add preview.go preview_test.go
git commit -m "feat(preview): layoutSidenotes — float footnotes into the right gutter"
```

---

## Task 4: Wire sidenotes into `renderPreview` + width gate + header

**Files:**
- Modify: `main.go` (`renderPreview` ~line 2130; the PREVIEW header ~line 1414)
- Test: `preview_test.go` (a seam test for the gate)

**Interfaces:**
- Consumes: `footnotesToSidenotes`, `layoutSidenotes`, `tufteGlamourStyle`, `m.previewTufte`, `m.preview.Width`, `m.colWidth`.
- Produces: `renderPreview` renders sidenotes when engaged; a helper `sidenoteGeometry(total int) (measure, gutter int, ok bool)` exposing the gate for testing.

- [ ] **Step 1: Write the failing test for the geometry gate**

Add to `preview_test.go`:
```go
func TestSidenoteGeometryGate(t *testing.T) {
	if _, _, ok := sidenoteGeometry(80); ok {
		t.Fatalf("width 80 (< 90) should not enable sidenotes")
	}
	measure, gutter, ok := sidenoteGeometry(120)
	if !ok {
		t.Fatalf("width 120 should enable sidenotes")
	}
	if gutter < 18 || gutter > 30 {
		t.Fatalf("gutter %d out of [18,30]", gutter)
	}
	if measure != 120-gutter-3 {
		t.Fatalf("measure %d != total-gutter-3", measure)
	}
}
```

- [ ] **Step 2: Run it to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestSidenoteGeometryGate -v`
Expected: FAIL (`undefined: sidenoteGeometry`).

- [ ] **Step 3: Add `sidenoteGeometry` and rewrite `renderPreview`**

Add near `renderPreview` in main.go:
```go
const sidenoteMinWidth = 90

// sidenoteGeometry returns the body measure and gutter width for the given total preview width,
// and whether the total is wide enough for margin sidenotes at all.
func sidenoteGeometry(total int) (measure, gutter int, ok bool) {
	if total < sidenoteMinWidth {
		return 0, 0, false
	}
	gutter = total / 3
	if gutter < 18 {
		gutter = 18
	}
	if gutter > 30 {
		gutter = 30
	}
	measure = total - gutter - 3 // " ┆ "
	return measure, gutter, true
}
```
Rewrite `renderPreview` so the Tufte-with-footnotes-and-width case takes the sidenote path; everything else is exactly as before:
```go
func (m *model) renderPreview() {
	wrap := m.preview.Width
	if wrap <= 0 {
		wrap = m.colWidth
	}
	// Tufte mode + wide terminal + footnotes → margin sidenotes.
	if m.previewTufte {
		if measure, gutter, ok := sidenoteGeometry(wrap); ok {
			if body, notes := footnotesToSidenotes(m.editor.Value()); len(notes) > 0 {
				r, err := glamour.NewTermRenderer(glamour.WithStyles(tufteGlamourStyle()), glamour.WithWordWrap(measure))
				if err != nil {
					m.status = "preview unavailable: " + err.Error()
					return
				}
				out, err := r.Render(body)
				if err != nil {
					m.status = "preview failed: " + err.Error()
					return
				}
				m.preview.SetContent(layoutSidenotes(out, notes, measure, gutter))
				m.sidenotesActive = true
				return
			}
		}
	}
	m.sidenotesActive = false
	styleOpt := glamour.WithStandardStyle(m.mdStyle)
	if m.previewTufte {
		styleOpt = glamour.WithStyles(tufteGlamourStyle())
	}
	r, err := glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(wrap))
	if err != nil {
		m.status = "preview unavailable: " + err.Error()
		return
	}
	out, err := r.Render(footnotesToEndnotes(m.editor.Value()))
	if err != nil {
		m.status = "preview failed: " + err.Error()
		return
	}
	m.preview.SetContent(out)
}
```
Add the `sidenotesActive bool` field to the model struct (near `previewTufte bool`, main.go ~211).

- [ ] **Step 4: Header hint (cosmetic)**

At the PREVIEW header (main.go ~1414), when `m.previewTufte`, append `· sidenotes` to the style label if `m.sidenotesActive`:
```go
style := "Default"
if m.previewTufte {
	style = "Tufte"
	if m.sidenotesActive {
		style = "Tufte · sidenotes"
	}
}
```

- [ ] **Step 5: Run tests + build + vet**

Run:
```
/opt/homebrew/bin/gofmt -w main.go preview.go preview_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: all pass, `TestSidenoteGeometryGate` PASS.

- [ ] **Step 6: Manual smoke (optional but recommended)**

Build and eyeball with a footnoted doc in a wide terminal: `ctrl+p` then `t` → notes should sit in the right gutter beside their refs. Narrow the terminal < 90 cols → falls back to endnotes.

- [ ] **Step 7: Commit**

```
git add main.go preview.go preview_test.go
git commit -m "feat(preview): render Tufte sidenotes in renderPreview behind a width gate"
```

---

## Task 5: README rewrite (standalone product + shortcuts table)

**Files:**
- Modify: `README.md` (full rewrite)

**Interfaces:**
- Consumes: the current `helpText` (main.go) for the shortcuts table; the env knobs okashi reads; the project model from CLAUDE.md.
- Produces: a GitHub-facing README with no companion-app name strings.

- [ ] **Step 1: Transcribe the current shortcuts + env knobs**

Read `helpText` in main.go and the env-var handling (`OKASHI_DIR`, `OKASHI_WIDTH`, `OKASHI_SMARTQUOTES`, `OKASHI_THEME`, `OKASHI_ICONS`, `OKASHI_AUTHOR`). The README's shortcuts table MUST match `helpText` exactly (same keys, same meanings); the env table MUST match the knobs the code reads.

- [ ] **Step 2: Write the README**

Rewrite `README.md` with these sections (see spec §3): Title + tagline; screenshot placeholder; Install (Homebrew placeholder marked TBD + `go build ./...` / `go run .`, Go 1.25); Quick start; **Keyboard shortcuts** (Markdown table grouped Navigation / Files / Writing / Export & preview / Search, transcribed from `helpText`); Project model (`.md` atom, manuscript = folder + `manifest.json`, category = plain folder, resources = unlisted files, legacy numbered folders read-only); Export (`ctrl+e` RTF+PDF, Manuscript/Tufte); Preview (`ctrl+p`, `t` toggles Tufte with margin **sidenotes** on wide terminals); Configuration (env table); Text selection (⌥/⇧+drag, ⌘C); License (match repo's existing license, else a placeholder line). No sibling-project references.

- [ ] **Step 3: Verify**

Run: `grep -niE '<companion-app-name>' README.md` → prints nothing (no companion-app names). Eyeball the shortcuts table against `helpText` (every row matches). Confirm the Markdown renders (headings, table pipes well-formed).

- [ ] **Step 4: Commit**

```
git add README.md
git commit -m "docs: rewrite README as a standalone product guide with a shortcuts table"
```

---

## Self-review notes
- **Spec coverage:** §1 sidenotes → Tasks 2–4; §2 scrub → Task 1; §3 README → Task 5. All covered.
- **Type consistency:** `footnotesToSidenotes(string) (string, []string)`, `layoutSidenotes(string, []string, int, int) string`, `sidenoteGeometry(int) (int, int, bool)`, `maskCode(string) (string, func(string) string)`, `superscriptRuns(string) []string`, `padTo(string, int) string`, `wrapPlain(string, int) []string`, model field `sidenotesActive bool`, const `sidenoteMinWidth = 90` — used consistently across tasks.
- **No placeholders in code steps:** every code step carries the actual code. The README's Homebrew/license TBDs are intended deliverable content (external facts), not plan placeholders.
- **Invariants:** Default + narrow-Tufte preview paths are preserved verbatim in Task 4; the endnote path and `footnotesToEndnotes` behavior are unchanged (Task 2 refactor is behavior-preserving, guarded by its existing tests).

---

## Task 6: Auto-widen the Tufte preview so sidenotes engage by terminal width (follow-up)

**Why:** the shipped gate used `m.preview.Width`, which `layout()` clamps to the writing measure (`min(colWidth, editorArea-2)`, default 72). So sidenotes never appear unless `OKASHI_WIDTH>=92`. Decouple the body **measure** (stays = writing measure) from the preview **pane width** (may grow to hold the margin), so Tufte sidenotes appear on any wide terminal at the default measure. Default mode and narrow-Tufte-endnote output stay identical.

**Files:**
- Modify: `main.go` (`layout()`, `renderPreview()`, the `WindowSizeMsg` handler at ~935, the model struct, the geometry helpers)
- Test: `preview_test.go`

**Interfaces:**
- Consumes: `footnotesToSidenotes`, `layoutSidenotes`, `tufteGlamourStyle`, `effectivePanels`.
- Produces: `sidenoteGeometry(avail, measure int) (gutter int, ok bool)` (NEW signature); `sidenotePlan(avail, colWidth int, buffer string) (measure, gutter int, body string, notes []string, ok bool)`; model field `previewAvail int`.

- [ ] **Step 1: Update the gate test to the new signature (write failing)**

Replace `TestSidenoteGeometryGate` in preview_test.go with:
```go
func TestSidenoteGeometryGate(t *testing.T) {
	// avail 80, measure 72 → gutter 5 < 18 → no sidenotes.
	if _, ok := sidenoteGeometry(80, 72); ok {
		t.Fatalf("avail 80 / measure 72 should not fit a sidenote margin")
	}
	// avail 96, measure 72 → gutter 21 → ok, within [18,30].
	gutter, ok := sidenoteGeometry(96, 72)
	if !ok {
		t.Fatalf("avail 96 / measure 72 should fit sidenotes")
	}
	if gutter < 18 || gutter > 30 {
		t.Fatalf("gutter %d out of [18,30]", gutter)
	}
	// avail 200 → gutter clamped to 30.
	if g, _ := sidenoteGeometry(200, 72); g != 30 {
		t.Fatalf("gutter should clamp to 30, got %d", g)
	}
}

func TestSidenotePlanEngagesOnlyWithRoomAndNotes(t *testing.T) {
	doc := "Alpha[^a] and beta[^b].\n\n[^a]: first\n[^b]: second\n"
	// Wide pane + footnotes → engaged, body measure stays 72.
	measure, gutter, _, notes, ok := sidenotePlan(120, 72, doc)
	if !ok || len(notes) != 2 || measure != 72 || gutter < 18 || gutter > 30 {
		t.Fatalf("wide+footnotes should engage: measure=%d gutter=%d notes=%d ok=%v", measure, gutter, len(notes), ok)
	}
	// Narrow pane → not engaged even with footnotes.
	if _, _, _, _, ok := sidenotePlan(80, 72, doc); ok {
		t.Fatalf("narrow pane should not engage sidenotes")
	}
	// Wide pane, no footnotes → not engaged.
	if _, _, _, _, ok := sidenotePlan(120, 72, "just prose\n"); ok {
		t.Fatalf("no footnotes should not engage sidenotes")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run 'TestSidenoteGeometryGate|TestSidenotePlan' -v`
Expected: FAIL (signature mismatch / `undefined: sidenotePlan`).

- [ ] **Step 3: Rework the geometry helpers**

In main.go, replace the old `const sidenoteMinWidth = 90` + `sidenoteGeometry(total)` with:
```go
const (
	sidenoteMinGutter = 18
	sidenoteMaxGutter = 30
)

// sidenoteGeometry reports the gutter width for a preview pane of `avail` columns holding a body
// of `measure` columns plus the 3-col " ┆ " gap, and whether a margin (>= sidenoteMinGutter) fits.
func sidenoteGeometry(avail, measure int) (gutter int, ok bool) {
	gutter = avail - measure - 3
	if gutter < sidenoteMinGutter {
		return 0, false
	}
	if gutter > sidenoteMaxGutter {
		gutter = sidenoteMaxGutter
	}
	return gutter, true
}

// sidenotePlan decides whether the Tufte preview should render margin sidenotes: the body measure
// stays the writing measure (colWidth), the gutter uses the pane's spare width, and it engages only
// when the pane is wide enough AND the doc has referenced footnotes. body/notes come from
// footnotesToSidenotes (body has superscript refs, no endnote section).
func sidenotePlan(avail, colWidth int, buffer string) (measure, gutter int, body string, notes []string, ok bool) {
	measure = colWidth
	if avail > 0 && avail < measure {
		measure = avail // tiny panes: never exceed the available width
	}
	g, gok := sidenoteGeometry(avail, measure)
	if !gok {
		return 0, 0, "", nil, false
	}
	b, ns := footnotesToSidenotes(buffer)
	if len(ns) == 0 {
		return 0, 0, "", nil, false
	}
	return measure, g, b, ns, true
}
```

- [ ] **Step 4: Store `previewAvail` in `layout()` + add the model field**

Add `previewAvail int` to the model struct (next to `sidenotesActive bool`). In `layout()` (main.go ~1508), after `cw := min(m.colWidth, editorArea-2)`:
```go
	m.previewAvail = editorArea - 2
	if m.previewAvail < 0 {
		m.previewAvail = 0
	}
```
Keep the existing `m.preview.Width = cw` line (renderPreview overrides it when sidenotes engage; this is the default/fallback width).

- [ ] **Step 5: Rewrite `renderPreview` to use the plan + widen the pane**

Replace `renderPreview` with:
```go
func (m *model) renderPreview() {
	if m.previewTufte {
		if measure, gutter, body, notes, ok := sidenotePlan(m.previewAvail, m.colWidth, m.editor.Value()); ok {
			r, err := glamour.NewTermRenderer(glamour.WithStyles(tufteGlamourStyle()), glamour.WithWordWrap(measure))
			if err != nil {
				m.status = "preview unavailable: " + err.Error()
				return
			}
			out, err := r.Render(body)
			if err != nil {
				m.status = "preview failed: " + err.Error()
				return
			}
			m.preview.Width = measure + 3 + gutter // widen the pane to hold the margin
			m.preview.SetContent(layoutSidenotes(out, notes, measure, gutter))
			m.sidenotesActive = true
			return
		}
	}
	m.sidenotesActive = false
	wrap := min(m.colWidth, m.previewAvail)
	if wrap <= 0 {
		wrap = m.colWidth // pre-layout (previewAvail not set yet)
	}
	m.preview.Width = wrap
	styleOpt := glamour.WithStandardStyle(m.mdStyle)
	if m.previewTufte {
		styleOpt = glamour.WithStyles(tufteGlamourStyle())
	}
	r, err := glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(wrap))
	if err != nil {
		m.status = "preview unavailable: " + err.Error()
		return
	}
	out, err := r.Render(footnotesToEndnotes(m.editor.Value()))
	if err != nil {
		m.status = "preview failed: " + err.Error()
		return
	}
	m.preview.SetContent(out)
}
```
Note: the fallback `wrap = min(colWidth, previewAvail)` equals the old `cw`, so Default and narrow-Tufte output and pane width are identical to before.

- [ ] **Step 6: Re-render the preview on resize (fixes stale/clipped content)**

In the `tea.WindowSizeMsg` handler (main.go ~935–938), after `m.layout()`, add:
```go
		if m.previewing {
			m.renderPreview()
		}
```
This keeps the sidenote pane width in sync when the terminal resizes (previously the content went stale).

- [ ] **Step 7: Run all tests + build + vet**

Run:
```
/opt/homebrew/bin/gofmt -w main.go preview.go preview_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: all pass; the two new/updated tests PASS.

- [ ] **Step 8: Commit**

```
git add main.go preview.go preview_test.go
git commit -m "feat(preview): auto-widen Tufte preview so sidenotes engage by terminal width"
```

---

## Task 7: README accuracy — sidenote trigger wording + T5 nits

**Files:**
- Modify: `README.md`

**Interfaces:** none.

- [ ] **Step 1: Fix the Preview section wording**

The Preview section currently says sidenotes engage "on wide terminals (≥ 90 columns)" and implies a column threshold. Reword to reflect Task 6: in Tufte mode (`t`), when the terminal is wide enough to hold the text plus a margin, footnotes render as **margin sidenotes**; on a narrow terminal they fall back to numbered endnotes. Do NOT mention `OKASHI_WIDTH` as the trigger (it is no longer the gate). The body measure stays the writing measure.

- [ ] **Step 2: Move `ctrl+c` to Navigation**

In the shortcuts table, move the `ctrl+c` (quit) row out of the Writing group into Navigation (or a standalone row). It quits the app, not a writing action.

- [ ] **Step 3: Restore the autosave note**

Add a short line (in Quick start or a small "Saving" note) describing the shipped behavior: okashi autosaves as you write and shows a save indicator; `ctrl+s` saves explicitly. (This shipped feature was dropped in the rewrite.)

- [ ] **Step 4: Verify + commit**

Run: `grep -niE '<companion-app-name>' README.md` (must be empty — no companion-app names). Eyeball the shortcuts table still matches `helpText` (minus the regrouping) and that the Preview wording matches Task 6 behavior.
```
git add README.md
git commit -m "docs: correct sidenote trigger wording + ctrl+c grouping + autosave note"
```
