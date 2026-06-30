# Grammar Tier 2 — optional on-device Apple backend — design

**Date:** 2026-06-29
**Status:** Approved (design only — build deferred; Go heuristics remain the primary path)
**Context:** Tier 1 (pure-Go heuristic grammar) is shipped and always-on. This designs an
**optional, opt-in, macOS-only, on-device** "deep grammar" backend that augments the
heuristics with contextual checking (e.g. real-word errors like "My to hurts") — using Apple
Intelligence (Foundation Models) when available, NSSpellChecker as a fallback. Everything
here is **gated**: the default `go build` stays pure-Go and cross-platform.

## Principles (non-negotiable)

- **Pure-Go default preserved.** The Apple backend lives behind a build tag
  (`//go:build darwin && cgo && applegrammar`). A no-tag build compiles a stub that reports
  "unavailable". `go build` with no tags → heuristics only, cross-platform, no cgo.
- **On-device & private.** Both backends run locally — no network (the privacy win over the
  LanguageTool option, which we are NOT pursuing). Surface "on-device" in the UI.
- **Heuristics stay primary.** Tier 1 runs inline, instant, every keystroke. The Apple pass
  is an *augmentation* — a whole-chapter async check — never a replacement.
- **The Grammar toggle gates everything.** No Apple call happens unless Analysis ▸ Grammar is on.

## User-facing behavior

- **Analysis tab, when Grammar is ON and an Apple backend is available:**
  - a **`Check grammar`** action row → runs the Apple pass over the current chapter (on-demand).
    Shows `checking grammar…` while running; on completion, findings render as decorations and
    the bottom bar lets you step/jump and click-to-fix (reuse the spell click-to-suggest bar).
  - an **`Auto-recheck`** sub-toggle (default **off**): when on, a debounced (~1.5 s idle) Apple
    pass runs after edits — but only while Grammar is active. Off by default so nothing fires
    unprompted.
  - a small backend label, e.g. `(Apple Intelligence)` or `(system checker)` / `(on-device)`.
- **When no Apple backend** (non-macOS, no-tag build, or AI unavailable): the rows are hidden;
  only the heuristics run. No behavior change from Tier 1.

## Architecture

### Backend interface (pure-Go, always compiled)
```go
type grammarFinding struct {
	Line, Start, End int    // location: source line index + rune range within it
	Message          string // human description ("Did you mean 'two'?")
	Replacements     []string
}

type grammarChecker interface {
	Name() string                                  // "Apple Intelligence" | "system checker"
	Available() bool                               // runtime availability
	Check(ctx context.Context, text string) ([]grammarFinding, error)
}

// selected at startup; nil when nothing is available (heuristics-only)
func appleGrammarChecker() grammarChecker
```
- `grammar_apple_stub.go` (`//go:build !(darwin && cgo && applegrammar)`): `appleGrammarChecker`
  returns nil. Keeps the default build pure-Go.
- `grammar_apple_darwin.go` (`//go:build darwin && cgo && applegrammar`): the cgo backends +
  selection (prefer Foundation Models, then NSSpellChecker).

### Backend selection (runtime, macOS build)
1. **Foundation Models** if `SystemLanguageModel.default.availability == .available`
   (macOS 26 + Apple silicon + Apple Intelligence enabled).
2. else **NSSpellChecker** (present on any macOS).
3. else nil → heuristics only.

### Async flow (Bubble Tea)
- `Check grammar` click (or the debounced auto timer) → a `tea.Cmd` that calls
  `checker.Check(ctx, chapterText)` off the main loop → returns `grammarResultMsg{findings, err}`.
- `Update` stores `m.appleFindings` (keyed by the current file); `applyDecorator` merges them
  with the heuristic + spell + POS decorations (precedence: spell > heuristic-grammar >
  apple-grammar > POS, or fold apple into the grammar layer — TBD at build time).
- A `m.checkingGrammar` flag drives the `checking grammar…` status + a spinner.
- Findings are **rune-range, per source line** so they slot straight into the existing
  per-line `Decorator` + click-to-fix machinery. The backend returns absolute char offsets;
  the bridge maps them to (line, runeStart, runeEnd).

### cgo bridges (macOS only)
- **NSSpellChecker (Objective-C — the easy one):**
  `#cgo LDFLAGS: -framework AppKit -framework Foundation`; call
  `[[NSSpellChecker sharedSpellChecker] checkGrammarOfString:… details:&details]` in a loop over
  the text; each detail yields a range + description → `grammarFinding`. No suggestions API for
  grammar (spelling has `guessesForWordRange:`), so `Replacements` may be empty for grammar hits.
- **Foundation Models (Swift — the harder one):** Swift isn't directly cgo-callable. Add a tiny
  Swift shim exposing `@_cdecl("okashi_fm_check")` C-ABI functions (input text → JSON findings),
  compiled to a static lib and linked via cgo (`#cgo LDFLAGS: -L… -lokashifm -framework
  FoundationModels`). The Swift side uses `LanguageModelSession` with a `@Generable` result type
  (or asks for JSON) prompting "Proofread; list grammar/spelling issues with offsets and
  suggestions." Map offsets → line/rune ranges. Guard behind the availability check.

## Build & distribution

- **Default:** `go build` (no tags) → pure-Go, cross-platform, heuristics only. Unchanged.
- **macOS release:** `go build -tags applegrammar` (needs Xcode/CLT + the Swift shim prebuilt).
  CGO_ENABLED=1. Produces the binary with the Apple backend.
- Document both in CLAUDE.md's build section when built.

## Phasing (build later, in this order)

- **Phase A — plumbing + NSSpellChecker:** the interface, the stub, the ObjC NSSpellChecker
  backend, the `Check grammar` action, the async `tea.Cmd`/`grammarResultMsg` flow, finding→
  decoration mapping, and click-to-fix reuse. Ships on-device grammar on any macOS.
- **Phase B — Foundation Models:** the Swift shim + `@_cdecl` bridge + availability detection +
  the prefer-FM-fallback selection. Ships the Apple Intelligence path.
- **Phase C — Auto-recheck:** the debounced sub-toggle (only while Grammar active).

## Out of scope

- LanguageTool / any network backend (explicitly not pursued — on-device only).
- Windows/Linux equivalents (no comparable on-device API; heuristics serve there).
- Rewrite/▸"improve this sentence" generative features — this is grammar *checking* only.

## Shared-contract note

This adds NO on-disk format and touches no manifest/markdown-flavor contract — it's a local,
read-only analysis backend. No `../inkmere` mirroring required. (If the macOS app later wants
the same backend, the Swift/ObjC bridges are reusable, but that's the app's concern.)
