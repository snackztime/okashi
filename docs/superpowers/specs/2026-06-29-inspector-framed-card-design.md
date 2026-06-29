# Inspector framed-card styling — design

**Date:** 2026-06-29
**Status:** Approved (mockup chosen)
**Context:** The inspector panel is visually flat — a left border, plain accent-text section
headers, sections running together. Make it "pop" as a defined **card**: a full rounded border
with the active tab name as the title, and labeled-rule section headers. (Chosen over per-section
boxes, which read busy and waste vertical space in a ~29-col panel.)

## Target look (chosen mockup, ~29-col inner)

```
╭ Words ─────────────────────╮
│ Words Outline Goals Analys │
│                            │
│ DOCUMENT ───────────────   │
│   Words            1,240   │
│   Characters       6,830   │
│   Paragraphs          18   │
│                            │
│ PROJECT ────────────────   │
│   Words           42,100   │
│   Chapters             7   │
╰────────────────────────────╯
```

## Components

- **`framedPanel(title, inner string, width, height int) string`** — wraps `inner` (the panel's
  multi-line content) in a rounded box (border in `subtle`), with `title` (accent) injected into the
  top border: `╭ {title} {─ fill} ╮`. Each inner line is padded/truncated ansi-aware to the content
  width; the box is padded/truncated to `height`. Build the top border manually (so the title can be
  accent-styled cleanly); sides `│ … │` with one space of padding; bottom `╰{─}╯`.
- **`sectionHeader(label string, width int) string`** — `accent+bold` UPPERCASE label + a space +
  `subtle` `─`×(fill to width). Replaces the `breadcrumbStyle.Render("Document")`-style headers in
  every tab (Document/Project, Daily/Project, Analysis/Syntax, Outline).
- **Content indent:** rows under a header get a 2-space indent (matches the mockup). Apply to the
  kv rows, checkboxes, progress lines, and outline rows.
- `inspector.View` returns the **inner** content (tab bar + indented sections with rule headers);
  `main.go` calls `framedPanel(activeTabLabel, inner, panelW, panelH)` and places that in the column
  (replacing the old `inspectorStyle` left-border wrap). The active tab label = `inspectorTabLabels()[in.tab]`.

## Geometry & click alignment (the careful part)

The frame changes the panel's content origin, so the mouse hit-tests MUST move with it:
- **Content width** shrinks: full box = 2 border cols + 2 padding cols → inner = `panelWidth - 4`
  (was `-3` with the left-only border). Update `inspectorInnerWidth()` accordingly and keep it the
  single source the tab bar / `View` width / placement all use.
- **Vertical:** the top border is row 0; the **tab bar is now row 1**; section rows shift down by 1.
  `analysisRowY(i)` and the tab-row click test must reflect this (tab row at the framed offset, not 0).
- **Horizontal:** content starts at col 2 (left border + 1 padding) — same as today's left-border +
  padding, so the X offset is unchanged; only the right border is new.
- `main.go`'s `MouseMsg` handler (tab click + Analysis checkbox click) must use the framed origin
  (the `+1` row for the top border). A render-based test asserts each clickable element's on-screen
  position equals its hit-test target — the gate for this change (same approach that caught the
  tab-bar wrap).

## Testing

- `framedPanel`: output has a rounded top with the title (`╭ Words `), `│`-bordered content lines all
  the same width, a rounded bottom; an over-long inner line is truncated, not overflowed; height is
  respected.
- `sectionHeader("Document", 24)` → starts with `DOCUMENT`, fills to width with `─`.
- **Alignment (gate):** render the full app with the inspector open; assert the on-screen row/col of
  each tab and each Analysis checkbox equals its hit-test target (`inspectorTabAtX`/`analysisRowY`
  + the main.go offsets) — clicking Spellcheck toggles Spellcheck, clicking the Goals tab switches to
  Goals, etc.
- Existing inspector tests updated to the new geometry (the `TestTabBarFitsOneRow`,
  `TestAnalysisRowAtY`, tab-click, checkbox-click tests); the tab bar still fits one row at the new
  (narrower) inner width.

## Out of scope

- Per-section sub-boxes; configurable border styles/colors; titles on tabs other than the active one.
- Changing any inspector *data* (Words/Goals/Outline/Analysis content is unchanged — only chrome).
