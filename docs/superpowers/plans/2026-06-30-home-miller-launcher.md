# Home-screen Miller launcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development.
> Steps use checkbox (`- [ ]`) syntax. ONE tool call per message, FIRST in the reply.

**Goal:** Replace the home screen with a centered logo + three framed columns
(**RECENT · LIBRARY · FILES**) + centered actions: Recent is an always-visible quick-launch;
selecting a project/folder in LIBRARY live-populates FILES (name + word count + a dim 1-line
snippet); Enter opens a file directly or opens a project's sidebar.

**Architecture:** All in `home.go` + a new `snippet.go`; no writing-view / `sidebarWidth`
changes. Reuses `framedPanel`, `resolveManuscript`, `wordCountCache`. Selection is
`(region, index)` over four regions (recent/library/files/actions); the LIBRARY selection is
held separately and drives FILES.

**Tech Stack:** Go, Bubble Tea, lipgloss, the existing `framedPanel`/`homeContent`/`homeItemAt`.

**Design spec:** `docs/superpowers/specs/2026-06-30-home-miller-drilldown-design.md`

## Global Constraints

- Module `okashi`; Go 1.25; `/opt/homebrew/bin/go`, `/opt/homebrew/bin/gofmt`.
- Snippet reads the **first ~400 bytes only**; mtime-cached; lazy (visible rows only).
- The three boxes are centered **as a group**; logo + actions centered over the same width;
  `lipgloss.Place` centers the whole block; `homeItemAt` reverses the offsets exactly
  (render == hit-test). Section headers/placeholders are non-clickable (no cell).
- LIBRARY always has a selected item (default first project, else first folder) so FILES is
  always populated regardless of focus.
- `gofmt`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit;
  `CGO_ENABLED=0 go build ./...` stays pure-Go.

---

## Task 1: Snippet cache (`snippet.go`)

**Files:** Create `snippet.go`, `snippet_test.go`

**Interfaces (Produces):**
```go
type snippetCache struct{ /* path → (mtime, text) */ }
func newSnippetCache() *snippetCache
func (c *snippetCache) get(path string) string // 1-line cleaned opening prose, "" on error
```

- [ ] **Step 1 — failing test** (`snippet_test.go`): write a temp `.md` with YAML frontmatter,
  an ATX heading, then a paragraph with `*emphasis*` and a `[link](x)`; assert `get` returns
  the paragraph prose with markdown stripped, whitespace collapsed, ≤80 runes, no `#`/`*`/`[`.
  A second test: a file > 1 MB whose prose is in the first 200 bytes still returns it (proves
  first-400-bytes read); and mtime invalidation (rewrite file → new snippet).
- [ ] **Step 2 — run RED.**
- [ ] **Step 3 — implement `snippet.go`:** `get` stats the path (mtime); on cache miss read
  `min(400, size)` bytes; `cleanSnippet(raw)`: drop a leading `---…---` frontmatter block, drop
  lines starting with `#`, strip leading `>`/`-`/`*`/`+`/`N.` markers, remove inline `*_`` ``[`
  `]`/`(`/`)` runes, collapse runs of whitespace/newlines to one space, trim, cut to 80 runes
  on a rune boundary. Cache and return.
- [ ] **Step 4 — GREEN; gofmt; vet; test; build; commit** `"home: snippet cache (first ~400
  bytes, markdown-stripped, mtime-cached)"`.

---

## Task 2: Library classification + FILES data (`home.go`)

**Files:** Modify `home.go`; Test `home_test.go`

**Interfaces:**
- Consumes: `snippetCache` (Task 1), `resolveManuscript`, `wordCountCache`.
- Produces:
```go
const ( homeRecentFile homeKind = iota; homeProject; homeFolder; homeNewDocument; homeNewProject; homeOpenOther )
type homeFileItem struct { name, path, snippet string; words int }
func classifyLibrary(workspace string) (projects, folders []homeItem) // manuscripts vs categories
func (m *model) recomputeHomeFiles()  // fills m.homeFiles from the selected library item
```

- [ ] **Step 1 — failing test:** a temp workspace with a manuscript dir (has `manifest.json`
  or `01-x.md`), a plain folder (loose `.md`s), and a hidden dir. Assert `classifyLibrary`
  puts the manuscript under projects, the plain folder under folders, excludes the hidden dir.
  A `recomputeHomeFiles` test: select the project → `m.homeFiles` lists its chapters (ordered)
  + loose with non-zero `words` and a non-empty `snippet`; select the folder → its `.md`s.
