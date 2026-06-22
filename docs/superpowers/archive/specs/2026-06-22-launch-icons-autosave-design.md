# Launch screen + file-pane icons + autosave — design

**Date:** 2026-06-22
**Status:** Approved (pending spec review)
**Roadmap:** New launch/home experience, superfile-style file pane, and autosave.

## Goal

Make okashi feel like a real writing app:

1. **Two screens** — a Crush-style **launch screen** (logo + recent files + projects) that collapses into a **minimal writing zone** (no banner) once you're in a file.
2. **Superfile-style file pane** — Nerd Font icons for folders/files (plain fallback), no permission clutter.
3. **Autosave** — debounced ~2s-idle saving so work is never lost.
4. **Focus dimming** — sentence-level (iA-style) dimming as part of focus mode.

Note: the "showing `-rw-r-xr-x`" report was a **stale `./okashi` binary** (the old `filepicker`). The current `filelist` already renders names only; this spec adds icons on top.

---

## Section 1 — Two screens: `home` and `writing`

A new top-level mode on `model`:

```go
type screen int
const (
    screenHome screen = iota
    screenWriting
)
```

- **Launch → `screenHome`.** Renders the logo + launch picker (Section 2) full-screen. No editor, no sidebar, no status bar.
- **Opening/creating a file → `screenWriting`.** Renders sidebar + editor + status **with no banner** — the minimal writing zone, reclaiming the ~4 banner rows.
- **`ctrl+o`** (in `writing`) returns to `screenHome`, rebuilding the recent/projects lists first.
- `View()` and `layout()` branch on `screen`. The banner (`bannerView`) renders **only** in `home`. In `writing`, `bodyH = height - 1` (status only); in `home`, the full height is the picker.

**Testing:** model starts in `screenHome`; selecting a recent/project transitions to `screenWriting`; `ctrl+o` returns to `screenHome`.

---

## Section 2 — Launch screen (Recent + Projects)

The picker is the entry ramp; the **sidebar remains the full in-project browser** (`ctrl+b`, unchanged). The picker hands off to it.

### Layout

```
                 [ okashi logo, centered ]

  RECENT
    chapter-03.md      my-novel/
    journal-06-22.md   journal/

  PROJECTS
    my-novel
    journal
    essays

    Open another folder…
```

### Content

- **Recent** — files from the recent store (Section: Persistence), shown as `<icon> basename   <dimmed parent-dir>/`. Most-recent-first. Entries whose path no longer exists are filtered out on load.
- **Projects** — immediate subdirectories of the okashi writing dir (`writingDir()`), sorted alphabetically, hidden dirs excluded.
- **Open another folder…** — a fixed final action.

### Model & navigation

A flat selectable list built from the two groups plus the action:

```go
type homeKind int
const (
    homeRecentFile homeKind = iota
    homeProject
    homeOpenOther
)
type homeItem struct {
    kind  homeKind
    label string // display text (basename / project name / "Open another folder…")
    path  string // file path, project dir, or "" for the action
}
```

`model` holds `homeItems []homeItem` and `homeSelected int`. Group headers ("RECENT"/"PROJECTS") are render-only; only items are selectable. Up/down move `homeSelected` (clamped); Enter dispatches:

- **homeRecentFile** → `loadFile(path)`; sidebar root = `filepath.Dir(path)`; `screen = screenWriting`.
- **homeProject** → sidebar root = `path` (the project dir), sidebar open + focused, no file loaded yet (empty editor); `screen = screenWriting`. User picks a file or `ctrl+n`.
- **homeOpenOther** → sidebar root = `writingDir()`, sidebar open + focused; `screen = screenWriting`. Browse anywhere via the sidebar's `..` navigation.

### Empty states

- No recents → omit the RECENT group. No projects → omit PROJECTS. First run (neither) → logo + just "Open another folder…" with a one-line hint ("No files yet — open a folder to start writing").

### Building the list

`func buildHomeItems(recents []string, projectsDir string) []homeItem` — pure given inputs (recents already filtered; reads `projectsDir` subdirs). Testable directly.

**Testing:** build over a temp dir with recents + subdirs → expected ordered items; empty states; Enter dispatch sets the right sidebar root + screen.

---

## Section 3 — File-pane icons (Nerd Font + plain fallback)

A new `icons.go`:

```go
type iconSet struct {
    folder, parent, file string
    byExt map[string]string
}
func resolveIcons() iconSet // chosen once at startup
```

- **Nerd Font set** (default): folder ``, parent ``, default file ``, and `byExt` for `.md `, `.markdown `, `.txt `, `.wg `, `.go `, etc. (trailing space in each glyph string for padding).
- **Plain set** (`OKASHI_ICONS=plain` or `=ascii`): folder `▸`, parent `↑`, file ` ` (space), empty `byExt` (everything uses `file`). Single-width Unicode, no special font.
- Resolution mirrors `previewStyle()`: read env once, store on the model (`icons iconSet`).

`icon(e fileEntry) string` returns the glyph for an entry (parent if name == "..", else folder, else `byExt[ext]` or `file`).

### Rendering changes

- `filelist.View()` renders `<icon><name>` (icon string already padded). The `/` dir suffix is **removed** (the icon conveys "folder"). Folders tinted with `accent`, files default; selection highlight unchanged. The filelist gets an `icons iconSet` field set from the model.
- The launch screen's Recent/Projects lists reuse `icon()` for consistency.

**Testing:** `resolveIcons()` returns the plain set when `OKASHI_ICONS=plain`, Nerd set otherwise; `icon()` maps `..`/dir/`.md`/unknown correctly. (Pure functions; no terminal.)

---

## Section 4 — Autosave (debounced ~2s idle)

