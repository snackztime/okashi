# Personal dictionary (add-to-dictionary) — design

**Date:** 2026-06-30
**Status:** Approved
**Context:** Spellcheck flags every character/place name forever — a constant papercut for
fiction. A **global** personal dictionary (one list across all your writing) lets you teach
okashi a word once. Completes the spellcheck feature.

## Storage

- One file: **`~/.config/okashi/dictionary.txt`** (resolve via the same base dir okashi uses
  for `recentPath`/`goalsPath` — XDG `$XDG_CONFIG_HOME` else `~/.config`), one word per line,
  UTF-8. Created on first add. Comments/blank lines ignored on load.
- Loaded once at startup into a `personalWords map[string]bool` keyed by the **lowercased**
  word (case-insensitive membership, matching how writers expect "Aramil"/"aramil" to both
  pass). Written atomically (`atomicWrite`) on each add.

## Spell integration (`spell.go`)

- `spellOK(word)` returns true if `personalWords[strings.ToLower(word)]` **or**
  `speller.Spell(word)` — the personal list is checked first, so an added word is immediately
  "correct" (no underline, no suggestion).
- `addToDictionary(word)`: normalize (trim, drop surrounding punctuation), add the lowercased
  form to `personalWords`, append the original to the file atomically, clear the relevant
  `suggestCache` entry. No-op for empty/numeric/already-present words.
- Load: `loadPersonalDictionary()` called alongside `loadSpeller()` at startup.

## Trigger (`main.go`)

When the cursor is on a **misspelled** word (spellcheck on), add it via:
- **A key — `ctrl+a`** ("add word") on the writing screen: takes the word under the cursor; if
  it's misspelled, `addToDictionary` it, set status `"added 'Aramil' to dictionary"`, and
  `applyDecorator()` so its underline clears immediately. (If `ctrl+a` collides with a needed
  binding, fall back to a dedicated key — confirm during build; it is currently unbound.)
- **From the spell suggestion bar:** when `m.suggesting` for a spell word, the bar gains a
  trailing **`＋ add`** affordance after the suggestions; selecting/clicking it adds the word
  and dismisses. (`spellHintSuggestionAtX` / the suggestion menu gain this extra slot.)

Only spell (red) findings are addable — grammar/POS/Apple findings are unaffected.

## Edge cases

- Word already correct / already in the dictionary → no-op + a gentle status.
- All-caps or numeric tokens → skip (consistent with `spellDecorator` which skips them).
- The dictionary file unreadable/unwritable → degrade silently (in-memory add still works for
  the session; status notes if the write failed).
- Concurrency: single-process app; atomic write is sufficient.

## Out of scope (later)

- Per-project dictionaries (chose global; a per-project layer could be added later behind the
  same `spellOK` check without changing the trigger).
- Remove-from-dictionary UI (edit the file by hand for now).
- Sharing the personal list with the macOS app (separate concern; the file format is trivially
  portable if wanted).

## Testing

- `addToDictionary` + `spellOK`: an added word passes `spellOK` (both cases); persists to the
  file; reload picks it up; numeric/empty/all-caps skipped; punctuation stripped.
- atomic write to a temp `$XDG_CONFIG_HOME`; unreadable dir degrades without panic.
- wiring: `ctrl+a` on a misspelled word adds it + clears the underline (decorator re-applied);
  on a correctly-spelled word it's a no-op; the suggestion bar's `＋ add` slot adds + dismisses.
