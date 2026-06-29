# Editor decoration spans + spellcheck — design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)
**Context:** Cycle 3-1 of the inspector roadmap (the "Analysis" work, split: this cycle =
decoration infra + spellcheck; next = live syntax). Adds a general per-line **decoration**
hook to the vendored `internal/textarea` (generalizing the single-span dim mechanism), and the
first consumer: **spellcheck** (an embedded pure-Go wordlist that underlines misspelled words
in the editor), toggled from a new inspector **Analysis** tab.

**Why it composes with the windowed editor:** the windowed `View()` renders only the visible
source lines; the decorator hook is called inside that per-line loop, so decoration cost is
**O(visible)** — spellcheck only tokenizes on-screen lines, never the whole buffer.

## 1. Editor decoration hook (`internal/textarea`)

```go
// Decoration styles a [Start,End) rune range within a source line.
type Decoration struct {
	Start, End int
	Style      lipgloss.Style
}

// Decorator, if set, is called once per VISIBLE source line during View and
// returns decorations (rune offsets within that line). nil → no decorations
// (render is byte-identical to today). okashi:decorations
Decorator func(line string) []Decoration
```

- **Integration point:** in `View`'s per-source-line loop (the windowed render), when
  `Decorator != nil`, call `decos := m.Decorator(string(line))` **once per source line**. For
  each wrapped piece, the decorations that intersect the piece (mapped piece-relative via the
  existing `pieceStart` rune offset) are passed to `renderSeg`.
- **`renderSeg` generalization:** today it splits a segment into dim / not-dim runs against one
  `[span0,span1)`. Generalize to also split on decoration boundaries: a run covered by a
  decoration renders with the decoration's `Style` (it **wins**); runs outside decorations keep
  the current dim/normal logic. Implement by extending `splitDimRuns` (or a new
  `splitStyledRuns`) to accept the piece's decorations and emit `{text, style}` runs with
  precedence decoration > dim > normal. The cursor-segment branch is unchanged structurally —
  it just routes its two sub-segments through the same styled-run splitter.
- **Gate / safety:** `Decorator == nil` path is the exact current code (no behavior change).
  The existing `dim_test`/`editing_test`/`moveline_test`/`typewriter_test` MUST stay green; new
  tests cover decorated rendering. This is editor-core surgery — implement on the most capable
  model, byte-identity preserved for the undecorated path.

## 2. Spellcheck (`spell.go`, `assets/words.txt`)

- **Wordlist:** `//go:embed assets/words.txt` — a permissively/public-domain English wordlist
  (~100–200k words, one per line). Built once into `var spellSet map[string]struct{}` (lazy
  `sync.Once`). All pure Go; no cgo, no network.
- `wordSpans(line string) [][2]int`: find word tokens (runs of Unicode letters plus `'`), each
  returned as `[start,end)` **rune** offsets.
- `spellDecorator(line string) []textarea.Decoration`: for each word span, lowercase the word;
  **skip** if it is in `spellSet`, shorter than 3 letters, or all-caps (likely an acronym);
  otherwise emit a `Decoration{Start, End, misspellStyle}`. `misspellStyle` =
  `lipgloss.NewStyle().Foreground(red).Underline(true)` (red `#ff5555`). No suggestions in v1.
- Performance: called per visible line; a line tokenize + set lookups is microseconds.

## 3. Analysis tab + toggle (`inspector.go`, `main.go`)

- Inspector gains `tabAnalysis`; `inspectorTabLabels()` → `{"Words","Outline","Goals","Analysis"}`
  (so `ctrl+y` cycles 4 tabs; click-to-switch already works). Tab #2 → cycle key already wired.
- **Analysis body:** checkbox rows:
  ```
  ◇ ANALYSIS

  [x] Spellcheck
  [ ] Syntax        (wired next cycle — shown disabled/dim for now)
  ```
  `[x]`/`[ ]` from the model's flags. The render returns the row layout; the model holds
  `analysisSpell bool` (and `analysisSyntax bool`, inert this cycle).
- **Toggle by click:** extend the inspector mouse handling — a left-click on a checkbox **row**
  in the Analysis tab toggles the corresponding flag. Hit-test: when the Analysis tab is active
  and the click is in the inspector body at the Spellcheck row, flip `m.analysisSpell`. A small
  `inspectorAnalysisRowAtY(localY int) (int, bool)` mirrors the rendered row order (like
  `inspectorTabAtX`).
- **Wiring:** when `m.analysisSpell` is true, `m.editor.Decorator = spellDecorator`; when false,
  `m.editor.Decorator = nil`. Set on toggle and on `loadFile` (so a freshly-opened chapter
  picks up the active setting). (Syntax composes here next cycle — for now only spellcheck.)

## Testing

- **Decoration hook:** with a `Decorator` returning a span over a known word, the editor `View`
  contains that word styled (ANSI present) and the rest unstyled; `Decorator == nil` → output
  unchanged (an existing-render comparison); a decoration spanning a wrapped-line boundary
  styles the right pieces; dim + decoration compose (decoration wins in its span).
- **Spellcheck:** `spellDecorator("teh quikc brown fox")` flags "teh"/"quikc" but not "brown"/"fox"
  (assuming those are in the embedded list); words < 3 letters and all-caps skipped; a correctly
  spelled sentence yields no decorations.
- **Analysis tab/toggle:** the tab renders `[x]/[ ] Spellcheck`; a click on the Spellcheck row
  flips `m.analysisSpell` and sets/clears `editor.Decorator`; `ctrl+y` reaches Analysis as the
  4th tab.
- Existing `internal/textarea` tests stay green (undecorated path unchanged).

## Out of scope (this cycle)

- Live **syntax** highlighting (next cycle — it's another `Decorator` consumer that composes
  with spellcheck).
- Spelling **suggestions** / add-to-dictionary / per-project custom words.
- Decoration kinds beyond inline rune-range styling (no gutter marks, no multi-line spans).