Active in `screenWriting` only.

### Mechanism

- `Init()` starts a **single** recurring `tea.Tick` (~1s) producing `autosaveTickMsg`; it runs for the app's lifetime (one loop only — never started a second time). It is harmless in `home` because `dirty` is only ever set while editing.
- Editor edits (any key routed to the editor) set `m.dirty = true` and stamp `m.lastEditAt = time.Now()`.
- On `autosaveTickMsg`: if `m.dirty && m.currentFile != "" && time.Since(m.lastEditAt) >= 2s` → `m.save()` (writes, clears `dirty`). Always reschedule exactly one next tick.
- Re-saving identical content is skipped via the existing dirty flag (no write if `!dirty`).

### Scope & safety

- Only autosaves a buffer with a path (`currentFile != ""`). `ctrl+n` assigns the path up front, so new files autosave too.
- On a write error, `save()` sets the error status and **leaves `dirty = true`** → retried next tick. Work is never silently dropped.
- Manual `ctrl+s` unchanged; it clears `dirty`.

### Indicator

The status bar's right side (currently `N words · +N session`) gains a leading mark: `●` (accent) when there are unsaved changes, `✓` (subtle) when clean. So: `✓ 1,240 words · +142 session`.

### Testability

- Decision isolated as `func (m model) autosaveDue(now time.Time) bool` (checks dirty + currentFile + idle ≥ 2s) — unit-tested directly.
- The tick handler calls `autosaveDue(time.Now())` then `save()`. Test: set `dirty`, `lastEditAt` in the past, feed `autosaveTickMsg` → file on disk updated; not-yet-idle → no write; no currentFile → no write.

---

## Section 5 — Focus dimming (sentence-level, iA-style)

Part of "focus mode": dims everything except the sentence under the cursor.

### Behavior

- A `dimEnabled bool` setting, **default true**. Dimming is active when
  `typewriter && dimEnabled`. So `ctrl+t` (typewriter) is the master focus-mode
  switch (centered caret + dimming); a **dedicated toggle key turns just the
  dimming off/on** while typewriter stays on (centering without dimming).
- Toggle key: a dedicated key finalized in the plan after confirming it's
  unbound in the vendored textarea's KeyMap (candidate: `ctrl+d`). Status shows
  the state ("dim on"/"dim off").
- Forward-looking: `dimEnabled` is a plain setting so the future options pane
  (out of scope, below) can drive it without refactoring.

### Rendering (vendored textarea patch)

- When active, the render loop styles each character by whether it falls within
  the **current sentence span**: inside → normal foreground; outside → `subtle`
  (dim). The current sentence is the gleaming line; with typewriter it's the
  centered one.
- **Current sentence span:** from the previous sentence terminator (`.`/`!`/`?`
  followed by whitespace) — or the paragraph start — up to and including the
  next terminator. A blank line (paragraph break) is a hard boundary on both
  sides, so dimming never bleeds across paragraphs.
- This is the deepest patch: the typewriter patch only set a scroll offset; this
  styles per character based on the character's absolute text offset. The plan
  details the offset mapping inside the existing wrapped-line render loop.

### Testability

- Pure `currentSentenceSpan(text string, cursorOffset int) (start, end int)` —
  unit-tested: mid-sentence, on a terminator, first/last sentence, multi-line
  sentence, paragraph-break boundaries, empty buffer.
- Integration: with dimming active, characters outside the span carry the dim
  style and characters inside do not (assert on the rendered output).

---

## Persistence — recent files

- **Location:** `<os.UserConfigDir()>/okashi/recent.json` (e.g. `~/Library/Application Support/okashi/recent.json` on macOS). Created lazily.
- **Format:** `{"files": ["<abs path>", ...]}` — most-recent-first, deduped by path, capped at **15**.
- **Updates:** on successful `loadFile`, and on the first successful `save` of a new file — prepend the path (removing any existing dupe), cap, write.
- **Load:** parse the file; drop entries whose path no longer exists; corrupt/missing → empty list (no error surfaced).
- **Testability:** `recentStore` takes its file path as a parameter (or a package var overridable in tests) so tests use a temp dir. Functions: `loadRecents(path) []string`, `addRecent(path, file) error`.

---

## Out of scope (non-goals)

- **Settings/options pane** (a right-side panel for dimming, spelling, syntax,
  etc.) — a future feature with its own design; `dimEnabled` is pre-structured
  for it. Spell-check and syntax highlighting are likewise future, separate.
- **Editor-core rope buffer** (roadmap #5) — deferred until a real performance
  problem appears (YAGNI).
- Fuzzy-filtering the launch list (plain up/down for now).
- Quick "new note" directly from home (use project → `ctrl+n`).
- Recursive/nested project discovery (only immediate subdirs of the okashi dir).
- Auto-detecting Nerd Font availability (opt-out via env instead).
- Editor mouse caret-positioning; preview wheel already works.

## Risks

- **Nerd Font glyphs** show as tofu without a Nerd Font — mitigated by `OKASHI_ICONS=plain`; documented in README.
- **Autosave timer churn** — a 1s tick is cheap; writes only happen on real edits after idle. Confirm no perceptible lag in a real terminal.
- **Banner removal in `writing`** changes `layout()` height math — verify the editor/sidebar fill correctly and the typewriter centering still holds with the reclaimed rows.
- **recent.json across machines** is local (not synced); acceptable.

## Build order (for the plan)

1. **Autosave** — independent, highest safety value.
2. **Icons** — local to `filelist`/`icons.go`.
3. **Focus dimming** — vendored-textarea render patch; pairs with typewriter.
4. **Launch screen + screens** — the structural change; depends on icons for the recent/project rows.
