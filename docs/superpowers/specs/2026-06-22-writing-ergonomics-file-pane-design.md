# Writing ergonomics + file-pane breadcrumb/confinement — design

**Date:** 2026-06-22
**Status:** Approved (pending spec review)
**Queue note:** This batch first, then **focus dimming** (Plan 2, already spec'd), then a **project rename** (okashi → TBD) — the rename will sweep the `OKASHI_*` env vars, the `okashi` workspace folder name, module path, repo, and Homebrew formula.

## Goal

Basic word-processor ergonomics plus a self-contained, signposted file pane:

1. **Editor input**: `esc` pane-switch, `Tab`/`Shift+Tab` indent/outdent, auto-continue Markdown lists, smart curly quotes.
2. **Configurable column width** (default 80 → 65, `OKASHI_WIDTH`).
3. **File pane**: a breadcrumb header relative to the okashi root, and navigation confined to that root.

---

## Section 1 — Editor input & keybindings

The editor is the vendored `okashi/internal/textarea`. Indent/outdent become exported methods on it (preserving cursor/undo); list-continuation and smart-quotes are handled in the app's key path before forwarding to the editor.

### 1a. `esc` toggles pane focus (replaces Tab)

- Remove the `case "tab"` focus-toggle. Add `case "esc"`: in writing mode, toggle focus editor ↔ sidebar (only when `sidebarVisible`, matching today's tab behavior). Bonus: if `previewing`, `esc` exits preview (same as `ctrl+p`).
- `esc` while `creatingFile` keeps its current meaning (cancel the name prompt) — that branch runs first, unchanged.

### 1b. `Tab` / `Shift+Tab` → indent / outdent

- Indent unit = **2 spaces** (a `const indentUnit = "  "`).
- Add to the vendored textarea: `Indent()` (insert the unit at the cursor) and `Outdent()` (remove up to one unit of leading whitespace from the current line; cursor adjusts). These manipulate `m.value`/cursor directly so undo and wrapping stay correct.
- App: `case "tab"` → `m.editor.Indent()`; `case "shift+tab"` → `m.editor.Outdent()` — only when the editor is the focused pane (else ignored).

### 1c. Enter auto-continues Markdown lists

- When the editor is focused, the cursor is at the **end of the current line**, and that line matches `^(\s*)([-*+]|\d+\.)\s+(.*)$`:
  - **Non-empty content** → consume Enter; insert newline + same indent + marker (for `N.`, the next integer).
  - **Empty content** (marker only) → consume Enter; clear the marker from the current line, leaving the cursor on the now-blank line (ends the list; no extra newline — matches common editors).
- Otherwise Enter is a normal newline (forwarded to the editor).
- A pure helper `listContinuation(line string) (prefix string, clear bool, ok bool)` computes the action; the app applies it via the textarea's insert/clear-line API.

### 1d. Smart curly quotes

- A `smartQuotes bool` setting, **default true**; `OKASHI_SMARTQUOTES=off` (or `false`/`0`) disables it (resolved once at startup; the future settings pane will also drive it). No dedicated key.
- When on and the editor is focused, typing `'` or `"` inserts the contextual curly glyph instead of the straight one:
  - Opening (`'` U+2018, `"` U+201C) if the char before the cursor is empty/whitespace or an opening bracket `([{`.
  - Closing (`'` U+2019, `"` U+201D) otherwise — this also yields the right apostrophe in contractions (`don't` → `don't`).
- A pure helper `smartQuote(prev rune, q rune) rune` decides; the app inserts the result and consumes the original key.

### Input routing

`esc`, `tab`, `shift+tab` become top-level `KeyMsg` cases (like the `ctrl+*` keys). Enter-as-list and quote-smartening are handled in the editor-routing branch (where keys currently forward to `m.editor`): inspect the `KeyMsg`; if it's a list-Enter or a smart-quote, do the special handling and skip the normal `editor.Update`; otherwise forward as today (dirty-tracking unchanged).

Because the special handlers (`Tab`/`Shift+Tab` indent, list-continuation, smart-quote insert) mutate the buffer outside the normal forward path, each must also set `m.dirty = true` and `m.lastEditAt = time.Now()` so autosave still fires.

### Testing (Section 1)

- `Indent`/`Outdent` (vendored textarea test): cursor/line state before/after.
- `listContinuation`: bullet, numbered (increment), nested indent, empty-item-ends, non-list line → ok=false.
- `smartQuote`: opening vs closing for both quote types across prev-char cases.
- Model-level: `tab`→indent, `shift+tab`→outdent, `esc`→focus toggle, Enter continues/ends a list, `'` becomes curly.

---

## Section 2 — Configurable column width

- `resolveColumnWidth() int`: read `OKASHI_WIDTH`; if a valid integer in **[20, 200]**, use it; else default **65**. Resolved once at startup, stored as `model.colWidth` (replacing the `columnWidth` const in `layout`).
- `layout` uses `m.colWidth` everywhere it used `columnWidth`.
- **Testing:** `resolveColumnWidth` honors a valid env value, clamps/falls back on out-of-range or garbage, defaults to 65 unset.

---

## Section 3 — File pane: breadcrumb + okashi-root confinement

### 3a. Confinement

- `filelist` gains a `root string` (= `writingDir()`), set in `newFilelist`/when the launch screen roots the pane.
- `SetDir(dir)` clamps: never set a directory above `root` (if `dir` is not `root` and not within `root`, snap to `root`).
- The `".."` entry is added only when `f.dir != f.root`. `activate()`/left/backspace up-navigation is a no-op at the root.
- Launch screen: the `homeOpenOther` label becomes **"Browse all files"** (still roots the sidebar at `writingDir()`); projects remain immediate subdirs of the root.

### 3b. Breadcrumb header

- A styled header line at the top of the sidebar, the path **relative to root**: `filepath.Base(root)` at the top (e.g. `okashi`), then each subdir joined by `" / "` (e.g. `okashi / Book Name`). A pure `breadcrumb(root, dir string) string` computes it.
- `filelist.View()` renders the header (accent style) then the file list. The list's visible height drops by the header height (1 row).
- **Mouse mapping:** the file list now starts `headerHeight` rows below the sidebar top, so the click→row offset becomes the header height instead of 0. `sidebarRow(msg.Y, headerHeight, m.files.height)` (headerHeight = 1).
- `layout` reduces `m.files.height` by the header height.

### Testing (Section 3)

- `SetDir` clamps at/above root; `".."` present only below root.
- `breadcrumb(root, dir)`: root → base name; nested → joined relative path.
- Mouse row mapping accounts for the header offset (pure `sidebarRow` already takes the offset; verify the caller passes header height).

---

## Out of scope (non-goals)

- Settings/options pane and spell-check/syntax (future, separate).
- Focus dimming (its own Plan 2).
- The project rename (queued after dimming).
- Multi-line selection indent (the textarea has no selection model; indent/outdent act on the current line).
- Smart-quote awareness of code fences (applies everywhere when on; toggle off for code-heavy writing).

## Risks

- **Indent/outdent in the vendored textarea** must keep cursor + soft-wrap + undo correct — covered by the editor patch + tests; verify in a real terminal.
- **Smart quotes** intercept raw `'`/`"` input — ensure they only fire in the editor (not the filename prompt) and never block the toggle-off path.
- **Breadcrumb header offset** is the same cross-cutting class as the banner removal: the header height must be reflected in BOTH `layout` (list height) and the mouse offset, or clicks misalign.
- **Confinement vs recents:** a recent file outside the root could exist from before; opening it still works (loadFile), but the sidebar root stays the okashi root — acceptable.

## Build order (plans)

Split into two plans for this batch (dimming is a separate Plan 2, already spec'd):

- **Plan A — Editor ergonomics:** Sections 1 + 2 (esc/tab/lists/quotes/width).
- **Plan B — File pane:** Section 3 (breadcrumb + confinement).
