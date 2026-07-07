# Full-review roadmap — Tiers 1–3 + non-goals

> **⚠️ SUPERSEDED (2026-07-07). Historical record — do not action from this doc.**
> Nearly all of Tier 0–3 shipped: data safety, prebuilt binaries + **v0.1.0 released**, first-run
> onboarding, DOCX, undo, find & replace, resume-at-cursor, snapshots, Project Properties, readability,
> corkboard, outline mode. Homebrew is the one deliberately-deferred item (binaries-only for now — see
> `RELEASING.md`). For current state and open work, see the project-status memory and the latest review
> `docs/claude-review-decision-prompt.md`. Kept for provenance only.

**Date:** 2026-07-03
**Source:** 3-agent full product review (adoption/opus, features/sonnet, reliability/sonnet), controller-synthesized.
**Status:** Superseded (was: Roadmap). Tier-0 (data safety) was specced separately and built first
(`2026-07-03-tier0-data-safety-design.md`).

---

## Tier 1 — Adoption blockers (build after Tier-0)

### 1.1 Publish prebuilt binaries + real Homebrew tap  · effort M · impact very-high
`.github/workflows/release.yml` builds a binary but never uploads it; `Formula/okashi.rb` is a
placeholder (fake sha256, v0.1.0 never tagged). Every install compiles from source (Go 1.25).
**Approach:** add goreleaser (or a build matrix) that attaches macOS arm64/amd64 + Linux binaries to
each GitHub release; tag a real v0.x.0; fill the formula's real url+sha (or a `brew tap`). Verify the
Apple-silicon (Foundation Models) bottle gates at runtime. Removes the hardest wall to being tried.

### 1.2 First-run onboarding — seed the sample, guide the empty home  · effort S · impact high
First launch shows an empty home (`(empty)`) with no guidance; meanwhile `demo/the-lighthouse` (a real
sample manuscript) sits unused in the repo. **Approach:** on first run into an empty writing dir, copy
`demo/the-lighthouse` in (or offer "create a sample project"); add a one-line affordance on the empty
home ("press + to create a project · ctrl+n for a document · F1 for keys"). Cheapest high-impact win —
converts the people who actually launch it and teaches the manifest model by example.

### 1.3 DOCX export  · effort M · impact high
Agents/editors want `.docx`; okashi exports only RTF+PDF, and the README overclaims RTF as "the
standard submission format." **Approach:** add a DOCX writer — OOXML is zipped XML, doable in **pure
Go via `archive/zip`** (no cgo), reusing the existing `ManuscriptDoc` AST (`export_ast.go`). Wire into
the `ctrl+e` prompt as a third target (or emit alongside RTF+PDF). Soften the README claim.

---

## Tier 2 — Real feature gaps for serious writers

### 2.1 Undo / redo  · effort M · value high  (also a data-safety item)
The vendored `internal/textarea` has **no undo stack** at all. **Approach (lean):** a coarse
checkpoint ring in the app — push the buffer to a bounded ring (~10) on each autosave tick / before
each destructive apply (spell/grammar replace, load); `ctrl+z` restores the previous checkpoint,
`ctrl+y`/`ctrl+shift+z` re-applies. Not a character-level undo; matches the autosave granularity the
writer already trusts. `ctrl+z` is currently unbound. (A true per-edit undo ring in the textarea is
the larger alternative.)

### 2.2 Find & replace (in-document)  · effort S · value high
`ctrl+f` search is find-only. **Approach:** extend the search screen with a replace input (e.g. a
second line, opener like a key within search); reuse `editor.ReplaceRange` (already used by
spell/grammar apply) to replace current / all in the active chapter. Manuscript-wide replace is a
stretch goal; document-scope is the lean, table-stakes cut.

### 2.3 Resume at last cursor position  · effort S · value high
`loadFile` always opens at line 0. **Approach:** extend `recent.json` entries from `string` to
`{path, line}`; store the current line on save/switch; `editor.MoveToLine(line)` after `SetValue`.
~20 lines, removes daily friction.

### 2.4 Manuscript-format title page in RTF/PDF export  · effort S · value M
The Manuscript export produces a running header but no standard agent title page. **Approach:** in
`writeRTF`/`writePDF` for `StyleManuscript`, prepend a title page: author + contact block, approx word
count (round to nearest ~250, summed from `ManuscriptDoc`), centered title/byline. `meta.Author` is
already available (`OKASHI_AUTHOR`). The single most visible gap vs the real submission standard.

### 2.5 Readability + reading time + word-frequency in the Words tab  · effort S · value M
No readability stats today. **Approach:** in the Words inspector tab (`inspector.go`, already computes
words/chars/paragraphs), add reading time (`words/238`), sentence-length mean±stddev (split on
`.!?`), and an overused-word list. Cheap pure-Go, high craft signal.

### 2.6 Timestamped snapshot history + manual `b` backup key  · effort M · value M
Follow-up to Tier-0's single-session backup: keep a capped, timestamped ring under `.okashi-bak/`
(e.g. last 10 per file) + a `b` key for an explicit "snapshot before a big cut," and a restore UI.

---

## Tier 3 — QoL polish

- **Discoverable selection mode**  · effort S — a `-- SELECT --` toggle that flips `tea.DisableMouse`
  so native drag-select works (fixes the "selection feels broken" first impression; ~30 lines, noted
  in the old roadmap). Today it's modifier-bypass (⌥/⇧-drag), documented but not discoverable.
- **Per-project settings**  · effort S — width/smartquotes per project (a `manifest.json` `settings`
  stanza or a per-dir config) instead of only global `OKASHI_*` env vars.
- **Non-UTF-8 guard in loadFile**  · effort S — today an invalid-UTF-8 (e.g. Latin-1) file is silently
  mutated on first save; add a `utf8.Valid` check + warn, don't mark dirty until an intentional edit.
- **Lock-in messaging** · effort trivial — the README should say loudly that work is plain `.md` +
  readable `manifest.json` (grep/git-friendly, zero lock-in) — a real trust advantage vs Scrivener.
- **commitStructure / moveDocument ordering** · effort S — reorder so a partial failure leaves the
  manifest and files consistent (review risks #5/#6).

---

## Deliberately NOT building (holds the lean / anti-PKM ethos)
- **EPUB export** — production, not composition; export DOCX → Calibre/Pandoc. Disproportionate to "lean."
- **Character / place / timeline metadata** — anti-PKM by design (CLAUDE.md). The okashi answer is a
  category folder of plain `.md`.
- **Multiple cursors / split view** — don't fit prose+TUI; expand surface that propagates to the
  companion app. The pager already covers "read one while writing another."

## Adoption verdict (context for prioritization)
The terminal is a hard ceiling on market SIZE — it excludes the non-technical novelist bulk that
okashi's *features* target. Accept **niche-but-loved** (developer-writers who live in the terminal);
don't chase mass-market Scrivener. The fixable part is that a blank first-run and compile-from-source
install cap okashi *below* even that ceiling — hence Tier-1's binaries + onboarding.
