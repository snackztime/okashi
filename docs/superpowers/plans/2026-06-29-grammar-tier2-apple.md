# Grammar Tier 2 — Apple on-device backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, FIRST element of the reply.

**Goal:** An opt-in, on-device "deep grammar" backend that augments the Go heuristics — NSSpellChecker (agreement/fragments, any macOS) and Foundation Models (contextual/real-word errors, macOS 26 + Apple Intelligence) — triggered by a `Check grammar` action in the Analysis tab. Default `go build` stays pure-Go.

**Architecture:** A pure-Go `grammarChecker` interface; a build-tagged (`darwin && cgo && applegrammar`) cgo backend bridging Objective-C (NSSpellChecker) and a Swift `@_cdecl` static lib (Foundation Models); a stub for all other builds. Findings flow through an async `tea.Cmd` → decorations (green) + click-to-fix.

**Tech Stack:** Go + cgo, Objective-C (AppKit), Swift (FoundationModels), Bubble Tea.

**Design spec:** `docs/superpowers/specs/2026-06-29-grammar-tier2-apple-design.md`
**Validated build recipe (spikes, all confirmed working on macOS 26.5 / arm64):**
- NSSpellChecker: a `.m` file compiled by cgo; `runtime.LockOSThread()`; `checkGrammarOfString:…details:` returns ranges + `NSGrammarUserDescription`.
- Foundation Models: `swiftc -emit-library -static -o libokashifm.a bridge.swift -framework FoundationModels` (dynamic stdlib — `-static-stdlib` is unsupported on Apple), then cgo `LDFLAGS: -L${SRCDIR} -lokashifm -framework FoundationModels -L$(xcrun --show-sdk-path)/usr/lib/swift`. `okashi_fm_available()` → 1, `okashi_fm_proofread()` runs on-device in ~1.5s.

## Global Constraints

- Module `okashi`; Go 1.25; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- **Default build stays pure-Go.** All cgo lives behind `//go:build darwin && cgo && applegrammar`. `go build` (no tags) must compile, be cgo-free, and behave exactly as today (heuristics only).
- **On-device only** — no network. The Grammar toggle gates everything; no Apple call unless `m.analysis.grammar` is on.
- Heuristics (Tier 1) remain primary/inline. The Apple pass is an on-demand augmentation.
- `grammarFinding` offsets are **rune ranges within a source line** (so they slot into the existing per-line `Decorator` + click-to-fix). The bridges return absolute UTF-16/byte offsets → convert.
- Apple findings render in the existing green `grammarStyle`.
- `gofmt`, `go vet ./...`, `go test ./...`, `go build ./...` (no tags) clean before each commit. The tagged build is verified separately: `CGO_ENABLED=1 go build -tags applegrammar` after the swift lib is built.

---

## Task 1: Backend interface + stub + build scaffolding (controller-led — cgo/Swift)

**Files:** Create `grammar_backend.go`, `grammar_apple_stub.go`, `grammar_apple_darwin.go`, `grammar_apple.h`, `grammar_apple.m`, `grammar_apple_fm.swift`, `Makefile` (or `build-apple.sh`); Test `grammar_backend_test.go`

**Interfaces (Produces):**
```go
type grammarFinding struct {
	Line, Start, End int      // source line index + rune range within that line
	Message          string   // human description
	Replacements     []string // suggested fixes (may be empty)
}
type grammarChecker interface {
	Name() string                                       // "Apple Intelligence" | "system checker"
	Available() bool
	Check(text string) ([]grammarFinding, error)        // whole-document; maps to per-line findings
}
func appleGrammarChecker() grammarChecker               // nil when unavailable
```

