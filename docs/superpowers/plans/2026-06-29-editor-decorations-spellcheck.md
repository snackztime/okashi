# Editor Decorations + Spellcheck Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

**Goal:** Add a general per-line **Decorator** hook to the vendored editor (styling arbitrary rune-ranges over the visible window), and its first consumer: **spellcheck** (embedded wordlist → red underline on misspellings), toggled from a new inspector **Analysis** tab.

**Architecture:** `internal/textarea` gains `Decorator func(line string) []Decoration`, called once per VISIBLE source line in the windowed `View`; `renderSeg` splits each piece by dim span AND decorations (precedence decoration > dim > normal). okashi's `spellDecorator` (embedded `assets/words.txt`) is set as the editor's `Decorator` when the Analysis-tab Spellcheck checkbox is on.

**Tech Stack:** Go, lipgloss, `//go:embed`. All pure-Go (no cgo/network).

**Design spec:** `docs/superpowers/specs/2026-06-29-editor-decorations-spellcheck-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- `Decorator == nil` → editor render BYTE-IDENTICAL to today (the gate). The existing `internal/textarea` tests (`dim_test`, `editing_test`, `moveline_test`, `typewriter_test`) MUST stay green unchanged.
- Decoration cost is O(visible): `Decorator` is called only for visible source lines (inside the windowed `View` loop).
- `assets/words.txt` already exists (234k lowercased words). Spellcheck: pure Go, lazy-loaded, no suggestions.
- Inspector `View` currently is `View(width int, doc docStats, proj projStats, outline string, goals goalStats) string` — this cycle adds a 6th `analysis analysisState` arg; update ALL call sites.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Editor decoration hook + styled-run render (`internal/textarea`)

**Files:** Modify `internal/textarea/textarea.go`, `internal/textarea/dim.go`; Test `internal/textarea/decoration_test.go`

**Interfaces:**
- Produces: `type Decoration struct { Start, End int; Style lipgloss.Style }`; `Decorator func(line string) []Decoration` field on `Model`; generalized `renderSeg` that also applies decorations.

- [ ] **Step 1: Write the failing test** — create `internal/textarea/decoration_test.go`:

```go
package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDecoratorStylesRange(t *testing.T) {
	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("alpha bravo charlie")
	// Underline "bravo" (runes 6..11).
	m.Decorator = func(line string) []Decoration {
		i := strings.Index(line, "bravo")
		if i < 0 {
			return nil
		}
		return []Decoration{{Start: i, End: i + len("bravo"), Style: lipgloss.NewStyle().Underline(true)}}
	}
	out := m.View()
	if !strings.Contains(out, "bravo") {
		t.Fatal("decorated word should still be present")
	}
	// The underline SGR (4) must appear, and only around bravo (alpha/charlie plain).
	if !strings.Contains(out, "\x1b[4m") && !strings.Contains(out, ";4m") {
		t.Fatalf("expected an underline SGR around the decorated range:\n%q", out)
	}
}

