# Parts-of-Speech Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

> **BYTE→RUNE TRAP (again):** prose token offsets come from BYTE `strings.Index`; `Decoration` needs RUNE offsets. Convert via `len([]rune(line[:byteOff]))` and TEST a multibyte line.

**Goal:** Replace the markdown `syntaxDecorator` with a parts-of-speech highlighter (adverbs/adjectives/passive) via `prose/v2`, and rework the Analysis tab into Spellcheck + a "Syntax" POS-toggle list — fixing the tab-bar wrap + click misalignment.

**Architecture:** `pos.go` tags each visible line with prose (memoized); `posDecorator(line, adverb, adjective, passive)` styles the active categories. Task 2 atomically swaps the Analysis UI to a POS list (and compacts the tab bar), changes `analysisState`, rewires `applyDecorator`/clicks, and deletes the markdown decorator — so the package always compiles.

**Tech Stack:** Go, `github.com/jdkato/prose/v2` (pinned), lipgloss, the `textarea.Decoration` hook.

**Design spec:** `docs/superpowers/specs/2026-06-29-pos-highlighting-design.md`.

## Global Constraints

- Module `okashi`; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`. Network available for `go get`.
- Decoration offsets are RUNE indices (byte→rune conversion; multibyte test).
- prose: `prose.NewDocument(line, prose.WithExtraction(false), prose.WithSegmentation(false))` → `doc.Tokens()` → `tok.Text`, `tok.Tag`. Verified tags: adverb `RB*`, adjective `JJ*`, past participle `VBN`.
- Categories: Adverb (yellow `#f1fa8c`), Adjective (cyan `#8be9fd`), Passive/weak (orange `#ffb86c`; be-verb + following `VBN`). Toggles session-only.
- Compose spellcheck spans FIRST (spell wins overlaps).
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: prose dep + POS engine (`pos.go`) — additive, self-contained

**Files:** Create `pos.go`, `pos_test.go`; modify `go.mod`/`go.sum`. (Do NOT touch `syntax.go`/`analysisState` here — the package stays compiling.)

**Interfaces:**
- Produces: `type posToken struct{ text, tag string; start, end int }`; `posTokens(line string) []posToken`; `posDecorator(line string, adverb, adjective, passive bool) []textarea.Decoration`. (Plain bools — NO `analysisState` dependency, so this task is self-contained.)

- [ ] **Step 1: Add prose**

```bash
cd /Users/michael/dev/okashi
/opt/homebrew/bin/go get github.com/jdkato/prose/v2@v2.0.0
```

- [ ] **Step 2: Write the failing test** — create `pos_test.go`:

```go
package main

import "testing"

func hasPosSpan(d []textarea.Decoration, s, e int) bool {
	for _, x := range d {
		if x.Start == s && x.End == e {
			return true
		}
	}
	return false
}

func TestPosDecorator(t *testing.T) {
	if d := posDecorator("She quickly ran", true, false, false); !hasPosSpan(d, 4, 11) {
		t.Fatalf("adverb 'quickly' [4,11) missing: %+v", d)
	}
	if d := posDecorator("the red car", false, true, false); !hasPosSpan(d, 4, 7) {
		t.Fatalf("adjective 'red' [4,7) missing: %+v", d)
	}
	dp := posDecorator("it was written", false, false, true)
	if !hasPosSpan(dp, 3, 6) || !hasPosSpan(dp, 7, 14) {
		t.Fatalf("passive 'was'+'written' spans missing: %+v", dp)
	}
	if d := posDecorator("She quickly ran", false, false, false); len(d) != 0 {
		t.Fatalf("no categories → no spans: %+v", d)
	}
}

func TestPosMultibyteOffsets(t *testing.T) {
	// "x — slowly" → x(0) sp(1) —(2) sp(3) slowly(4..10); rune offsets, not bytes.
	if d := posDecorator("x — slowly", true, false, false); !hasPosSpan(d, 4, 10) {
		t.Fatalf("adverb after em-dash should be rune [4,10): %+v", d)
	}
}

func TestPosMemoized(t *testing.T) {
	posTokens("warm up the cache please")
	if len(posCache) == 0 {
		t.Fatal("posTokens should populate posCache")
	}
}
```

- [ ] **Step 3: Run to verify it fails** — `/opt/homebrew/bin/go test . -run TestPos 2>&1 | tail` → undefined `posDecorator`/`posCache`.

- [ ] **Step 4: Create `pos.go`:**

