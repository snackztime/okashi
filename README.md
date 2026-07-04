# okashi

A terminal writing app for long-form manuscripts and prose. Plain `.md` files, a
manuscript-aware sidebar, live word counts, RTF, PDF & DOCX export, and a full-screen
distraction-free editor — all from the command line.

<!-- Generate with `vhs demo.tape` (see demo.tape). -->
![okashi — opening a chapter and toggling the Tufte preview](docs/demo.gif)

---

## Install

### Homebrew (macOS & Linux)

```sh
brew install snackztime/okashi/okashi
```

### Prebuilt binary

Download the archive for your OS/architecture from the
[Releases](https://github.com/snackztime/okashi/releases) page, extract it, and put
`okashi` on your `PATH`:

```sh
tar -xzf okashi_*_darwin_arm64.tar.gz
sudo mv okashi /usr/local/bin/
```

### From source (Go 1.25)

```sh
git clone https://github.com/snackztime/okashi
cd okashi
go build -o okashi .     # build the binary
go run .                # or run without installing
```

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

**Saving:** okashi autosaves as you write and shows a save indicator in the
status bar. Press `ctrl+s` to save explicitly at any time.

**Snapshots:** every file keeps a ring of timestamped backups in a `.okashi-bak/`
folder beside it. Select a file in the sidebar and press `b` to browse them —
preview any snapshot, take one on demand with `n`, or restore one with `⏎`
(your current version is backed up first, so a restore is never destructive).

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
| `ctrl+c` | Quit |

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

### Export & preview

| Key | Action |
|-----|--------|
| `ctrl+e` | Export (RTF · PDF · DOCX) |
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

**No lock-in.** Your work is just Markdown files in ordinary folders, with a
small human-readable `manifest.json` for order and titles — no database, no
proprietary bundle. Everything is `grep`-able, diff-able, and git-friendly, and
reads perfectly well in any other editor. okashi writes files atomically
(temp-file + rename) so a crash or a synced-folder conflict can't corrupt them.
Stop using okashi tomorrow and your manuscript is exactly where you left it.

---

## Export

Press `ctrl+e` to export. Choose a style:

| Key | Style | Description |
|-----|-------|-------------|
| `m` | Manuscript | Double-spaced manuscript format for agents/editors (submit the `.docx`) |
| `t` | Tufte | Elegant serif, for a readable or printable copy |

Both styles produce a `.rtf`, a `.pdf`, and a `.docx`, written to `<project>/export/`.
(`.docx` is what most agents and editors ask for.)
When invoked from the outline, the full manuscript is exported (all chapters
concatenated). When invoked from the editor, only the current document is
exported.

A whole-manuscript Manuscript-style export opens with a standard title page:
your name and contact block (`OKASHI_AUTHOR` / `OKASHI_CONTACT`) top-left, an
approximate word count top-right, and the title centered below. Set
`OKASHI_AUTHOR` to also stamp your name into the running header. Single-chapter
exports skip the title page.

---

## Preview

`ctrl+p` opens a rendered Markdown preview of the current document (powered by
[glamour](https://github.com/charmbracelet/glamour)). The preview is read-only;
`↑`/`↓` scroll, `ctrl+p` returns to editing.

Inside the preview, press `t` to toggle **Tufte view** — a book-style layout
that floats footnotes into **margin sidenotes** when the terminal is wide enough
to hold the text plus a right margin. The body stays at your writing measure; the
preview pane widens to accommodate the notes. On a narrower terminal, footnotes
fall back to numbered endnotes.

The preview theme follows your terminal background (dark or light). Override
with `OKASHI_THEME=dark` or `OKASHI_THEME=light`.

---

## Configuration

### Properties (in-app)

Press `i` on a project in the launch hub to open **Properties** — an editable screen for the
things you'd otherwise set via env vars:

- **Title** — the manuscript display title (written to `manifest.json`; manuscripts only).
- **Author** and **Contact** — your name and a multi-line contact block for the export title page,
  saved to a personal `config.json` in your OS config dir (macOS `~/Library/Application
  Support/okashi/`, Linux `~/.config/okashi/`) — set once, applies to every project.
- **Width** and **Smart quotes** — per-project editor preferences, saved to `<project>/.okashi.json`.

`⇥` moves between fields, `⏎` edits, `space` toggles, `ctrl+s` saves, `esc` backs out. Both stores
are plain JSON you can read or edit by hand.

### Environment variables

Each variable below is a **default** that the matching Properties field overrides when set. An
env-only setup keeps working unchanged.

| Variable | Default | Description |
|----------|---------|-------------|
| `OKASHI_DIR` | _(see below)_ | Override the writing folder — set this to point okashi anywhere |
| `OKASHI_WIDTH` | `72` | Editor column width, 20–200 (per-project override in Properties) |
| `OKASHI_SMARTQUOTES` | `on` | Smart curly quotes as you type; set `off`, `false`, or `0` to disable |
| `OKASHI_THEME` | _(auto)_ | Force `dark` or `light` for the Markdown preview |
| `OKASHI_ICONS` | _(auto)_ | Glyph set: `nerd` (Nerd Font glyphs), `plain` (Unicode only), or unset for auto-detect |
| `OKASHI_AUTHOR` | _(none)_ | Author name for the Manuscript running header + title page (editable in Properties) |
| `OKASHI_CONTACT` | _(none)_ | Free-text contact block for the Manuscript title page (editable in Properties) |

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