- [ ] **Step 2 — RED.**
- [ ] **Step 3 — implement:** add the `homeFolder` kind; `classifyLibrary` reads the workspace
  subdirs and uses okashi's manuscript test (manifest.json present OR a numerically-prefixed
  file) to split; `buildHomeItems` uses it (recents + projects + folders + actions).
  `recomputeHomeFiles` resolves the selected library item: Project → `resolveManuscript` order
  (chapter titles + loose); Folder → `allowedDocExts` files name-sorted; Recent-focus is NOT a
  library item (FILES follows the LIBRARY selection only). Fill `words` (m.files.wc) + `snippet`
  (m.snippets). Add model fields `homeFiles []homeFileItem`, `librarySelected int`, `snippets
  *snippetCache`; init `snippets` in `initialModel`.
- [ ] **Step 4 — GREEN; gofmt; vet; test; build; commit** `"home: classify projects vs folders;
  recompute FILES (ordered chapters/loose, words + snippet)"`.

---

## Task 3: Three-framed-column render, navigation, open (`home.go`, `main.go`) — CONTROLLER

**Files:** Modify `home.go`, `main.go`; Test `home_test.go`, `smoke_test.go`

**Interfaces:** Consumes Tasks 1–2. Replaces `homeContent`/`homeMove`/`homeCycleRegion`/
`openHomeSelection`/`resetHomeSelection`; regions become `regionRecent, regionLibrary,
regionFiles, regionActions`.

- [ ] **Step 1 — failing tests:** (a) `homeContent` returns three framed boxes (each line
  contains a column's content; titles `RECENT`/`LIBRARY`/`FILES` present), the block centered;
  (b) every `homeCell` round-trips through `homeItemAt` (including both lines of a two-line FILES
  row); (c) moving the LIBRARY selection down recomputes `m.homeFiles`; (d) `right` from RECENT
  → LIBRARY → FILES; `enter` on a FILES item sets screen=writing + loads the file; `enter` on a
  LIBRARY project opens its dir in the sidebar; (e) empty workspace → RECENT/LIBRARY placeholders,
  actions reachable, no panic.
- [ ] **Step 2 — RED.**
- [ ] **Step 3 — implement** (per the spec):
  - **Render:** build each column's inner lines (RECENT: recent names; LIBRARY: PROJECTS header
    + projects, FOLDERS header + folders, headers dim; FILES: header + two-line rows
    name/right-count then dim snippet). Wrap each in `framedPanel(title, inner, colW, colH, "")`;
    `JoinHorizontal(Top, recBox, gap, libBox, gap, filBox)`; equalize `colH`; window long lists
    per column. Compute `blockW`; center logo + actions over it.
  - **Cells:** as each clickable line is emitted, record a `homeCell{region, index, row, x0, x1}`
    in block-relative coords (add the box's border `+1` x / the box's top `+1` y + the column's
    x-origin within the joined block). Two-line FILES rows → two cells, same `(regionFiles, idx)`.
  - **Selection/nav:** `regionLibrary` selection is `m.librarySelected`; other regions use
    `m.homeIndex`; `m.homeRegion` is the focused column. `homeMove` left/right cross
    RECENT↔LIBRARY↔FILES (+ ACTIONS below); up/down within; moving LIBRARY calls
    `recomputeHomeFiles`. `resetHomeSelection` focuses RECENT (or first present) and sets
    `librarySelected` to the first project/folder, then `recomputeHomeFiles`.
  - **Open:** `openHomeSelection` — RECENT/FILES → load file (focus editor, SetDir to its dir);
    LIBRARY → SetDir to the container (sidebar); ACTIONS → existing actions.
  - **`ctrl+o`/startup:** rebuild items + `resetHomeSelection` + `recomputeHomeFiles`.
  - **Responsive:** if the 3-box width > screen, drop RECENT; if still too wide, single column.
- [ ] **Step 4 — empirically verify** (controller): render at 100×30 and 70×20; dump the three
  boxes; click each cell via the `homeCellXY`/`homeItemAt` round-trip; confirm LIBRARY→FILES
  live update + file open. Then GREEN; gofmt; vet; full suite; both builds; commit `"home:
  three framed columns (Recent · Library · Files) Miller launcher + snippets"`.

---

## Self-Review

**Spec coverage:** snippet cache → T1; classify + FILES data → T2; three framed columns +
nav + open + responsive + hit-test → T3. Recent-always-visible, library-drives-files,
file-granular open, centered-as-a-group all in T3.

**Placeholder scan:** none; T1/T2 give concrete code, T3 gives the concrete render/nav/cell
rules (controller-implemented with empirical geometry checks).

**Type consistency:** `homeFileItem`/`homeFolder`/`classifyLibrary`/`recomputeHomeFiles`
defined in T2, consumed in T3; regions renamed consistently; `snippetCache` from T1 used in T2.

**Risk:** the framed-column geometry + multi-line cell hit-test (controller re-verifies the
render==hit-test round-trip empirically, as with the 2-column home). Keep the default build
pure-Go (no new deps).
