# Outline document + inspector Outline tab — design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)
**Context:** Cycle 1 of the inspector roadmap's remaining work (see memory
`inspector-outline-roadmap`). Adds a per-project **planning outline document** (free-form
nested-bullet markdown), editable in the main editor via a toggle, and shown read-only in the
inspector's new **Outline** tab. Resolves the "outline" naming clash by reclaiming the word
for the planning doc and renaming the existing chapter-navigator screen to **Binder**.

## Keymap (confirmed)

- **`ctrl+l` → Outline doc** — toggles the main editor between the current chapter and the
  project's `outline.md` (creates it if absent), saving on the way in/out.
- **`ctrl+k` → Binder** — the existing chapter-navigator screen (reorder/new/rename/read),
  **renamed** from "outline" to "Binder" (user-facing labels only; behavior unchanged).
- **`ctrl+y`** now **cycles** inspector tabs: Words → Outline → (past the last) closes.

## Outline document

- File: `outline.md` in the current project folder (`m.files.dir`). A visible, unnumbered
  **Resource** — already excluded from manuscript order/export by the filename convention; no
  special handling needed. It appears in the sidebar like any loose doc.
- Model state: `outlineReturnFile string` (the chapter to return to).
- **`ctrl+l` toggle** (writing screen only):
  - If `m.currentFile == <dir>/outline.md`: `m.save()` (the outline), then `loadFile(outlineReturnFile)` if set (back to the chapter).
  - Else: `m.save()` (the chapter), set `outlineReturnFile = m.currentFile`, ensure `outline.md`
    exists (`atomicWrite(path, []byte("- \n"), 0o644)` — a starter bullet hinting the format —
    if `os.Stat` fails, then `m.files.SetDir(m.files.dir)` to surface it in the sidebar), then
    `loadFile(outline.md)`.
  - On create failure, set `m.status` and abort (no state change).
- The doc is plain markdown — `save()` (already atomic) writes it; it round-trips the corpus.

## Binder rename (the existing ctrl+l screen)

- Move the current `ctrl+l` body to a new **`ctrl+k`** case: `if m.files.view.ordered() {
  m.enterOutline() } else { m.status = "not a manuscript" }`.
- `ctrl+l` becomes the outline-doc toggle (above).
- Relabel user-facing "outline" → "Binder" in the three status hints:
  - Writing hint (`initialModel`): `ctrl+l outline` → `ctrl+l outline · ctrl+k binder`.
  - Binder-screen hint: `outline · ↑↓ select · enter open · r rename · m read · ctrl+e export · esc back` → `binder · …`.
  - Pager hint: `… o outline …` → `… o binder …`.
- Internal identifiers (`screenOutline`, `outlineModel`, `enterOutline`) stay as-is (no churn);
  only user-facing strings change. The binder's behavior is untouched.

## Inspector Outline tab

- `inspector.go`: `const ( tabWords inspectorTab = iota; tabOutline )`;
  `func inspectorTabLabels() []string { return []string{"Words", "Outline"} }` (shared by the
  tab bar render AND the cycle so they never diverge).
- `cycle()` on `*inspectorModel`: if hidden → show + `tab = tabWords`; else `tab++`, and if
  `int(tab) >= len(inspectorTabLabels())` → hide + reset `tab = tabWords`.
- `View` gains an `outline string` param: `View(width int, doc docStats, proj projStats,
  outline string) string`. The tab bar renders both labels (active highlighted via
  `selectedStyle`). Body switches on `in.tab`: `tabWords` → the existing Words body;
  `tabOutline` → the outline render.
- **Outline render:** empty/whitespace → subtle `(empty — ctrl+l to edit)`. Otherwise each
  line wrapped to `width`; top-level bullets (no leading whitespace) in `accent`, nested lines
  in default. (Plain, readable; the nested-bullet structure carries the hierarchy.)
- `main.go` `View()`: when the inspector is visible, read the outline via a helper
  `readOutlineDoc(dir string) string` (`os.ReadFile(dir/outline.md)`, "" on error — tiny file,
  read only when the panel is up) and pass it to `inspector.View`. `ctrl+y` handler becomes
  `m.inspector.cycle(); m.layout()`.
- The existing `TestInspectorViewRendersWords` call gets the new `""` outline arg.

## Testing

- **Outline toggle:** on a temp manuscript (a numbered chapter open), `ctrl+l` → `currentFile`
  is `outline.md` and the file now exists on disk; `ctrl+l` again → `currentFile` is the
  original chapter (returned via `outlineReturnFile`).
- **Binder rebind:** `ctrl+k` on a manuscript → `m.screen == screenOutline`; `ctrl+l` no
  longer opens `screenOutline`.
- **Tab cycle:** hidden → `cycle()` → visible+`tabWords` → `cycle()` → visible+`tabOutline` →
  `cycle()` → hidden (`tab` reset).
- **Outline tab render:** `View(..., tab=tabOutline, outline="- Top\n  - sub")` contains "Top"
  and "sub"; empty outline → contains "empty".
- Existing inspector/smoke tests stay green (Words tab unaffected; `ctrl+y` still shows the
  panel on first press).

## Out of scope (later cycles)

- Goals tab (word-count progress bar, daily goal) — cycle 2.
- Analysis tab (spellcheck, live syntax) — cycle 3 (likely split).
- Rich outline rendering (collapsible nodes, click-to-jump) — the read-only text render is enough.
- Multiple outline docs / per-chapter outlines — one `outline.md` per project folder.
