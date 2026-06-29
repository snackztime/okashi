# Live syntax highlighting — design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)
**Context:** Cycle 3-2 (final inspector piece). A `syntaxDecorator` that styles markdown tokens
in the editor, composing with spellcheck via the existing `Decorator` hook (cycle 3-1). The
decoration infra is already built; this adds a second consumer + wires the Analysis-tab
**Syntax** checkbox.

## syntaxDecorator (`syntax.go`)

`syntaxDecorator(line string) []textarea.Decoration` — a per-line markdown tokenizer (O(visible),
no cross-line state). Token set + styles:

- **Heading** `^(#{1,6})(\s+)(.*)$` → the markers (`#…` + space) in `subtle`; the heading text in
  `accent`, bold.
- **Bold** `**…**` and `__…__` → bold (whole match incl. delimiters).
- **Italic** `*…*` and `_…_` → italic. **Bold is matched first**; runes already covered by a
  bold span are NOT also italicized (track occupied rune ranges so spans don't double-cover).
- **Inline code** `` `…` `` → a code color (green `#50fa7b`).
- **Link** `[text](url)` → the `text` in link-cyan (`#8be9fd`), the `(url)` in `subtle`.
- **List marker** (reuse `listItemRe` = `^(\s*)([-*+]|\d+\.)\s+`) → the marker in `subtle`.

Styles are package-level vars in `syntax.go` (`synHeadingStyle`, `synBoldStyle` =
`lipgloss.NewStyle().Bold(true)`, `synItalicStyle` = `…Italic(true)`, `synCodeStyle`,
`synLinkStyle`, `synMarkerStyle`). Offsets are **rune** ranges. Overlap rule: build spans in
priority order (heading → list marker → inline code → link → bold → italic), skipping any rune
already covered, so a given rune gets exactly one syntax style. Multi-line fenced code blocks
are out of scope (per-line only).

## Composition + toggle (`main.go`, `inspector.go`)

- `applyDecorator()` composes the active decorators (spellcheck spans FIRST so the red
  misspelling underline wins overlaps — `splitStyledRuns` picks the first covering decoration):
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
- **Syntax checkbox click:** in the Analysis-tab mouse handler, row 1 (Syntax) flips
  `m.analysis.syntax` + `applyDecorator()` (today row 1 is inert).
- **Un-dim the Syntax row:** the inspector `tabAnalysis` body renders Syntax dimmed (`subtle`);
  change it to a normal checkbox row like Spellcheck (now that it's wired).

## Testing

- `syntaxDecorator`: `"# Title"` → a heading span over "Title" (accent bold); `"**bold** plain"`
  → a bold span over `**bold**` only; `"*it*"` → italic; `"***x***"` → does not double-cover (no
  panic, deterministic); `` "`code`" `` → code span; `"[a](b)"` → link text + url spans; `"- item"`
  → marker span; plain prose → no spans.
- **Composition:** with both on, `applyDecorator` sets a decorator returning spell spans before
  syntax spans (a misspelled bold word shows the red underline, not the bold-only style, in the
  overlap — assert the spell span precedes the syntax span for the same range).
- **Toggle:** clicking the Syntax row flips `m.analysis.syntax` and sets/updates
  `editor.Decorator`; with spell already on, both compose; the inspector renders `[x] Syntax`
  normal (not dimmed).
- Existing tests stay green (spellcheck unaffected; the `Decorator==nil` editor path unchanged).

## Out of scope

- Multi-line fenced code blocks / blockquotes (per-line tokenizer has no cross-line state).
- Nested emphasis correctness beyond the bold-before-italic occupied-range rule.
- Configurable syntax colors / themes (fixed palette for v1).