```go
package main

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdkato/prose/v2"
	"okashi/internal/textarea"
)

type posToken struct {
	text, tag  string
	start, end int // rune offsets
}

var (
	adverbStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")) // yellow
	adjStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")) // cyan
	passiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb86c")) // orange
)

var (
	posMu    sync.Mutex
	posCache = map[string][]posToken{}
)

// posTokens tags a line with prose, mapping tokens to RUNE offsets. Memoized:
// the Decorator runs per visible line each frame, but lines change only on edit.
func posTokens(line string) []posToken {
	posMu.Lock()
	if t, ok := posCache[line]; ok {
		posMu.Unlock()
		return t
	}
	posMu.Unlock()

	toks := tagLine(line)

	posMu.Lock()
	if len(posCache) > 4096 {
		posCache = map[string][]posToken{}
	}
	posCache[line] = toks
	posMu.Unlock()
	return toks
}

func tagLine(line string) []posToken {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	doc, err := prose.NewDocument(line, prose.WithExtraction(false), prose.WithSegmentation(false))
	if err != nil {
		return nil
	}
	var toks []posToken
	byteCursor := 0
	for _, tk := range doc.Tokens() {
		idx := strings.Index(line[byteCursor:], tk.Text)
		if idx < 0 {
			continue
		}
		bs := byteCursor + idx
		be := bs + len(tk.Text)
		toks = append(toks, posToken{
			text:  tk.Text,
			tag:   tk.Tag,
			start: len([]rune(line[:bs])),
			end:   len([]rune(line[:be])),
		})
		byteCursor = be
	}
	return toks
}

func isBeVerb(s string) bool {
	switch strings.ToLower(s) {
	case "am", "is", "are", "was", "were", "be", "been", "being":
		return true
	}
	return false
}

// posDecorator styles the active POS categories on one line.
func posDecorator(line string, adverb, adjective, passive bool) []textarea.Decoration {
	if !adverb && !adjective && !passive {
		return nil
	}
	toks := posTokens(line)
	var decos []textarea.Decoration
	add := func(t posToken, style lipgloss.Style) {
		decos = append(decos, textarea.Decoration{Start: t.start, End: t.end, Style: style})
	}
	for i, t := range toks {
		if adverb && strings.HasPrefix(t.tag, "RB") {
			add(t, adverbStyle)
		}
		if adjective && strings.HasPrefix(t.tag, "JJ") {
			add(t, adjStyle)
		}
		if passive && isBeVerb(t.text) {
			add(t, passiveStyle)
			for j := i + 1; j < len(toks); j++ {
				if strings.HasPrefix(toks[j].tag, "RB") {
					continue // skip an adverb between "was" and the participle
				}
				if toks[j].tag == "VBN" {
					add(toks[j], passiveStyle)
				}
				break
			}
		}
	}
	return decos
}
```

- [ ] **Step 5: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go mod tidy
/opt/homebrew/bin/go test . -run TestPos -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w pos.go pos_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add pos.go pos_test.go go.mod go.sum
git commit -m "pos: prose-backed POS engine (adverb/adjective/passive decorator, memoized)"
```

---

## Task 2: Swap Analysis UI to POS + remove markdown syntax (`inspector.go`, `main.go`) — atomic

**Files:** Modify `inspector.go`, `main.go`; delete `syntax.go`, `syntax_test.go`; Test `inspector_test.go`, `smoke_test.go`. (One atomic change: the `analysisState` struct, the UI, the wiring, and the deletion all land together so the package compiles.)

**Interfaces:**
- Consumes: `posDecorator(line, adverb, adjective, passive bool)` (Task 1), `spellDecorator`.
- Produces: `analysisState{ spell, adverb, adjective, passive bool }`; `inspectorInnerWidth() int`; `(inspectorModel).tabBar() string` (one row); compacted `inspectorTabAtX`; `analysisRowY(i int) int` + `inspectorAnalysisRowAtY(localY) (int, bool)` for rows 0=Spellcheck,1=Adverb,2=Adjective,3=Passive.

- [ ] **Step 1: Write the failing tests** — add to `inspector_test.go`:

```go
func TestAnalysisTabPOSList(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabAnalysis}
	out := in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: true})
	for _, w := range []string{"Spellcheck", "Syntax", "Adverb", "Adjective", "Passive"} {
		if !strings.Contains(out, w) {
			t.Fatalf("analysis tab missing %q:\n%s", w, out)
		}
	}
	if !strings.Contains(out, "[x] Spellcheck") || !strings.Contains(out, "[x] Adverb") {
		t.Fatalf("checked boxes wrong:\n%s", out)
	}
}

func TestTabBarFitsOneRow(t *testing.T) {
	in := inspectorModel{visible: true}
	bar := in.tabBar()
	if lipgloss.Height(bar) != 1 {
		t.Fatalf("tab bar should be one row, got %d: %q", lipgloss.Height(bar), bar)
	}
	if lipgloss.Width(bar) > inspectorInnerWidth() {
		t.Fatalf("tab bar width %d exceeds inner width %d", lipgloss.Width(bar), inspectorInnerWidth())
	}
}

