# okashi

A minimal, Crush-flavored terminal writing app. ASCII banner up top, a file
pane on the left, a centered 80-column writing surface on the right. Collapse
the sidebar and it becomes a full-screen, distraction-free page.

## Install

### Homebrew

```sh
brew install snackztime/tap/okashi
```

(Requires a `snackztime/homebrew-tap` repo containing `Formula/okashi.rb`. Until
then, install straight from this checkout: `brew install --build-from-source ./Formula/okashi.rb`.)

### From source

```sh
go mod tidy   # pulls the Charm libraries
go run .       # run in place
go install .   # or install the okashi binary to $GOBIN
```

Requires Go (see `go.mod` for the minimum). If you want the latest Charm
releases instead of the pinned ones, run `go get -u ./...` after the first
`tidy`.

## Releasing

Tag and push — the `release` workflow builds, tests, creates the GitHub Release,
and prints the `url`/`sha256` to paste into `Formula/okashi.rb`:

```sh
git tag v0.1.0 && git push origin v0.1.0
```

## Keys

| Key      | Action                                  |
|----------|-----------------------------------------|
| `ctrl+b` | Toggle the file sidebar (focus mode)    |
| `esc`    | Switch focus between sidebar and editor (exit preview) |
| `tab`    | Indent (Shift+Tab to outdent) in the editor             |
| `ctrl+n` | Create a new file (type a name, Enter)  |
| `ctrl+p` | Toggle a rendered Markdown preview       |
| `ctrl+t` | Toggle typewriter scrolling (centered caret) |
| `ctrl+d` | Toggle focus dimming (dim all but the current sentence) |
| `ctrl+o` | Back to the launch screen (recent files / projects) |
| `ctrl+s` | Save the open file                      |
| `ctrl+c` | Quit                                    |

Inside the sidebar, arrow keys + Enter navigate folders and open a file; the
mouse works too — wheel scrolls, a single click selects (and focuses the pane),
a double-click opens a file or enters a folder.
`ctrl+n` opens a blank buffer in the folder you're currently browsing — give it
a name (a bare name gets `.md`) and `ctrl+s` writes it to disk.

## Markdown preview

New files default to `.md`, which is just plain text you edit directly — but
`ctrl+p` renders the current buffer as formatted Markdown (via [glamour], the
library behind `glow`): headings, **bold**, lists, the lot. It's a read-only
snapshot — `↑`/`↓` scroll, `ctrl+p` flips back to editing. The theme follows
your terminal's background (dark/light); override with `OKASHI_THEME=light`.

[glamour]: https://github.com/charmbracelet/glamour

## Writing ergonomics

- **Tab / Shift+Tab** indent and outdent (two spaces).
- **Enter** on a Markdown list line (`- `, `* `, `+ `, `1.`) continues the list;
  Enter on an empty item ends it.
- **Smart quotes** turn `'`/`"` into curly quotes as you type (on by default;
  set `OKASHI_SMARTQUOTES=off` for code-heavy writing).
- **Column width** defaults to 65; set `OKASHI_WIDTH=<n>` (20–200) to taste.

## Focus mode

With typewriter on (`ctrl+t`), okashi also dims everything except the sentence
you're in — your current sentence stays bright and centered, the rest fades.
`ctrl+d` toggles just the dimming (keeping centered scrolling). Turning
typewriter off turns both off. (`ctrl+d` takes over the editor's delete-forward;
use Delete/Backspace instead.)

## Launch screen

okashi opens on a launch screen: your **recent files** and your **projects**
(the folders in your okashi dir), plus "Open another folder…". Pick a recent
file to jump straight in, or a project to browse it in the sidebar. Once you're
in a file the logo disappears — a full minimal writing zone. `ctrl+o` returns
to the launch screen.

From the launch screen you can open a recent file or project, **create a new
document or project**, or browse all files — by keyboard or mouse (click to
select, double-click to open). Type a name ending in `/` to make a folder.

The file pane is confined to your okashi workspace folder — a breadcrumb at the
top of the pane (`okashi / Book Name`) shows where you are, and you can't browse
above the workspace. "Browse all files" on the launch screen returns to the
workspace root.

The breadcrumb segments are clickable — click `okashi` or a parent folder to jump
there. On deep paths it shows `okashi / … / Drafts`, keeping the nearest folders
clickable. A `3/12` indicator appears when the list is taller than the pane.

## Icons

The file pane and launch lists use Nerd Font glyphs. If your terminal isn't
using a Nerd Font, set `OKASHI_ICONS=plain` for a plain-Unicode set.

## Autosave

Your work saves automatically a couple seconds after you stop typing (for any
file with a name — `ctrl+n` names it up front). The `●`/`✓` mark by the word
count shows unsaved vs saved. `ctrl+s` still saves on demand.

## Where your files live

On launch okashi opens in a writing folder, resolved in this order:

1. **`$OKASHI_DIR`** — set this to point okashi anywhere you like.
2. **iCloud Drive** — `~/Library/Mobile Documents/com~apple~CloudDocs/okashi`,
   when iCloud Drive is enabled on the Mac.
3. **`~/Documents/okashi`** — fallback when iCloud is off (or on Linux).

The folder is created on first run; you don't need to make it yourself.

## How it's wired

It's a standard Bubble Tea app — one `model` holds all state, `Update` handles
messages, `View` renders. The pieces:

- **`main.go`** — the root model. Holds the `filelist`, the (vendored) `textarea`, the
  `sidebarVisible` flag, and which pane has `focus`. `Update` intercepts the
  global keys (collapse, switch, save, quit) and otherwise forwards messages to
  whichever pane is focused. `View` builds the screen top-to-bottom: banner,
  body, status line.
- **`styles.go`** — the Lipgloss palette and the ASCII banner. This is where the
  "vibe" lives; tweak colors and borders here.

### The centered 80-column trick

The editor's content width is clamped to `columnWidth` (80). In `View`, the
rendered editor is dropped into `lipgloss.Place(...)` with `lipgloss.Center`,
which pads both sides evenly — so the text is left-justified *within* the
column, but the column floats in the middle of the available space. Collapsing
the sidebar just hands the editor the full window width to center within.

### Typewriter scrolling

The caret stays pinned to the vertical center of the writing pane (`ctrl+t`
toggles it; on by default). Bubble Tea's `textarea` only edge-anchors its
viewport, so okashi vendors it under `internal/textarea/` and adds a small
`Typewriter` patch: it pads the view with half a screen of blank rows and sets
the scroll offset to the caret's wrapped row, centering every line — including
the first and last while writing at the end of the file.

## Roadmap

1. ~~**Typewriter scroll**~~ ✅ Done — caret pinned to center; `ctrl+t` toggles.
2. **Focus dimming** — render the non-current sentence/paragraph in a dim style.
3. ~~**Word count / session stats** in the status line.~~ ✅ Done — the status bar
   shows live `N words · +N session` (net words added since the file was opened).
4. ~~**Chapter list**~~ ✅ Done — owned `filelist` replaces the filepicker
   (and adds mouse support).
5. **Editor-core hardening** — if `bubbles/textarea` strains on long manuscripts
   (undo depth, huge files, soft-wrap edge cases), move to a rope-backed buffer.

## Rebanner

The ASCII banner in `styles.go` was generated with figlet's "small" font:

```sh
figlet -f small okashi        # paste output into bannerArt in styles.go
# (no figlet? `brew install figlet`)
```