func TestNilDecoratorUnchanged(t *testing.T) {
	// With no Decorator, the rendered output must equal the dim/plain render
	// (sanity: View doesn't panic and contains the text).
	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.SetWidth(40)
	m.SetHeight(6)
	m.Focus()
	m.SetValue("plain text here")
	if m.Decorator != nil {
		t.Fatal("Decorator should default to nil")
	}
	if !strings.Contains(m.View(), "plain text here") {
		t.Fatal("nil-decorator render should contain the text unchanged")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test ./internal/textarea/ -run 'TestDecorator|TestNilDecorator' 2>&1 | tail` → `Decoration`/`Decorator` undefined.

- [ ] **Step 3: Add the type + field + generalize the run splitter**

In `internal/textarea/textarea.go`, add (near the `Dim`/`DimStyle` fields):

```go
	// Decorator, when set, is called once per visible source line during View and
	// returns rune-range decorations to style (spellcheck/syntax). nil → no
	// decorations (render unchanged). okashi:decorations
	Decorator func(line string) []Decoration
```

And the type (top-level):

```go
// Decoration styles a [Start,End) rune range within a source line.
type Decoration struct {
	Start, End int
	Style      lipgloss.Style
}
```

In `internal/textarea/dim.go`, generalize the run splitter so each run carries the lipgloss style to use. Replace `splitDimRuns` + `dimRun` with a style-aware splitter (keep the old name as a thin wrapper if other code calls it, else update the one caller in `renderSeg`):

```go
type styledRun struct {
	text  string
	style lipgloss.Style
}

// splitStyledRuns splits seg (first rune at absolute offset absStart) into runs,
// choosing each rune's style by precedence: a covering decoration > dim > normal.
// decos are ABSOLUTE-offset ranges (the View converts line-relative → absolute).
func splitStyledRuns(seg []rune, absStart, span0, span1 int, dimOn bool, normal, dimStyle lipgloss.Style, decos []Decoration) []styledRun {
	styleAt := func(off int) lipgloss.Style {
		for _, d := range decos {
			if off >= d.Start && off < d.End {
				return d.Style
			}
		}
		if dimOn && (off < span0 || off >= span1) {
			return dimStyle
		}
		return normal
	}
	var runs []styledRun
	i := 0
	for i < len(seg) {
		st := styleAt(absStart + i)
		j := i + 1
		for j < len(seg) && styleAt(absStart+j).String() == st.String() {
			j++
		}
		runs = append(runs, styledRun{text: string(seg[i:j]), style: st})
		i = j
	}
	return runs
}
```

Rewrite `renderSeg` to use it and accept the line's absolute decorations. Change its signature to `renderSeg(seg []rune, absStart, span0, span1 int, style lipgloss.Style, decos []Decoration) string`:

```go
func (m Model) renderSeg(seg []rune, absStart, span0, span1 int, style lipgloss.Style, decos []Decoration) string {
	if !m.Dim && len(decos) == 0 {
		return style.Render(string(seg)) // fast path, unchanged
	}
	var b strings.Builder
	for _, run := range splitStyledRuns(seg, absStart, span0, span1, m.Dim, style, m.DimStyle, decos) {
		b.WriteString(run.style.Render(run.text))
	}
	return b.String()
}
```

(When `decos` is empty this produces byte-identical output to the old dim path — runs grouped by dim/normal, styled the same — so the existing dim tests stay green.)

- [ ] **Step 4: Call the Decorator per visible line + thread decos to renderSeg in `View`**

In `View`, inside the per-source-line loop (where `wrappedLines := m.memoizedWrap(line, m.width)` is computed, before the wrapped-piece loop), compute the line's absolute decorations once:

```go
		var lineDecos []Decoration
		if m.Decorator != nil {
			for _, d := range m.Decorator(string(line)) {
				lineDecos = append(lineDecos, Decoration{Start: lineOffset + d.Start, End: lineOffset + d.End, Style: d.Style})
			}
		}
```

Pass `lineDecos` to every `renderSeg(...)` call in the piece loop (the three call sites at ~textarea.go:1308/1315/1318 each gain a trailing `, lineDecos` arg). The decorations are absolute, so they apply correctly to any wrapped piece via the existing `absStart`.

- [ ] **Step 5: Run the new tests AND all existing textarea tests**

```bash
/opt/homebrew/bin/go test ./internal/textarea/ -v 2>&1 | tail -40
```
Expected: new decoration tests PASS and EVERY existing test (dim/editing/moveline/typewriter) still PASSES. If an existing test fails, the undecorated render diverged — fix the splitter, do NOT edit the tests.

- [ ] **Step 6: gofmt; full suite; build; commit**

```bash
/opt/homebrew/bin/gofmt -w internal/textarea/textarea.go internal/textarea/dim.go internal/textarea/decoration_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add internal/textarea/textarea.go internal/textarea/dim.go internal/textarea/decoration_test.go
git commit -m "textarea: per-line Decorator hook + styled-run render (decoration > dim > normal)"
```

---

## Task 2: Spellcheck decorator + embedded wordlist (`spell.go`)

**Files:** Create `spell.go`, `spell_test.go`; add `assets/words.txt` (already on disk)

**Interfaces:**
- Consumes: `textarea.Decoration` (Task 1), `red`-ish color.
- Produces: `spellDecorator(line string) []textarea.Decoration`.

- [ ] **Step 1: Write the failing test** — create `spell_test.go`:

```go
package main

import "testing"

func TestSpellDecorator(t *testing.T) {
	decos := spellDecorator("teh quikc brown fox")
	// "teh" and "quikc" are misspelled; "brown"/"fox" are real words.
	if len(decos) != 2 {
		t.Fatalf("expected 2 misspellings (teh, quikc), got %d: %+v", len(decos), decos)
	}
	// First decoration covers "teh" (0..3).
	if decos[0].Start != 0 || decos[0].End != 3 {
		t.Fatalf("first misspelling span = [%d,%d), want [0,3)", decos[0].Start, decos[0].End)
	}
	// A correctly-spelled line yields nothing.
	if d := spellDecorator("the quick brown fox"); len(d) != 0 {
		t.Fatalf("correct line should have no decorations, got %+v", d)
	}
	// Short words and all-caps are skipped.
	if d := spellDecorator("OK is a NASA go"); len(d) != 0 {
		t.Fatalf("short/all-caps words should be skipped, got %+v", d)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestSpellDecorator 2>&1 | tail` → `spellDecorator` undefined.

- [ ] **Step 3: Create `spell.go`:**

```go
package main

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"okashi/internal/textarea"
)

//go:embed assets/words.txt
var wordsFile string

var (
	spellOnce sync.Once
	spellSet  map[string]struct{}
)

func loadSpellSet() {
	spellSet = make(map[string]struct{}, 240000)
	for _, w := range strings.Split(wordsFile, "\n") {
		if w != "" {
			spellSet[w] = struct{}{}
		}
	}
}

var misspellStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")).Underline(true)

// wordSpans returns [start,end) rune ranges of word tokens (letters + apostrophe).
func wordSpans(line string) [][2]int {
	var spans [][2]int
	runes := []rune(line)
	i := 0
	for i < len(runes) {
		if unicode.IsLetter(runes[i]) {
			j := i + 1
			for j < len(runes) && (unicode.IsLetter(runes[j]) || runes[j] == '\'') {
				j++
			}
			spans = append(spans, [2]int{i, j})
			i = j
		} else {
			i++
		}
	}
	return spans
}

// spellDecorator flags misspelled words (red underline). Words in the embedded
// list, shorter than 3 letters, or all-caps (acronyms) are skipped.
func spellDecorator(line string) []textarea.Decoration {
	spellOnce.Do(loadSpellSet)
	var decos []textarea.Decoration
	runes := []rune(line)
	for _, s := range wordSpans(line) {
		w := runes[s[0]:s[1]]
		word := strings.Trim(string(w), "'")
		if len([]rune(word)) < 3 {
			continue
		}
		if word == strings.ToUpper(word) { // all-caps acronym
			continue
		}
		if _, ok := spellSet[strings.ToLower(word)]; ok {
			continue
		}
		decos = append(decos, textarea.Decoration{Start: s[0], End: s[1], Style: misspellStyle})
	}
	return decos
}
```

- [ ] **Step 4: Run tests; gofmt; commit (include the asset):**

```bash
/opt/homebrew/bin/go test . -run TestSpellDecorator -v 2>&1 | tail -15
/opt/homebrew/bin/gofmt -w spell.go spell_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add spell.go spell_test.go assets/words.txt
git commit -m "spell: embedded wordlist + spellDecorator (misspelled words → red underline)"
```

---

## Task 3: Analysis tab (`inspector.go`)

**Files:** Modify `inspector.go`; Test `inspector_test.go`

**Interfaces:**
- Produces: `tabAnalysis`; `inspectorTabLabels()` → `{"Words","Outline","Goals","Analysis"}`; `type analysisState struct{ spell, syntax bool }`; `inspectorAnalysisRowAtY(localY int) (int, bool)`; `View(... , analysis analysisState)`.

- [ ] **Step 1: Write the failing test** — add to `inspector_test.go`:

```go
func TestInspectorAnalysisTab(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	on := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, syntax: false})
	if !strings.Contains(on, "Spellcheck") || !strings.Contains(on, "Syntax") {
		t.Fatalf("analysis tab should list Spellcheck and Syntax:\n%s", on)
	}
	if !strings.Contains(on, "[x] Spellcheck") {
		t.Fatalf("spell on → checked box:\n%s", on)
	}
	off := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{})
	if !strings.Contains(off, "[ ] Spellcheck") {
		t.Fatalf("spell off → empty box:\n%s", off)
	}
}

