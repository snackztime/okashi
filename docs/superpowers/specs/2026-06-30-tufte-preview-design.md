# Tufte preview style + footnotes — design

**Date:** 2026-06-30
**Status:** Approved
**Context:** The `ctrl+p` markdown preview renders through glamour with a generic dark/light
theme, and shows footnote syntax literally (a known glamour limitation). Give the preview a
**style toggle (Default ⇄ Tufte)** mirroring the export's Manuscript/Tufte choice, and make
footnotes render readably (the higher-value half — footnotes/sidenotes are central to Tufte).

## Style toggle

- Model field `previewTufte bool` (default off). While previewing, **`t`** toggles it (the
  editor is read-only in preview, so a plain letter is free; documented in the preview footer
  + F1). The preview re-renders on toggle.
- **Default** style: the current `glamour.WithStandardStyle(m.mdStyle)` (dark/light), unchanged.
- **Tufte** style: a custom `glamour.StyleConfig` (`tufteGlamourStyle`) selected via
  `glamour.WithStyles(...)` instead of `WithStandardStyle`. Tuned for the readable-book feel
  within terminal limits: warm/parchment foreground, restrained headings (no heavy `#`
  markers — small-caps-ish via case where practical, a thin rule under H1/H2), comfortable
  paragraph spacing, italic emphasis, muted blockquote/rule, monospace code unchanged. No
  truecolor dependency beyond what glamour already emits (degrades on 256).
- The preview header shows the active style: `PREVIEW · Tufte` / `PREVIEW · Default`.

## Footnotes (both styles)

glamour has no footnote extension, so **pre-process the markdown before rendering**
(`footnotesToEndnotes(md string) string`):

1. Collect definitions `^\[\^(id)\]:\s*(text)` (text may continue on indented continuation
   lines); remove them from the body.
2. Number them in first-reference order; replace each inline reference `\[\^id\]` with a
   superscript-ish marker — `¹²³…` where representable, else `[n]`.
3. Append a `Notes` section at the end: a horizontal rule, a `Notes` heading, then a numbered
   list `n. text`. (Matches how the PDF export already turns footnotes into per-chapter
   endnotes — consistent mental model.)
4. Unreferenced definitions are dropped; references with no definition keep the literal marker.

This runs in `togglePreview`'s render path for BOTH styles (it's a glamour-limitation fix, not
Tufte-specific). Pure string transform, no goldmark dependency in the preview path.

## Wiring (`main.go`)

- `togglePreview` / the preview render: `md := footnotesToEndnotes(buffer)`, then
  `glamour.NewTermRenderer(styleOpt, glamour.WithWordWrap(wrap))` where `styleOpt` is
  `WithStyles(tufteGlamourStyle)` when `previewTufte` else `WithStandardStyle(m.mdStyle)`.
- The `t` toggle in the preview key handling re-renders into `m.preview` (the viewport).
- `previewTufte` persists for the session (optionally seedable from `OKASHI_PREVIEW_STYLE` or
  the last export style — keep to a session toggle in v1).

## Edge cases

- A document with no footnotes → `footnotesToEndnotes` returns it unchanged (no empty Notes
  section).
- Malformed/duplicate footnote ids → stable first-wins numbering; a definition without a
  reference is dropped; a reference without a definition keeps `[id]`.
- Very long preview → the existing viewport windowing handles scroll; the style change doesn't
  affect wrap width.
- Terminals without italics/colors → glamour degrades; the endnotes still render as text.

## Out of scope (later)

- True Tufte sidenotes/margin notes (impossible in a single-column terminal — endnotes are the
  faithful terminal analog).
- A shared semantic theme JSON with the macOS app (CLAUDE.md §3, still aspirational).
- Footnote support in the editor's live view (this is preview-only).

## Testing

- `footnotesToEndnotes`: reference + definition → marker + a numbered Notes section; multiple
  refs to one id reuse the number; continuation lines captured; no-footnote input unchanged;
  orphan reference keeps its marker; orphan definition dropped.
- the Tufte `StyleConfig` builds a renderer without error and produces non-empty styled output;
  the toggle flips `previewTufte` and the header label; the default path is byte-identical to
  today for footnote-free input.
