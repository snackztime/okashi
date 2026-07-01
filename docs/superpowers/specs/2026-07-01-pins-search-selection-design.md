# Pinning · Multi-source search · Mouse-selection docs — design

**Date:** 2026-07-01
**Status:** Approved (direction)
**Context:** Three independent quality-of-life features batched into one cycle. No shared foundation —
each is a self-contained addition. Built in order: multi-source search → selection docs → pinning.

---

## 1. Multi-source search (a "search everywhere" key)

**Today:** `ctrl+f` search has two scopes toggled by `Tab` — `scopeProject` (0, searches
`m.files.root`) and `scopeDocument` (searches the current buffer). `searchProject(root, allowed,
query, limit)` walks one root.

**Change:** add a third scope `scopeAll` reached by a **separate key** (`ctrl+a`, not the `Tab`
cycle — Tab keeps flipping Document ↔ Project). In `scopeAll`, search every library source's root.

- `func searchAllSources(sources []source, allowed map[string]bool, query string, limit int) []searchHit`
  — iterate `sources`, skip unreachable (`!s.reachable()`), `searchProject(s.root(), …)` each with a
  shrinking remaining-limit; prefix each hit's display `name` with the source name
  (`s.Name + "/" + rel`) so results read `Wicklight/my-novel/ch3.md:12`.
- `recomputeSearch` (search.go:107) handles `scopeAll` → `searchAllSources(m.sources, m.files.allowed, q, searchLimit)`.
- The `ctrl+a` key in `updateSearch` sets `m.searchScope = scopeAll` (intercepted before the input,
  like `Tab`) and recomputes. `Tab` from `scopeAll` returns to Project (keeps the 2-way Tab cycle).
- `searchView` scope label shows `All sources`; the footer mentions `ctrl+a all sources`.
- **Tests:** `searchAllSources` finds hits across ≥2 source roots, tags each with the source name,
  skips an unreachable source, respects the limit.

## 2. Mouse text-selection — documentation only

**No code path change.** Terminal apps that enable mouse reporting (okashi does, for wheel/click)
suppress the terminal's native drag-select; the fix is the terminal's modifier bypass.

- Add one line to the F1 `helpText` (main.go:42): `⌥/⇧+drag  select text (native) · ⌘C copy`.
- (README rewrite — separate backlog item — will explain: hold **⌥ Option** on iTerm2/Ghostty/
  Terminal.app, or **Shift** on most others, then drag; `⌘C` copies.)
- **No test** (a help-string line).

## 3. Pinning (projects/folders)

A top **PINNED** strip on home for quick-jump to frequent containers. Containers only — not
`◦ Notes`, not documents (Recent already surfaces files).

**Store (`pins.go`, mirrors `recent.go`):**
- `pins.json` in `os.UserConfigDir()/okashi/pins.json` — `{"pins": [<abs container path>, …]}`.
- `func pinsPath() string`; `func loadPins(path string) []string` (missing/corrupt → nil);
  `func togglePin(path, dir string) []string` (add if absent / remove if present, atomic write,
  returns the new list); path-parameterized like `recent.go` so tests pass a temp path.

**Model + interaction:**
- `m.pinned []string` loaded at `initialModel` (`loadPins(pinsPath())`).
- Home key **`p`**: when the LIBRARY region is focused on a **project or folder** (`homeProject`/
  `homeFolder` — NOT `homeLoose`), toggle its pin (`togglePin`), refresh `m.pinned`, `rebuildHome`.
- A pinned container that no longer exists on disk is skipped (filtered at build).

**Render/nav (a new `regionPinned` strip, mirroring `regionRecent`):**
- `buildHomeItems` prepends `homePinned` items (kind) for each live pin (label `★ <base>`, path).
- `homeGroups` routes `homePinned` into a `pinned` slice; `m.pinned()` accessor.
- `homeContent` renders a full-width `PINNED` framed strip **above** the RECENT strip (reuse the
  horizontal `recentStrip` layout as `pinnedStrip`), windowed via `homeWindowOffset`; record cells.
- `visibleRegions` = `[regionPinned?, regionRecent, cols…, regionActions]` (pinned only when
  non-empty). `homeMove`: the PINNED strip is horizontal (←→ within); `↓` enters RECENT (or the
  first column / actions); it's the topmost region (`↑` does nothing). `regionPinned` is NOT a
  browse column (excluded from `visibleCols`, like `regionRecent`).
- Selecting a pinned item (`enter`/click) = selecting that container: `SetDir` into it and open its
  sidebar (same as `openHomeSelection`'s `regionLibrary`/`homeProject` behavior).
- **Tests:** `loadPins`/`togglePin` round-trip (temp path, add+remove, no-dup); `p` on a home
  project toggles the pin + it appears in `pinned()`; `p` on `◦ Notes` is a no-op; a dead pin is
  filtered from the strip; the pinned strip renders `★ name` and hit-tests (render == hit-test).

## 4. Sequencing (for the plan)

1. **Multi-source search** — `searchAllSources` + `scopeAll` + `ctrl+a` + label. (search.go/main.go)
2. **Mouse-selection help line** — one F1 `helpText` addition. (main.go)
3. **Pinning** — `pins.go` store; `m.pinned` + `p` toggle; the `PINNED` strip region (build,
   render, nav, hit-test). (pins.go/home.go/main.go)

## 5. Out of scope

- Pinning documents or `◦ Notes`; drag-to-reorder pins; cross-machine pin sync.
- A full README rewrite (separate backlog item; this only adds the F1 help line).
- Any change to okashi's mouse-capture model (selection stays terminal-native).