- [ ] **Step 1:** `grammar_backend.go` — the struct + interface above (pure Go, always compiled). No build tag.
- [ ] **Step 2:** `grammar_apple_stub.go` (`//go:build !(darwin && cgo && applegrammar)`): `func appleGrammarChecker() grammarChecker { return nil }`.
- [ ] **Step 3:** `grammar_backend_test.go` — assert `appleGrammarChecker()` is callable and (in the default build) returns nil. `go test ./...` green, cgo-free.
- [ ] **Step 4:** Add the **Objective-C NSSpellChecker bridge** `grammar_apple.h` + `grammar_apple.m` (from the validated spike: loop `checkGrammarOfString:…details:`, emit `{absLocation, length, description}` rows into a C array the Go side reads). Provide `okashi_grammar_check(const char* text, ...)` returning issue ranges (UTF-16 offset + length) + descriptions.
- [ ] **Step 5:** Add the **Swift Foundation Models bridge** `grammar_apple_fm.swift` with `@_cdecl("okashi_fm_available")` and `@_cdecl("okashi_fm_proofread")`. Use a `@Generable` result type for clean structured output (NOT free text — the free-text spike echoed unchanged words):
  ```swift
  @Generable struct FMIssue { @Guide(description: "the exact wrong substring") let wrong: String
                              @Guide(description: "the corrected text") let fix: String
                              let reason: String }
  @Generable struct FMResult { let issues: [FMIssue] }
  // session.respond(to: text, generating: FMResult.self) → encode issues as JSON → strdup
  ```
  The Go side parses the JSON and **locates each `wrong` substring in the document** to derive offsets (FM doesn't reliably emit character offsets).
- [ ] **Step 6:** `Makefile` target `apple`: `swiftc -emit-library -static -o libokashifm.a grammar_apple_fm.swift -framework FoundationModels` then `CGO_ENABLED=1 go build -tags applegrammar -o okashi-apple .`. Document the recipe.
- [ ] **Step 7:** `grammar_apple_darwin.go` (`//go:build darwin && cgo && applegrammar`): the cgo preamble (`#include "grammar_apple.h"`, `#cgo LDFLAGS: -framework AppKit -framework Foundation -L${SRCDIR} -lokashifm -framework FoundationModels -L<swiftlib>`), `runtime.LockOSThread` on use, two concrete checkers (`fmChecker`, `spellChecker`) implementing `grammarChecker`, and `appleGrammarChecker()` returning fm if `okashi_fm_available()`, else spell, else nil. (Offset mapping lives in Task 2; here just return raw issues.)
- [ ] **Step 8:** Verify BOTH builds: `go build ./...` (no tags, pure Go) AND `make apple` (tagged) compile; the tagged binary runs both backends on a smoke sentence. Commit.

---

## Task 2: Findings → per-line rune ranges (`grammar_apple_darwin.go`)

**Files:** Modify `grammar_apple_darwin.go`; Test `grammar_apple_darwin_test.go` (tagged)

**Interfaces (Consumes):** the raw bridge output (Task 1). **Produces:** `Check()` returns `[]grammarFinding` with correct per-line rune ranges.

- [ ] **Step 1 (test, tagged `//go:build … applegrammar`):** feed a 2-line text with a known grammar error on line 1 → assert a finding with the right `Line` and a rune range covering the offending word; assert a clean text → no findings.
- [ ] **Step 2:** Implement offset mapping: NSSpellChecker returns UTF-16 offsets into the whole text → convert to (line index, rune start, rune end) by walking the document's lines and UTF-16→rune columns. For FM, locate each `wrong` substring (first match at/after a cursor, or all matches) → (line, rune range). Dedup overlapping NSSpell+FM hits (prefer FM's correction when both cover the same span).
- [ ] **Step 3:** Run the tagged test; `make apple`; commit. (No effect on the default build.)

---

## Task 3: Async flow + Analysis "Check grammar" action (`main.go`, `inspector.go`)

**Files:** Modify `main.go`, `inspector.go`; Test `smoke_test.go`, `inspector_test.go`

