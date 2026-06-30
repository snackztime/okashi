# Grammar checking Tier 1 (heuristic, offline) — design

**Date:** 2026-06-29
**Status:** Approved
**Context:** Boost the Analysis section with grammar, beyond spelling/POS. Tier 1 = pure-Go,
offline, false-positive-safe mechanical rules, as a new "Grammar" toggle. (Tier 2 = opt-in
LanguageTool online for contextual/real-word errors — a later cycle.)

## Engine (`grammar.go`)

`grammarDecorator(line string) []textarea.Decoration` — a per-line decorator (O(visible), like
spell/POS), returning rune-range decorations styled `grammarStyle` (orange-ish underline, distinct
from spell red). Rules (each only where it can't false-positive):
- **Doubled word:** `\b(\p{L}+)\s+\1\b` (case-insensitive) → underline the second occurrence.
- **Double space** between non-space chars → underline the extra space run.
- **Space before punctuation:** a space immediately before `,.;:!?` (not `...`) → underline the space.
- **a/an:** `\ba\s+[aeiouAEIOU]` → suggest "an"; `\ban\s+[^aeiouAEIOU\s]` (a consonant, excluding
  silent-h edge — keep it simple: flag `an` + consonant) → suggest "a". Underline the article.
- **Missing terminal punctuation:** only when the WHOLE source line is a paragraph sentence (not a
  markdown heading `#…`, not a list item via `listItemRe`, not blank, and the cursor is NOT on this
  line — don't nag the line you're typing) and it doesn't end with `.!?:`-class punctuation or a
  trailing `"`/`)` after one → underline the last char. (Use prose sentence segmentation is optional;
  a per-line heuristic is enough for v1 — flag a non-heading/list, non-empty line lacking terminal
  punctuation.)

All offsets are RUNE ranges. No suggestions surfaced in v1 beyond the underline + (a/an) the
cursor-hint can show the fix later; keep v1 to highlighting.

## Analysis tab (`inspector.go`, `main.go`)

- `analysisState` gains `grammar bool`. The Analysis "Syntax" list... actually a new line under
  Spellcheck: `[ ] Grammar` (its own row), color `grammarStyle`. `inspectorAnalysisRowAtY`/
  `analysisRowY` extend to include the Grammar checkbox row; `inspectorTabAtX` unaffected.
- `applyDecorator` composes grammar with spell + POS: spell first (red underline wins), then
  grammar, then POS. Active when `analysis.grammar`.

## Don't-nag-the-current-line rule

The "missing terminal punctuation" rule must NOT flag the line the cursor is on (you're mid-sentence).
`grammarDecorator` is per-line and stateless, so pass the cursor's line in (or skip terminal-punct in
the decorator and apply it only for non-cursor lines at the call site). Simplest: `applyDecorator`'s
grammar closure captures the editor's current row and `grammarDecorator(line, isCursorLine bool)`
suppresses the terminal-punctuation rule when `isCursorLine`.

## Testing

- `grammarDecorator`: `"the the cat"` → doubled-word span on the 2nd "the"; `"a apple"` → a/an span;
  `"word ,"` → space-before-punct; `"hello  world"` → double-space; `"This is fine."` → none; a
  heading `"# Title"` / list `"- item"` → no missing-terminal-punct flag; `"This has no period"`
  (non-cursor line) → terminal-punct span, but suppressed when it's the cursor line.
- Compose: with Grammar + Spellcheck on, a misspelled doubled word shows the spell underline (spell
  wins the overlap); grammar findings render in `grammarStyle`.
- Analysis tab: `[ ] Grammar` renders; clicking it toggles + sets the decorator; `analysisRowY`
  geometry stays aligned (controller re-verifies).

## Out of scope (Tier 2 / later)

- Context confusables (to/too, its/it's), subject-verb, real-word errors → LanguageTool (Tier 2).
- Clickable grammar suggestions in the bottom bar (could reuse the spell bar later).