func TestInspectorAnalysisRowAtY(t *testing.T) {
	// Body rows after the tab bar + blank + header: Spellcheck row, Syntax row.
	if r, ok := inspectorAnalysisRowAtY(spellRowY); !ok || r != 0 {
		t.Fatalf("spellRowY → row %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(spellRowY + 1); !ok || r != 1 {
		t.Fatalf("syntax row → %d ok=%v, want 1", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}
```

Also UPDATE every existing `in.View(...)` call in `inspector_test.go` to add the 6th arg `, analysisState{}`.

- [ ] **Step 2: Run to verify it fails** — undefined `tabAnalysis`/`analysisState`/`inspectorAnalysisRowAtY`/`spellRowY` + arity errors.

- [ ] **Step 3: Update `inspector.go`:**

Tab const + labels:

```go
const (
	tabWords inspectorTab = iota
	tabOutline
	tabGoals
	tabAnalysis
)

func inspectorTabLabels() []string { return []string{"Words", "Outline", "Goals", "Analysis"} }
```

Analysis state + the body-row geometry constant + hit-test:

```go
type analysisState struct{ spell, syntax bool }

// spellRowY is the body row (within the inspector, y from the top) of the
// Spellcheck checkbox: tab-bar(0) + blank(1) + "Analysis"(2) + blank(3) → 4.
const spellRowY = 4

func inspectorAnalysisRowAtY(localY int) (int, bool) {
	row := localY - spellRowY
	if row == 0 || row == 1 {
		return row, true
	}
	return 0, false
}

func checkbox(on bool) string {
	if on {
		return "[x] "
	}
	return "[ ] "
}
```

Add the `analysis analysisState` param to `View` and a `tabAnalysis` body (the other cases unchanged):

```go
	case tabAnalysis:
		b.WriteString(breadcrumbStyle.Render("Analysis") + "\n\n")
		b.WriteString(checkbox(analysis.spell) + "Spellcheck\n")
		b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(checkbox(analysis.syntax) + "Syntax"))
```

The `View` signature becomes `View(width int, doc docStats, proj projStats, outline string, goals goalStats, analysis analysisState) string`. Keep Words/Outline/Goals bodies unchanged. (Syntax row is dimmed `subtle` — wired next cycle.)

- [ ] **Step 4: Run tests; gofmt; commit:**

```bash
/opt/homebrew/bin/go test . -run 'TestInspectorAnalysis|TestInspector' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w inspector.go inspector_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add inspector.go inspector_test.go
git commit -m "inspector: Analysis tab (Spellcheck/Syntax checkboxes) + row hit-test"
```

---

## Task 4: Wire spellcheck toggle (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces:** Consumes `spellDecorator` (Task 2), `analysisState`/`tabAnalysis`/`inspectorAnalysisRowAtY` (Task 3), `textarea.Model.Decorator`.

- [ ] **Step 1: Write the failing test** — add to `smoke_test.go`:

```go
func TestSpellcheckToggleViaAnalysisClick(t *testing.T) {
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
	if m.analysis.spell {
		t.Fatal("spellcheck should default off")
	}
	// Click the Spellcheck checkbox row in the inspector body.
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: spellRowY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.spell {
		t.Fatal("clicking the Spellcheck row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("enabling spellcheck should set the editor Decorator")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — undefined `m.analysis`.

- [ ] **Step 3: Add the field + apply helper**

Add to the `model` struct: `analysis analysisState`. Add a helper:

```go
// applyDecorator sets the editor's Decorator from the current analysis toggles.
// (Syntax composes here in the next cycle; for now only spellcheck.)
func (m *model) applyDecorator() {
	if m.analysis.spell {
		m.editor.Decorator = spellDecorator
	} else {
		m.editor.Decorator = nil
	}
}
```

Call `m.applyDecorator()` at the end of `loadFile` (so a newly-opened chapter keeps the setting).

- [ ] **Step 4: Handle the Analysis checkbox click in `MouseMsg`**

In the `case tea.MouseMsg:` block, after the existing inspector tab-click handling (Task 4 of the Goals cycle), add a checkbox-row click for the Analysis tab:

```go
		if showInspector && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && m.inspector.tab == tabAnalysis {
			localX := msg.X - (m.width - inspectorWidth) - 2
			if localX >= 0 {
				if row, ok := inspectorAnalysisRowAtY(msg.Y); ok {
					if row == 0 {
						m.analysis.spell = !m.analysis.spell
						m.applyDecorator()
					}
					// row 1 (Syntax) is wired next cycle.
					return m, nil
				}
			}
		}
```

(Place it so it returns on a checkbox hit but falls through otherwise; it requires `m.inspector.tab == tabAnalysis` and a body-row Y, so it won't collide with the tab-row (Y==0) click handler above it.)

- [ ] **Step 5: Pass `m.analysis` to the inspector View**

Where the inspector column is built in `View()`, add the 6th arg:

```go
		insInner := m.inspector.View(inspectorWidth-3, computeDocStats(m.editor.Value()), proj, readOutlineDoc(m.files.dir), gs, m.analysis)
```

(Keep the single `proj`/`gs` computations from the Goals cycle.)

- [ ] **Step 6: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestSpellcheckToggle|TestInspectorTabClick|TestInspectorToggle' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w main.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add main.go smoke_test.go
git commit -m "spell: Analysis-tab Spellcheck checkbox toggles the editor decorator"
```

---

## Self-Review

**Spec coverage:** decoration hook + styled-run render + per-line call + nil-gate → Task 1; embedded wordlist + spellDecorator (skip rules) → Task 2; Analysis tab + checkboxes + row hit-test → Task 3; spell toggle wiring (Decorator set on click + loadFile) + click handler + View arg → Task 4.

**Placeholder scan:** none — full code throughout. The Syntax row is intentionally inert/dimmed this cycle (noted), not a placeholder.

**Type consistency:** `Decoration{Start,End,Style}` (Task 1) consumed by `spellDecorator` (Task 2) and the inspector is unaffected by it; `analysisState{spell,syntax}`, `tabAnalysis`, `inspectorAnalysisRowAtY`, `spellRowY`, `View(...6 args)` (Task 3) consumed by main.go (Task 4); `renderSeg` gains a `decos` param and all three call sites in `View` are updated (Task 1 Step 4); all `inspector.View` call sites updated to 6 args (Task 3 test + Task 4 main.go).

**Risk note (executor):** Task 1 is editor-core surgery on the windowed render. The existing `internal/textarea` tests assert the rendered output — they are the byte-identity gate for the `Decorator==nil`/no-decos path; never edit them to pass. Implement Task 1 on the most capable model. `splitStyledRuns` compares styles via `.String()` to group runs — ensure the normal vs dim vs decoration styles are distinct objects so grouping is correct.
