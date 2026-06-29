# Parts-of-speech highlighting (replaces markdown syntax) — design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)
**Context:** The previous "Syntax" feature highlighted *markdown* tokens; the user wants **parts
of speech** — a writing-craft highlighter (adverbs/adjectives/passive). This replaces the
markdown `syntaxDecorator` with a POS decorator backed by a real tagger, and reworks the
Analysis tab into Spellcheck + a "Syntax" list of POS toggles — which also fixes the tab-bar
wrap + checkbox click-misalignment.

## Dependency

- Add `github.com/jdkato/prose/v2` (pure-Go averaged-perceptron POS tagger, Penn-Treebank tags,
  model embedded). Pin it; run `go mod tidy`. (CLAUDE.md allows pinned pure-Go deps.)

## POS engine (`pos.go`, replaces `syntax.go`)

- `posTokens(line string) []posToken` where `posToken{ text, tag string; start, end int }` —
  tag the line with prose (`prose.NewDocument(line, prose.WithExtraction(false),
  prose.WithSegmentation(false))` → `doc.Tokens()`), and map each token back to its **rune**
  offsets in the line (walk a rune cursor, find each token's text from the cursor; advance).
- **Memoize** line→tokens (a mutex-guarded `map[string][]posToken`, soft-capped, or `sync.Map`):
  the `Decorator` is called per visible line each frame, but lines change only on edit, so this
  is a cache hit per frame. Cache miss only runs prose.
- `posDecorator(line string, a analysisState) []textarea.Decoration` — for each ACTIVE category,
  emit decorations:
  - **Adverb** (`a.adverb`): tokens whose tag starts `RB` → `adverbStyle` (yellow `#f1fa8c`).
  - **Adjective** (`a.adjective`): tag starts `JJ` → `adjStyle` (cyan `#8be9fd`).
  - **Passive/weak** (`a.passive`): a be-verb (lowercased text in `{am,is,are,was,were,be,been,
    being}`) → `passiveStyle` (orange `#ffb86c`); if a following token (skipping adverbs) is a
    past participle (`VBN`), decorate THAT token too (marks the passive construction). Each
    matched token is its own rune-range decoration.

## Analysis tab redesign (`inspector.go`)

- **Tab bar fits one row.** The 4-tab bar currently overflows the inspector (33 cols vs ~28
  inner) and wraps, shifting the body — the cause of the click misalignment. Compact the chips
  (single-space separators, not `" label "`) so the bar fits the inspector's true inner width
  on ONE row; pass that true inner width to `View`. A render test asserts the tab bar's height
  is 1 (`lipgloss.Height` of the bar ≤ inner width == 1).
- **Body:** `[ ] Spellcheck`, blank, `Syntax` header, then `[ ] Adverb` / `[ ] Adjective` /
  `[ ] Passive/weak`, each with its category color on the label.
- **Hit-tests recomputed:** `inspectorTabAtX` updated to the compacted chip widths;
  `inspectorAnalysisRowAtY(localY)` returns an index for each checkbox row (0=Spellcheck,
  1=Adverb, 2=Adjective, 3=Passive) at the rows they actually render (derive from the layout;
  a render-based test asserts each clicked Y maps to the right checkbox).
- `analysisState` becomes `{ spell, adverb, adjective, passive bool }` (the old `syntax` bool is
  removed). `View` keeps its 6-arg shape (`analysis analysisState`).

## Wiring (`main.go`)

- `applyDecorator()` composes: spellcheck and POS. If any of `adverb/adjective/passive` is on,
  a POS decorator is active; combine with spellcheck **spell-first** (spell underline wins):
  ```go
  posOn := m.analysis.adverb || m.analysis.adjective || m.analysis.passive
  switch {
  case m.analysis.spell && posOn:
      a := m.analysis
      m.editor.Decorator = func(line string) []textarea.Decoration {
          return append(spellDecorator(line), posDecorator(line, a)...)
      }
  case m.analysis.spell:
      m.editor.Decorator = spellDecorator
  case posOn:
      a := m.analysis
      m.editor.Decorator = func(line string) []textarea.Decoration { return posDecorator(line, a) }
  default:
      m.editor.Decorator = nil
  }
  ```
- The Analysis-tab click handler maps each checkbox row to its flag (Spellcheck / Adverb /
  Adjective / Passive) → flip + `applyDecorator()`.
- **Remove** `syntax.go` + `syntax_test.go` (the markdown decorator) and any `m.analysis.syntax`
  references.

## Testing

- `posTokens`: rune offsets correct (incl. a multibyte line); tokens map back to the line.
- `posDecorator`: "She quickly ran" with adverb on → "quickly" decorated; "the red car" with
  adjective on → "red"; "it was written" with passive on → "was" + "written" decorated; nothing
  when the category is off; categories compose (multiple on → each word its color).
- Memoization: a second call with the same line doesn't re-tag (e.g. a call counter, or just
  assert identical result fast).
- Layout: tab bar renders on ONE row at the inspector inner width; `inspectorAnalysisRowAtY`
  maps each rendered checkbox row to the right index (render-based assertion).
- Wiring: clicking each checkbox toggles its flag + sets the Decorator; compose with spellcheck
  spell-first; existing spellcheck tests green.

## Out of scope

- Persisting the POS toggles (session-only for v1, like the current spell toggle).
- Sentence-level analysis (passive across line breaks; readability/complex-sentence flags).
- Per-category color customization.