func TestAnalysisRowAtY(t *testing.T) {
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(0)); !ok || r != 0 {
		t.Fatalf("Spellcheck row → %d ok=%v, want 0", r, ok)
	}
	if r, ok := inspectorAnalysisRowAtY(analysisRowY(3)); !ok || r != 3 {
		t.Fatalf("Passive row → %d ok=%v, want 3", r, ok)
	}
	if _, ok := inspectorAnalysisRowAtY(0); ok {
		t.Fatal("the tab-bar row is not a checkbox row")
	}
}
```

Add to `smoke_test.go`:

```go
func TestPOSToggleViaAnalysisClick(t *testing.T) {
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
	x := m.width - inspectorWidth + 4
	nm, _ = m.Update(tea.MouseMsg{X: x, Y: analysisRowY(1), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if !m.analysis.adverb {
		t.Fatal("clicking the Adverb row should enable it")
	}
	if m.editor.Decorator == nil {
		t.Fatal("a POS category on should set the editor Decorator")
	}
}
```

Update the existing `TestInspectorAnalysisTab`/`TestInspectorAnalysisRowAtY`/`TestSpellcheckToggleViaAnalysisClick`/`TestSyntaxToggleComposesWithSpell` to the new layout/state (the last one — about the removed markdown syntax — is DELETED; its compose intent is now covered by `TestPOSToggleViaAnalysisClick` + the spell-first order). Update existing `in.View(...)` calls if the inner-width arg changed.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: `inspector.go`** — `analysisState{ spell, adverb, adjective, passive bool }` (drop `syntax`); `inspectorInnerWidth()` returns the true inner content width (the value `main.go` passes to `View`; keep equal); `tabBar()` renders labels single-space-separated, active via `selectedStyle`, fitting one row; `inspectorTabAtX` uses the compacted chip widths; the `tabAnalysis` body = `[x/ ] Spellcheck`, blank, `Syntax` header, `[x/ ] Adverb`/`[x/ ] Adjective`/`[x/ ] Passive/weak` (labels in their category styles); `analysisRowY(i)` is the single source of each checkbox Y, with `inspectorAnalysisRowAtY` its inverse.

- [ ] **Step 4: `main.go`** — `applyDecorator()` composes (spell-first):

```go
func (m *model) applyDecorator() {
	a := m.analysis
	posOn := a.adverb || a.adjective || a.passive
	switch {
	case a.spell && posOn:
		m.editor.Decorator = func(line string) []textarea.Decoration {
			return append(spellDecorator(line), posDecorator(line, a.adverb, a.adjective, a.passive)...)
		}
	case a.spell:
		m.editor.Decorator = spellDecorator
	case posOn:
		m.editor.Decorator = func(line string) []textarea.Decoration {
			return posDecorator(line, a.adverb, a.adjective, a.passive)
		}
	default:
		m.editor.Decorator = nil
	}
}
```

And the Analysis click block → switch on `row` (0 spell, 1 adverb, 2 adjective, 3 passive) → flip + `applyDecorator()`. Remove any `m.analysis.syntax` reference.

- [ ] **Step 5: Delete the markdown decorator + verify no dangling refs**

```bash
git rm syntax.go syntax_test.go
grep -rn "syntaxDecorator\|analysis.syntax\|\.syntax\b" *.go | grep -v _test || echo "no dangling refs"
```

- [ ] **Step 6: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . 2>&1 | tail -4
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go smoke_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add inspector.go main.go inspector_test.go smoke_test.go
git commit -m "inspector: Analysis POS list (Adverb/Adjective/Passive) + one-row tab bar; wire POS toggles; drop markdown syntax"
```

---

## Self-Review

**Spec coverage:** prose dep + POS engine + memoize + multibyte + categories → Task 1; Analysis POS list + one-row tab bar + aligned hit-tests + compose + markdown-syntax removal → Task 2.

**Placeholder scan:** none — concrete code in Task 1; Task 2 is described against named single-source helpers (`inspectorInnerWidth`/`tabBar`/`analysisRowY`) the tests share. Existing-test updates are enumerated.

**Type consistency:** `posDecorator(string,bool,bool,bool)` (Task 1, NO struct dep → self-contained/compiles); `analysisState{spell,adverb,adjective,passive}` (Task 2) consumed by `applyDecorator`/clicks; `inspectorInnerWidth`/`tabBar`/`analysisRowY`/`inspectorAnalysisRowAtY` shared by inspector render, hit-test, and the smoke test.

**Compile-safety (key fix):** Task 1 is purely additive (`pos.go` only; `syntax.go` and `analysisState` untouched) → package compiles. Task 2 changes the struct + UI + wiring AND deletes `syntax.go` in ONE commit → package compiles. No intermediate broken state.

**Risk note:** byte→rune mapping (multibyte gate); prose transitive deps (gonum) → `go mod tidy` + commit `go.sum`; the one-row tab bar is the click-alignment fix (gated by `TestTabBarFitsOneRow` + `analysisRowY` assertions).
