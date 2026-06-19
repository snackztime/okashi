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
| `tab`    | Switch focus between sidebar and editor |
| `ctrl+n` | Create a new file (type a name, Enter)  |
| `ctrl+p` | Toggle a rendered Markdown preview       |
| `ctrl+s` | Save the open file                      |
| `ctrl+c` | Quit                                    |

Inside the sidebar, arrow keys + Enter navigate folders and open a file.
`ctrl+n` opens a blank buffer in the folder you're currently browsing — give it
a name (a bare name gets `.md`) and `ctrl+s` writes it to disk.

## Markdown preview

New files default to `.md`, which is just plain text you edit directly — but
`ctrl+p` renders the current buffer as formatted Markdown (via [glamour], the
library behind `glow`): headings, **bold**, lists, the lot. It's a read-only
snapshot — `↑`/`↓` scroll, `ctrl+p` flips back to editing. The theme follows
your terminal's background (dark/light); override with `OKASHI_THEME=light`.

[glamour]: https://github.com/charmbracelet/glamour

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

- **`main.go`** — the root model. Holds the `filepicker`, the `textarea`, the
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

### Typewriter scrolling — your next build

This is the one feature the framework doesn't hand you. The `textarea` keeps its
own internal viewport pinned to the *bottom*, not the center. See
`applyTypewriterScroll` in `main.go` for the full plan. Short version: track the
caret's wrapped row, then either (a) vendor `bubbles/textarea` and add a
`SetViewportOffset` method (~20 lines, cleanest), or (b) replace the writing
pane with a custom renderer over soft-wrapped lines centered on the caret (more
code, total control — where a polished v1 ends up).

## Roadmap

1. **Typewriter scroll** (above) — the signature feel.
2. **Focus dimming** — render the non-current sentence/paragraph in a dim style.
3. **Word count / session stats** in the status line (`m.editor.Value()`).
4. **Chapter list** — swap the filepicker for a flat `list` of a project's files.
5. **Editor-core hardening** — if `bubbles/textarea` strains on long manuscripts
   (undo depth, huge files, soft-wrap edge cases), move to a rope-backed buffer.

## Rebanner

The ASCII banner in `styles.go` was generated with figlet's "small" font:

```sh
figlet -f small okashi        # paste output into bannerArt in styles.go
# (no figlet? `brew install figlet`)
```