**Interfaces (Consumes):** `appleGrammarChecker()` (Task 1), `grammarFinding` (Task 1). **Produces:** `m.grammarChecker`, `m.appleFindings map[string][]grammarFinding`, `m.checkingGrammar bool`, a `grammarResultMsg`.

- [ ] **Step 1 (test):** with a stub checker injected (so the default build can test the flow without cgo — `appleGrammarChecker` is overridable via a package var `newGrammarChecker = appleGrammarChecker`), clicking the `Check grammar` row dispatches a command and a `grammarResultMsg{findings}` stores them in `m.appleFindings[currentFile]`; the Analysis row only shows when grammar is on AND `m.grammarChecker != nil`.
- [ ] **Step 2:** `main.go`: at startup set `m.grammarChecker = newGrammarChecker()`. Add `m.appleFindings`, `m.checkingGrammar`. A `checkGrammarCmd` `tea.Cmd` calls `m.grammarChecker.Check(chapterText)` and returns `grammarResultMsg`. `Update` handles the msg (store findings, clear `checkingGrammar`, `applyDecorator`). Status shows `checking grammar…` while pending.
- [ ] **Step 3:** `inspector.go`: under the Grammar checkbox, render a `Check grammar  (<backend name>)` action row when `grammar && checker != nil`; extend `analysisRowY`/`inspectorAnalysisRowAtY`/the click switch to include the action (it triggers the command, not a toggle). Keep the 5 checkbox rows aligned; controller re-verifies click geometry.
- [ ] **Step 4:** Run tests; `go build ./...` (no tags) green; commit.

---

## Task 4: Merge Apple findings into decorations + click-to-fix (`main.go`)

**Files:** Modify `main.go`; Test `smoke_test.go`

**Interfaces (Consumes):** `m.appleFindings` (Task 3), the existing `applyDecorator` + click-to-suggest bar.

- [ ] **Step 1 (test):** with `m.appleFindings` populated for the current file, the editor `Decorator` returns green decorations at those ranges; clicking a flagged span opens the suggestion bar showing the finding's `Replacements`, and selecting one applies it via `ReplaceRange`.
- [ ] **Step 2:** `applyDecorator`: fold `m.appleFindings[m.currentFile]` for the visible lines into the per-line decorations (green `grammarStyle`), composed after heuristics (spell > heuristic-grammar > apple-grammar > POS). The click-to-fix path (already built for spelling) gains a case for Apple findings → show `Message`/`Replacements`.
- [ ] **Step 3:** Edits invalidate stale offsets: clear `m.appleFindings[file]` on edit to that file (findings reflect the text at check time). Re-check via the action.
- [ ] **Step 4:** Run tests; `go build ./...` green; commit.

---

## Out of scope (follow-ups)

- **Phase C — Auto-recheck** (debounced while Grammar active): a later cycle.
- Homebrew tap / CI bottling (default pure-Go + Apple-silicon bottle): packaging, separate from the code.
- The macOS app reusing the bridges.

## Self-Review

**Spec coverage:** backend interface + gating (Task 1), both bridges + build recipe (Task 1), offset mapping (Task 2), the on-demand `Check grammar` action + async flow + runtime gating (Task 3), decoration merge + click-to-fix + edit-invalidation (Task 4). Auto-recheck explicitly deferred.

**Placeholder scan:** the bridge code references the validated spikes; Step 5's `@Generable` replaces the garbled free-text output (a real fix, not a placeholder).

**Type consistency:** `grammarFinding{Line,Start,End,Message,Replacements}` and `grammarChecker` defined in Task 1, consumed unchanged in 2-4; `newGrammarChecker` package var enables testing the flow in the pure-Go build.

**Risk:** the cgo/Swift build (Task 1) is the hard part but is fully spiked. The Analysis row geometry changes again (an action row) — controller re-verifies clicks empirically. The default build must stay pure-Go — verified by `go build ./...` with no tags + the cgo-free stub.
