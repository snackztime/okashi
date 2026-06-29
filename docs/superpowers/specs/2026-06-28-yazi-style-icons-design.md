# Yazi-style file-pane icons — design

**Date:** 2026-06-28
**Status:** Approved (pending spec review)
**Context:** Make okashi's file/library pane look more like the user's yazi setup —
per-filetype Nerd Font glyphs, each colored by type (the way yazi does), drawn from the
user's Dracula palette. okashi already has a small Nerd Font set (folder, file, 4 extensions)
+ a `plain` ASCII fallback; this expands it to a curated writing-relevant set and adds
per-type color.

## Goal

Each file/folder type resolves to a **glyph + a color**. The pane renders the glyph in its
type color (folders cyan, pdf red, images green, etc.); the base palette (purple selection,
accent, subtle) is unchanged. The `OKASHI_ICONS=plain` ASCII set stays monochrome (its point
is no-Nerd-Font terminals). Same glyphs flow to the home-screen recents (shared `icon()`).

**Scope:** okashi-local (terminal Nerd Font glyphs); the macOS app uses its own iconography,
so no shared-theme contract is touched.

## Model (`icons.go`)

Replace the glyph-only `iconSet` with glyph+color entries:

```go
type glyph struct {
	ch    string         // includes its trailing space, as today
	color lipgloss.Color // "" = no per-type color (plain set / fallback)
}

type iconSet struct {
	folder, parent, file, action glyph
	byExt                        map[string]glyph
}

// iconFor returns the glyph and its color for an entry. Color "" means render
// the glyph uncolored (plain/ascii set).
func (s iconSet) iconFor(e fileEntry) glyph { … }   // .. → parent; dir → folder; byExt; else file
```

Keep a thin `icon(e) string` returning `iconFor(e).ch` so existing call sites/tests that only
need the glyph keep working.

### Curated nerd set (glyph + Dracula color)

| field / ext | glyph | color |
|---|---|---|
| `folder` | `` (nf-fa-folder) | `#8be9fd` cyan |
| `parent` (`..`) | `` (nf-fa-arrow_up) | `#6272a4` grey |
| `.md` / `.markdown` | `` | `#f8f8f2` fg |
| `.txt` | `` | `#f8f8f2` fg |
| `.pdf` | `` (nf-fa-file_pdf_o) | `#ff5555` red |
| `.png/.jpg/.jpeg/.gif/.webp` | `` (nf-fa-file_image_o) | `#50fa7b` green |
| `.json/.toml/.yml/.yaml/.sh` | `` | `#f1fa8c` yellow |
| `file` (generic) | `` (nf-fa-file) | `#6272a4` grey |
| `.wg` | `` | `#f8f8f2` fg |
| `action` (`+`) | `` (nf-fa-plus) | accent |

(The exact glyph bytes are transcribed verbatim in the plan; the user confirmed they render in
their terminal. Colors live as named constants in `styles.go` next to the palette.)

### plain / ascii set (unchanged behavior)

Glyphs as today (`▸ `, `↑ `, `  `, `+ `), **color `""`** for every entry → rendered
uncolored. Monochrome, exactly as now.

## Rendering (`filelist.go`, `home.go`)

The row becomes `gutter + colored-glyph + styled-name`. The glyph is colored with its type
color **only on non-selected rows**; on the **selected** row (white-on-purple
`selectedStyle`) the glyph is rendered plain so the selection's white foreground applies and
stays legible on the purple bar.

- **dir (non-selected):** folder glyph in folder-color + name in `accent` (today the whole row
  was accent; now the glyph carries the folder color, the name keeps accent).
- **section/chapter row (`sectionRow`, non-selected):** the `.md` glyph colored + title +
  right-aligned word count (unchanged).
- **loose file (non-selected):** colored glyph + stem + dim extension (unchanged behavior for
  the name).
- **selected row:** glyph plain (no type color); whole row through `selectedStyle`. Section
  rows selected keep the existing `sectionRow(e, false)` path, with the glyph plain.
- **home recents:** same `gutter + colored-glyph + label`.

Helper: a small `renderIcon(g glyph, selected bool) string` that returns
`lipgloss.NewStyle().Foreground(g.color).Render(g.ch)` when `g.color != "" && !selected`, else
`g.ch` (let the row/selection style color it). Used by both `filelist.go` and `home.go`.

## Testing

- `iconFor` returns the right glyph+color per type: folder (cyan), `.md` (fg), `.pdf` (red),
  `.png` (green), `.toml` (yellow), generic (grey), `..` (parent). `icon()` still returns the
  glyph.
- `plain` set: glyphs are ASCII and every color is `""` (→ rendered uncolored).
- Pane `View`: a non-selected `.pdf` row contains the red ANSI fg around the glyph; the
  **selected** row's glyph carries no type-color ANSI (selection styles it). A non-selected
  dir shows the folder color on the glyph and accent on the name.
- Home recents render the per-extension colored glyph (extends the existing
  `TestHomeViewUsesPerExtensionIconForRecents`).

## Out of scope

- Retuning okashi's base palette to Dracula (selection/dir colors stay as they are).
- Open/closed folder distinction (single folder glyph).
- A broad yazi-parity icon DB (curated writing set only).
- The inspector / outline / goals / spellcheck / syntax work (separate, later efforts —
  foundation-first per the roadmap).
