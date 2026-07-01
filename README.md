# okashi

A terminal writing app for long-form manuscripts and prose. Plain `.md` files, a
manuscript-aware sidebar, live word counts, RTF + PDF export, and a full-screen
distraction-free editor — all from the command line.

<!-- screenshot placeholder -->

---

## Install

### From source (Go 1.25)

```sh
git clone https://github.com/snackztime/okashi
cd okashi
go build ./...          # build the binary
go run .                # run without installing
go install .            # or install to $GOBIN
```

### Homebrew

_(Planned — not yet available.)_

---

## Quick start

```sh
okashi              # open the writing app
okashi --version    # print the version
okashi --help       # show help
```

okashi opens in your writing folder (see [Configuration](#configuration) for
where that is). The sidebar shows your documents and projects on the left; the
editor is centered on the right. Collapse the sidebar with `ctrl+b` for a
full-screen writing surface.

---

## Keyboard shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `ctrl+b` | Toggle sidebar |
| `ctrl+y` | Inspector tabs |
| `ctrl+l` | Outline |
| `ctrl+k` | Binder |
| `ctrl+o` | Home (launch screen) |
| `esc` | Switch focus / back |

### Files

| Key | Action |
|-----|--------|
| `ctrl+n` | New file (`+` new, right-click / F2 rename) |
| `r` | Rename file |
| `M` | Move file or folder |
| `del` | Delete file |
| `d` | Duplicate file |

### Writing

| Key | Action |
|-----|--------|
| `ctrl+s` | Save |
| `ctrl+t` | Typewriter scrolling (caret stays centered) |
| `ctrl+d` | Focus dim (dim everything outside the current sentence) |
| `ctrl+g` | Set goals |
| `ctrl+r` | Spelling suggestions |
| `ctrl+c` | Quit |

### Export & preview

| Key | Action |
|-----|--------|
| `ctrl+e` | Export (RTF + PDF) |
| `ctrl+p` | Markdown preview |
| `t` | Toggle Tufte view (inside preview) |

### Search

| Key | Action |
|-----|--------|
| `ctrl+f` | Search (Tab to scope · ctrl+a all sources) |

---

## Project model

The atom is one `.md` file. Larger structures are plain folders:

- **Manuscript** — a folder containing a `manifest.json`. The manifest is the
  sole source of order and display titles. Files listed in `items` are chapters;
  unlisted `.md` files are Resources (visible but not part of the ordered view or
  export). `ctrl+e` exports the whole manuscript.
- **Category** — a plain folder of unnumbered documents (no manifest). Good for
  loose notes, research, or reference material.
- **Resources** — `.md` files inside a manuscript folder that are not listed in
  `items`, or unnumbered files at the root or in a category.
- **Legacy manuscripts** — a folder with no manifest but at least one
  numerically-prefixed file (e.g. `01-opening.md`) is recognized for display
  only: order by numeric prefix, titles de-slugged from filenames. This is a
  read-only transitional view; no structural writes are offered.

The **structure mode** (`s` from the binder) lets you reorder, insert, and
remove chapters in a manifest manuscript. Changes are staged and applied behind
a single confirmation.

---

## Export

Press `ctrl+e` to export. Choose a style:

| Key | Style | Description |
|-----|-------|-------------|
| `m` | Manuscript | Courier, double-spaced — the standard agent/editor submission format |
| `t` | Tufte | Elegant serif, for a readable or printable copy |

Both styles produce a `.rtf` and a `.pdf`, written to `<project>/export/`.
When invoked from the outline, the full manuscript is exported (all chapters
concatenated). When invoked from the editor, only the current document is
exported.

Set `OKASHI_AUTHOR` to include your name in the Manuscript running header.

---

## Preview

`ctrl+p` opens a rendered Markdown preview of the current document (powered by
[glamour](https://github.com/charmbracelet/glamour)). The preview is read-only;
`↑`/`↓` scroll, `ctrl+p` returns to editing.

Inside the preview, press `t` to toggle **Tufte view** — a book-style layout
that floats footnotes into **margin sidenotes** on wide terminals (≥ 90 columns).
On narrower terminals, footnotes fall back to endnotes at the bottom of the page.

The preview theme follows your terminal background (dark or light). Override
with `OKASHI_THEME=dark` or `OKASHI_THEME=light`.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `OKASHI_DIR` | _(see below)_ | Override the writing folder — set this to point okashi anywhere |
| `OKASHI_WIDTH` | `72` | Editor column width, 20–200 |
| `OKASHI_SMARTQUOTES` | `on` | Smart curly quotes as you type; set `off`, `false`, or `0` to disable |
| `OKASHI_THEME` | _(auto)_ | Force `dark` or `light` for the Markdown preview |
| `OKASHI_ICONS` | _(auto)_ | Glyph set: `nerd` (Nerd Font glyphs), `plain` (Unicode only), or unset for auto-detect |
| `OKASHI_AUTHOR` | _(none)_ | Author name for the Manuscript export running header |

### Writing folder

okashi opens in a writing folder resolved in this order:

1. `$OKASHI_DIR` — set this to point okashi anywhere you like.
2. iCloud Drive — `~/Library/Mobile Documents/com~apple~CloudDocs/okashi`, when iCloud Drive is enabled.
3. `~/Documents/okashi` — cross-platform fallback (iCloud off, or Linux).

The folder is created on first run.

---

## Text selection

okashi enables mouse reporting so the scroll wheel and click-to-focus work.
This suppresses the terminal's native drag-to-select. To select text:

- **iTerm2 / Ghostty / Terminal.app** — hold **⌥ Option** and drag.
- **Most other terminals** — hold **Shift** and drag.

Then **⌘C** (or your terminal's copy key) to copy.

---

## License

MIT — see [LICENSE](LICENSE).

Copyright (c) 2026 Michael Pentz.
