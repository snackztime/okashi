# Inspector foundation (shell + Words tab) — design

**Date:** 2026-06-28
**Status:** Approved (pending spec review)
**Context:** First, foundation-first slice of the planned right-side **inspector** panel (see
memory `inspector-outline-roadmap`). This cycle builds the panel shell — a toggleable right
column with a tab framework + the three-column layout — and its first tab, **Words**
(reading stats). Outline document/view and the Goals/Analysis tabs are later cycles.

**Scope discipline:** okashi is lean and the scope oracle for the macOS app. This adds one
read-only info panel reflecting the open doc + project — no editing, no new persistence.

## Layout (three columns)

The writing screen becomes `[sidebar] │ editor (centered) │ [inspector]`.

- `inspectorWidth = 32` (matches `sidebarWidth`).
- `minEditorMeasure = 50` — the writing-measure floor that triggers auto-hide.
- **Effective-panel rule** (computed every layout/View; never mutates `m.sidebarVisible`):
  a shared helper returns `(showSidebar, showInspector bool, editorArea int)`:
  - `showInspector = m.inspector.visible` (writing screen only).
  - `showSidebar = m.sidebarVisible`.
  - If `showInspector && showSidebar` and `width - sidebarWidth - inspectorWidth <
    minEditorMeasure`, set `showSidebar = false` (suppressed *for this render* while the
    inspector is open; it returns when the inspector closes or the window widens).
  - `editorArea = width - (showSidebar?sidebarWidth:0) - (showInspector?inspectorWidth:0)`.
  Both `layout()` (sizing) and `View()` (composition) call this helper so they never diverge.
- `View()` composes the columns with `lipgloss.JoinHorizontal`; the editor still centers in its
  area via `lipgloss.Place`. The inspector renders in `inspectorStyle` — a panel with a
  **left** rounded border (mirroring `sidebarStyle`'s right border) + padding.

## Toggle

- `ctrl+i` toggles `m.inspector.visible`, handled in the **writing-screen** update only (the
  inspector reflects the open doc; it does not apply on home/outline/pager). Calls `layout()`.
- Status-bar hint gains `ctrl+i inspector`.

## Shell (`inspector.go`, new)

```go
type inspectorTab int
const tabWords inspectorTab = 0 // more tabs (Goals, Analysis, Outline) added later

type inspectorModel struct {
	visible bool
	tab     inspectorTab
}

// View renders the tab bar + the active tab's body, fit to width/height.
func (in inspectorModel) View(width, height int, doc docStats, proj projStats) string
```

- **Tab bar:** rendered from a slice of tab labels with the active one highlighted (accent).
  Only `Words` exists now; the slice/abstraction is in place so a future tab is one entry.
  The **tab-switch key is deferred** to the cycle that adds tab #2 (nothing to switch to yet).
- Read-only, non-focusable: `ctrl+b`/`esc` focus logic is untouched; the inspector never takes
  focus this cycle.

## Words tab data

```go
type docStats  struct{ words, chars, paragraphs int }
type projStats struct{ words, chapters int; manuscript bool }
```

- `computeDocStats(text string) docStats`: `words = wordCount(text)` (existing);
  `chars = utf8.RuneCountInString(text)`; `paragraphs` = count of non-empty blocks when
  splitting on blank lines (a run of `\n` with only whitespace between). Empty buffer → zeros.
  Computed from `m.editor.Value()` each render — O(chapter), cheap.
- `projStats`: from the current file pane's resolved manuscript — `words` = sum of chapter word
  counts via the existing `wordCountCache`; `chapters` = number of resolved chapters;
  `manuscript = chapters > 0`. Reuses `m.files` (its dir, resolved chapters, `wc`) so it
  matches the sidebar's numbers. Non-manuscript context → `manuscript = false`, and the body
  shows the folder's document-word total with the "Chapters" line omitted.

Rendered body (labels left, right-aligned numbers, `commafy` for thousands):
```
◇ WORDS

Document
  Words       1,204
  Characters  6,830
  Paragraphs     38

Project
  Words      47,032
  Chapters       12
```

## Testing

- `computeDocStats`: words/chars/paragraphs for a multi-paragraph sample; empty → all zero;
  trailing blank lines don't inflate the paragraph count.
- `projStats` for a manuscript dir: total words = sum of chapters, `chapters` correct,
  `manuscript = true`; for a plain folder: `manuscript = false`.
- `inspectorModel.View` renders "WORDS" (active tab), "Document"/"Project" sections, and the
  numbers; non-manuscript omits "Chapters".
- Effective-panel helper: wide window → all three columns (`showSidebar && showInspector`,
  `editorArea` correct); narrow window with inspector open → `showSidebar == false` and the
  editor keeps ≥ `minEditorMeasure`; inspector closed → sidebar returns.
- `ctrl+i` toggles `inspector.visible` on the writing screen; a smoke check that the writing
  `View()` contains the inspector body when visible and omits it when not.

## Out of scope (later cycles)

- The outline **document** (separate `.md`, editor toggle) and its read-only **Outline view** tab.
- **Goals** tab (word-count progress bar, daily goal) and **Analysis** tab (spellcheck/syntax).
- The tab-switch key (added with tab #2). Inspector focus/interaction (it's read-only here).
- Per-chapter breakdown in the inspector (the sidebar already shows per-chapter counts).
