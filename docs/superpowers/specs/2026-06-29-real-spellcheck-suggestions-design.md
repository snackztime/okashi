# Real spellcheck + spelling suggestions — design

**Date:** 2026-06-29
**Status:** Approved
**Context:** The current spellcheck (raw `/usr/share/dict/words`, set-membership lookup) produces
heavy false positives — inflections (`jumps`, `emailed`), contractions (`don't`, `it's`),
possessives (`Sarah's`) — so dense underline runs *look like phrases*. This replaces the engine
with a real hunspell checker (`gospell`) and adds key-triggered spelling suggestions.

**Verified (probes):** with the `en` hunspell dict, gospell accepts `jumps`/`emailed`/
`reconnected`/`don't`/`it's`/`Sarah's`/`cafe's` (correct) and still flags `teh`/`quikc`/`brilig`;
`gs.Suggest("teh",5)`→`[the ten eh tech tea]`, `quikc`→`[quick]`, `recieve`→`[receive relieve]`,
`seperate`→`[separate]`, `langauge`→`[language]`. Quality is good; the correct word ranks first.

## 1. Engine swap (`spell.go`)

- Add `github.com/client9/gospell` (pure-Go hunspell; pin `v0.9.2`).
- Embed `assets/en.dic` (~552 KB) + `assets/en.aff` (~3 KB) from
  [wooorm/dictionaries `en`](https://github.com/wooorm/dictionaries/tree/main/dictionaries/en).
  Build `var speller *gospell.GoSpell` lazily (`sync.Once`) via `gospell.NewGoSpellReader`.
  Ship the dictionary's `LICENSE` as `assets/en.LICENSE` (the dict is tri-licensed
  GPL-2.0/LGPL-2.1/MPL-1.1; the MPL option permits redistribution with the notice) and a one-line
  attribution in `assets/en.README`.
- **Remove** `assets/words.txt` (2.4 MB), `wordsFile`, `spellSet`, `loadSpellSet`.
- Helpers:
  - `spellOK(word string) bool` — `loadSpeller(); return speller.Spell(word)`.
  - `spellSuggest(word string, limit int) []string` — `speller.Suggest(word, limit)` → the
    `.Word` of each `gospell.Suggestion`, in order; `nil` on error.
- `spellDecorator(line string)` keeps `wordSpans` (letters + `'` tokens, rune offsets) but the
  per-token test becomes: skip a token that is **all-caps** (acronym) or contains a digit;
  otherwise flag it when `!spellOK(word)`. (Drop the old `<3 runes` skip — gospell knows short
  words; keep `misspellStyle` = red underline.) gospell handles case/possessive/contraction, so
  pass the raw token (e.g. `don't`, `Sarah's`) straight to `spellOK`.

## 2. Editor primitives (`internal/textarea`)

The cursor word must be located and replaced precisely. Add to the vendored `Model`:
- `CursorColumn() int` — the cursor's **rune** column on the current logical line (`m.col`).
- `ReplaceRange(start, end int, s string)` — on the current row, replace runes `[start,end)` with
  `s`, set the cursor column to `start + len([]rune(s))`, and invalidate the wrap cache for that
  line (same path `InsertString`/delete use). Guard `0 ≤ start ≤ end ≤ len(line)`.

Both are small, pure additions with unit tests (insert/replace at line start, middle, end; a
multibyte line). They do not touch the windowed `View` or decoration paths.

## 3. Suggestions UI (`main.go`)

- **Word under cursor:** `wordUnderCursor() (word string, start, end int, ok bool)` — take the
  current logical line (`editor.Value()` split on `\n`, index `editor.Line()`), `col :=
  editor.CursorColumn()`, and find the `wordSpans` span with `start ≤ col ≤ end` (so a
  just-finished word counts). Returns the word and its rune range on that line.
- **Model state:** `suggesting bool`, `suggestions []string`, `suggestIndex int`,
  `suggestStart, suggestEnd int`, `suggestWord string`.
- **Trigger (`ctrl+r`)**, on the writing screen when no other prompt is active:
  - `w, s, e, ok := wordUnderCursor()`; if `!ok` → status `"no word under cursor"`.
  - else if `spellOK(w)` → status `"‘w’ looks correct"`.
  - else `sugg := spellSuggest(w, 7)`; if empty → status `"no suggestions for ‘w’"`.
  - else set `suggesting=true`, `suggestions=sugg`, `suggestIndex=0`, `suggestWord=w`,
    `suggestStart=s`, `suggestEnd=e`.
- **Menu keys (when `suggesting`):** `left`/`right` move `suggestIndex` (clamped); `1`–`9` pick
  that index directly and apply; `enter` applies the selected; `esc` cancels (status
  `"suggestion cancelled"`); `ctrl+c` quits. All other keys are swallowed (menu is modal, like
  `m.renaming`).
- **Apply:** `chosen := matchCase(suggestWord, suggestions[i])`;
  `editor.ReplaceRange(suggestStart, suggestEnd, chosen)`; `suggesting=false`; status
  `"‘word’ → ‘chosen’"`; `m.applyDecorator()` stays in effect.
  - `matchCase(orig, sugg)`: if `orig` is all-caps → upper `sugg`; if `orig` is title-case
    (first rune upper, rest not all-upper) → upper-first `sugg`; else `sugg` as-is. (gospell
    suggestions come lowercased.)
- **Render (status line):** before the `m.renaming` branch, `if m.suggesting` →
  `"suggest ▸ "` + the suggestions joined by `" · "`, the selected one wrapped in
  `selectedStyle`. (Truncate to width if needed.)

## Testing

- **Engine:** `spellOK` true for `jumps`/`don't`/`Sarah's`/`emailed`, false for `teh`/`quikc`;
  `spellSuggest("teh",5)` contains `"the"`; `spellDecorator` flags `teh` not `jumps`/`don't` in a
  sentence; all-caps/digit tokens skipped.
- **Editor:** `ReplaceRange` on `"the cat"` replacing `[4,7)` with `"dog"` → `"the dog"`, cursor at
  col 7; multibyte line offsets correct; `CursorColumn` reflects cursor moves.
- **UI:** `wordUnderCursor` returns the span at the cursor; `ctrl+r` on a misspelled word opens the
  menu with suggestions; `enter` replaces the word (case-preserved: `Teh→The`); `esc` cancels
  leaving the buffer unchanged; `ctrl+r` on a correct word shows the status note, no menu;
  number-key selection applies the right item.

## Out of scope

- Add-to-dictionary / personal word list (a natural next cycle — `wifi`/`Naptime`-type words stay
  flagged for now).
- Auto-correct / inline replacement without the menu; multi-word/grammar suggestions.
- Suggestions for words spanning a soft-wrap boundary are computed on the logical line (fine —
  `wordSpans` works on the unwrapped line).
