# CLAUDE.md — okashi

Terminal writing app for long-form manuscripts and prose. Go + Bubble Tea (Elm-style
Model/Update/View) + lipgloss. Two real roles:

1. **A shipped product** — the CLI/agent face of a plain-`.md` writing corpus.
2. **The scope oracle** for the sibling macOS app (`../inkmere`). The app matches okashi's
   feature set; okashi defines what "lean" means. The shared design + contracts reference
   lives in that repo's `SPEC.md`.

> **Reconciled 2026-06-26** against the shipped okashi. An earlier draft of this file
> described an aspirational architecture (gap-buffer editor, a manifest ordering file) that
> the codebase does not use; this version describes what okashi actually is, keeps the
> genuinely-good invariants, and flags the two adopted-but-not-yet-implemented rules and the
> one open cross-app contract.

---

## Architecture invariants — DO NOT violate or "improve away"

### TUI stack
- Bubble Tea architecture: `Model` / `Update` / `View`. Side effects via `tea.Cmd`, never
  inline in `Update`. lipgloss for styling; `bubbles` for stock components.
- **The editor is a VENDORED `internal/textarea`** — a fork of `bubbles/textarea` that
  okashi owns (typewriter scrolling, focus dimming, configurable measure width, smart curly
  quotes, `MoveToLine`, …). Editor-core changes go in `internal/textarea`, not by swapping
  in stock `bubbles/textarea`.

### Performance model — how okashi stays instant on a 400-page work
The strategy is **split-into-files + windowed rendering**, NOT one giant buffer:
- A manuscript is many per-chapter `NN-title.md` files. **The editor loads ONE file at a
  time** (`loadFile` → `editor.SetValue`), so it never holds a 400-page buffer — each
  editing surface is chapter-sized.
- **`View()` MUST stay O(visible).** Render only the visible window. The read-through
  **manuscript pager** (`pager.go`) builds its wrapped line→source map **once** on open and
  renders only `lines[offset:offset+height]`; the file pane and outline do likewise. Never
  stringify a whole document every frame — Bubble Tea diffs the entire `View()` output, so a
  giant string is the performance killer.
- A true gap-buffer editor core is a possible **future** optimization only if single-file
  editing ever needs very large files; it is **not** needed for okashi's chapter-sized model
  and is not currently built.
- Keep lipgloss **out of the hot path** — pre-style static chrome; don't restyle per cell
  per frame.

### Files & sync — okashi's signature obligation
- **RULE (ADOPTED): write atomically (temp file + rename**, atomic on the same volume);
  never write in place. *Status:* `save()`, export, the backup copies, the new-section file,
  and the recents store all write atomically via `atomicWrite` (`atomicwrite.go`). okashi
  runs outside the macOS sandbox and **cannot use `NSFileCoordinator`** (the mechanism the app
  uses to coordinate with the iCloud daemon), so atomic writes + iCloud `NSFileVersion` are
  what keep the shared corpus from corrupting.
- Backups: destructive structural ops (outline reorder, new-section insert) snapshot the
  affected files into `<project>/.backup/<timestamp>/` first (`backup.go`). `.backup/` and
  all dotfiles are excluded from the pane and from manuscript detection.

### Theming
- lipgloss styling; the theme is detected once at startup (override with `OKASHI_THEME`),
  rendered as truecolor where the terminal supports it (degrades to 256). ⚠️ Truecolor
  through tmux needs `RGB`/`Tc` enabled.
- A **shared semantic-role theme JSON** with the app (see Shared Contracts §3) is
  aspirational — okashi does not read a shared theme file today.

---

## Project model (the shipped reality)

- **The atom is one `.md` file.**
- **Manuscript** = a folder containing **≥1 numerically-prefixed file** (`01-opening.md`).
  Auto-detected (`isManuscript`). Order = the **leading run of digits parsed as an integer**
  (`1`=`01`=`001`; `2` before `10`); display titles are the filename **de-slugged**
  (`02-the-letter.md` → "the letter"). Files keep their real names.
- **Category** = a plain folder of unnumbered docs. **Loose / "Resources"** = unnumbered
  files (at the root, in a category, or sitting inside a manuscript as notes); inside a
  manuscript they're shown but **excluded** from the ordered manuscript and from export.
- **There is no manifest file.** Membership + order + titles come from the filename
  convention above. (This diverges from the earlier draft — see Shared Contracts §1.)
- Shipped features: the launch hub; a manuscript-aware sidebar (titles + per-chapter word
  counts); the **outline** (`ctrl+l`: select/open, `J/K` reorder with renumber-on-disk +
  backup, `n` new section, `r` rename, `m` → pager); **convert** (`ctrl+l` on a plain chapter
  folder offers to number it into a manuscript); the **pager** (`m`: read-through with
  jump-to-edit); **export** (`ctrl+e`: RTF + PDF, Manuscript or Tufte style); **rename**
  (`r`); markdown **preview** (`ctrl+p`, glamour).
- Env knobs: `OKASHI_DIR`, `OKASHI_WIDTH`, `OKASHI_SMARTQUOTES`, `OKASHI_THEME`,
  `OKASHI_ICONS`, `OKASHI_AUTHOR` (export header).

### Scope discipline
- **okashi defines the lean feature set — keep it lean.** New surface area here propagates to
  the app. When a change would expand scope, surface it and ask.
- No tag system / smart folders / PKM. No sub-document "sheets" (granularity = how files are
  split).

---

## ⚠️ SHARED CONTRACTS — MIRROR THIS BLOCK IN `../inkmere`

okashi and the macOS app operate the **same on-disk corpus**. Keep this block aligned across
both repos.

### 1. Manuscript ordering & membership — HARD GATE + OPEN CROSS-APP ITEM
- okashi's contract is the **filename convention**: numeric prefix = order, de-slugged
  filename = title, unnumbered = loose/Resources (excluded from order/export). **No manifest.**
- ⚠️ **OPEN (deferred to inkmere):** `../inkmere` is **not built yet**. When it is, it will
  likely introduce a **manifest** at project creation; that ordering decision is **dictated
  from there** and then brought back here to reconcile. Until inkmere lands and the choice is
  made, okashi is authoritative on **filename-prefix**. If inkmere adopts a manifest, pick ONE
  (filename-prefix or manifest) and implement it in **both** codebases together — do not change
  okashi's convention unilaterally.
- HARD GATE: before changing the on-disk **ordering/membership convention** (prefix format,
  zero-pad rules, loose-file semantics), STOP, confirm with the user, and implement in both
  apps together. Routine data writes — reorder, add/remove a file, rename a title — are normal
  ops and proceed without prompting.

### 2. Markdown flavor — HARD GATE (ADOPTED)
- Flavor = **CommonMark + GFM (tables, task lists, strikethrough, autolinks) + footnotes**,
  via `goldmark` with the matching extensions. **Footnotes must be enabled** to match the
  app's `swift-markdown`/cmark-gfm config.
- *Status:* the **export** parser (`export_ast.go`) uses `goldmark` + **GFM + Footnote**
  extensions (matching the app); footnotes export as per-chapter endnotes; the rarer GFM
  constructs degrade (tables → pipe rows, strikethrough → plain, task lists → `[ ]`/`[x]`).
  The `ctrl+p` **preview** uses glamour, which renders GFM but exposes no hook to add the
  footnote extension — so footnote syntax shows literally in the preview (known limitation).
- Before changing the supported flavor/extension set: STOP, confirm, implement in BOTH
  codebases together. No proprietary syntax. Pin `goldmark` + extension versions.

### 3. Theme JSON — LIGHT (cosmetic, no gate)
- Themes are **semantic-role JSON** (`background, foreground, heading, emphasis, code, link,
  selection, gutter`), read by both apps. *Aspirational for okashi* until it reads a shared
  theme file; today it detects a theme itself. If adopted, keep the role set in sync (adding a
  role updates both apps and all theme files together).

*(End of mirrored block.)*

---

## Build & test

- Module: `okashi` (flat `package main` at the repo root + `internal/textarea`). Go `1.25`
  (raised by the pure-Go PDF dep). **On this dev machine `go` is not on PATH — invoke it as
  `/opt/homebrew/bin/go`** (and `gofmt` likewise).
- Build: `go build ./...`  ·  Run: `go run .`  ·  Test: `go test ./...`  ·  Vet: `go vet ./...`
- Key deps (all pure Go): `bubbletea`, `bubbles`, `lipgloss`, `glamour`, `x/ansi`,
  `yuin/goldmark` (parsing — shared-contract governed, pin it), `codeberg.org/go-pdf/fpdf`
  (PDF export — the maintained fork, NOT the archived `github.com/go-pdf/fpdf`), `x/text`
  (cp1252 transcode for the manuscript PDF). ET Book TTFs are embedded under `assets/etbook/`
  for the Tufte PDF.
- Design/plan history lives under `docs/superpowers/{specs,plans}/`.

---

## Working agreement

- okashi is **lean by design** and is the oracle the app matches — bias toward restraint; ask
  before expanding scope.
- When a task would touch a **shared contract** (§1–§3) or the **atomic-write rule**, surface
  it and confirm rather than proceeding.
- The shared design reference is the app repo's `SPEC.md`; this file is okashi's operational
  rule set.
- Adopted & shipped: **atomic writes** (pending earlier, now in `save()` + export) and **GFM +
  footnotes** in the export parser (shipped with Tasks 1–3 of the 2026-06-22 export refactor).
