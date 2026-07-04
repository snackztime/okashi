# Project Properties — design spec

**Date:** 2026-07-04
**Status:** Approved design (decisions confirmed 2026-07-04). Feeds a just-in-time implementation
plan. Slots into the staged roadmap **next, before Stage 7 (Snapshots)**, and **absorbs Stage 8b**
(per-project settings).

## Goal

Replace env-only configuration with an in-app, editable **Properties** screen for project and
personal metadata, feeding both the Manuscript title page (Stage 5) and the live editor. okashi's
first *editable* surface (the inspector is read-only stats today).

## Non-goals

- No manifest **schema** change (author/contact do NOT go in `manifest.json`) → the shared-contract
  HARD GATE is not triggered.
- No global app-preferences beyond the five fields below (no theme/icons editing in v1 — those stay
  env-only; YAGNI).
- No global writing-screen shortcut in v1 — the hub is the entry point (see UI). A writing-screen
  shortcut is a deferred nicety.

## Storage — three tiers, precedence high → low, per field

| Field | Source of truth (written by Properties) | Env fallback | Built-in default |
|---|---|---|---|
| Author | `~/.config/okashi/config.json` (personal, global) | `OKASHI_AUTHOR` | `""` |
| Contact | `~/.config/okashi/config.json` | `OKASHI_CONTACT` | `""` |
| Width | `<project>/.okashi.json` (per-project) | `OKASHI_WIDTH` | `72` |
| Smartquotes | `<project>/.okashi.json` | `OKASHI_SMARTQUOTES` | `true` |
| Title | `manifest.json` `title` (already okashi-owned) | — | derived folder / filename |

**Precedence rule (per field):** the file value wins **if present**; otherwise the env value wins if
set and valid; otherwise the built-in default. "Present" means the JSON key exists — so a config
file that omits `width` still honors `OKASHI_WIDTH`, and a user who sets `width` in Properties
overrides the env. This keeps every existing env-based setup working unchanged.

**Personal vs per-project rationale:** author/contact are person-level constants (your address is the
same across manuscripts) → global config, entered once. Width/smartquotes are project-level (a
screenplay and a novel want different measures) → per-project. Title already lives in the manifest.

### File formats

`~/.config/okashi/config.json` (personal; `os.UserConfigDir()/okashi/config.json`, same dir as
`recent.json`):
```json
{ "author": "Jane Ledoux", "contact": "123 Rue Ordinaire\nMontréal H2X 1Y4\njane@example.com" }
```

`<project>/.okashi.json` (per-project; a dotfile → already excluded from the file pane):
```json
{ "width": 80, "smartquotes": true }
```

Both are optional. A missing file, unreadable JSON, or an absent key falls through to the env/default
tier — never an error, never a crash (mirrors `recent.json`'s tolerant load).

## New Go surface

New file `settings.go`:

- `type userConfig struct { Author string; Contact string }` with pointer-free JSON; load is
  tolerant (missing/corrupt → zero value).
- `type projectSettings struct { Width *int; Smartquotes *bool }` — **pointers** so "unset" (nil,
  fall through to env) is distinct from "set to zero/false". Custom marshalling not needed; `omitempty`
  on pointers already omits nil.
- `func userConfigPath() string` → `os.UserConfigDir()/okashi/config.json` (`""` if no config dir).
- `func loadUserConfig(path string) userConfig`
- `func saveUserConfig(path string, c userConfig) error` — `atomicWrite`, `MkdirAll` the dir.
- `func projectSettingsPath(dir string) string` → `dir/.okashi.json`.
- `func loadProjectSettings(dir string) projectSettings`
- `func saveProjectSettings(dir string, s projectSettings) error` — `atomicWrite`.
- `type effectiveSettings struct { Author, Contact string; Width int; Smartquotes bool }`
- `func resolveSettings(dir string) effectiveSettings` — overlays defaults ← env ← file, per the
  precedence rule. Uses the existing `resolveColumnWidth`/`resolveSmartQuotes` for the env tier
  (refactored to `resolveColumnWidthEnv() (int, bool)` / `resolveSmartQuotesEnv() (bool, bool)`
  returning a `present` flag so the file tier can override cleanly), and reads `OKASHI_AUTHOR`/
  `OKASHI_CONTACT` for the identity tier.

## Integration points

1. **Startup (`initialModel`)** — replace the direct `resolveColumnWidth()` / `resolveSmartQuotes()`
   calls with `resolveSettings(startupDir)` for the initial project; seed `m.colWidth` and
   `m.smartQuotes` from it.
2. **Project switch** — wherever okashi changes `m.files.dir` (opening a project from the hub),
   re-call `resolveSettings(newDir)` and apply: `m.colWidth = eff.Width`, `m.smartQuotes =
   eff.Smartquotes`, then re-run the existing width-application path (`m.editor.SetWidth`, preview
   width) so the measure updates live on switch.
3. **Title page (`export.go`)** — replace `os.Getenv("OKASHI_AUTHOR")` / `os.Getenv("OKASHI_CONTACT")`
   with `resolveSettings(dir).Author/.Contact` so Properties-edited identity flows to the export.
   (Env still works via the resolver's fallback tier.)
4. **Properties save** — writes only the stores whose fields changed: `config.json` (author/contact),
   `.okashi.json` (width/smartquotes), `manifest.json` title (manifest projects only, via the
   existing `writeManifest` read-modify-write). After save, apply width/smartquotes to the live
   editor and refresh the sidebar title.

## UI — dedicated Properties screen

New `screenProperties` + `propertiesModel` in `properties.go`.

**Entry:** from the **hub library**, press `i` (info/properties) on a selected project or folder
(alongside the existing `p` pin / `d` remove / `a` add-source hints). Opens Properties for that
directory. The hint row gains `i properties`.

**Fields** (top → bottom), each a labeled row:
1. **Title** — single-line `textinput`. Editable only when the dir is a manifest manuscript; for a
   legacy/category folder it renders read-only (dimmed, derived name) and is skipped in the tab order.
2. **Author** — single-line `textinput` (global).
3. **Contact** — multiline **vendored `internal/textarea`** (small, ~4 rows), for a stacked address.
4. **Width** — single-line `textinput`, numeric; invalid/out-of-range [20,200] on commit reverts to
   the prior value with a status note.
5. **Smartquotes** — a toggle row (`on`/`off`), flipped with `space`/`⏎`.

**Navigation & editing:**
- `↑`/`↓` or `tab`/`shift+tab` move the selection between editable fields (skipping a read-only Title).
- `⏎` focuses the selected field for editing (textinput/textarea receives keys); `⏎` again (or `esc`)
  on a single-line field commits and returns to navigation; in the multiline contact field, `esc`
  commits (so `⏎` can insert newlines). The smartquotes row toggles directly on `space`/`⏎`.
- `ctrl+s` saves all changed stores and shows a confirmation status; stays on the screen.
- `esc` (in navigation mode) exits to the hub. If there are unsaved edits, it prompts
  `unsaved changes — s save · d discard · esc cancel` (mirrors okashi's existing confirm idiom).

**Layout** follows the pager/outline/snapshot screen chrome: a `── properties · <title> ──` header,
the field list, and a footer hint line (`⇥ field · ⏎ edit · ctrl+s save · esc back`).

**Model fields:** `propertiesModel{ dir string; isManuscript bool; titleInput, authorInput,
widthInput textinput.Model; contactArea textarea.Model; smartquotes bool; focus int; editing bool;
dirty bool; confirmExit bool; orig effectiveSettings+title }`. `orig` snapshots the loaded values so
`dirty` and per-store save decisions compare against it.

## Tests

`settings_test.go`:
- `resolveSettings` precedence: file overrides env overrides default, per field; a file missing a key
  falls through to env; corrupt/missing files → env/default, no error.
- `projectSettings` pointer semantics: `width:0` in the file is honored as 0 (not treated as unset)
  — proves the `*int` distinction (though 0 is out of the valid range, the resolver's own clamp
  handles validity separately; the test asserts the JSON round-trips a present-zero distinctly from
  nil).
- `loadUserConfig`/`saveUserConfig` and `loadProjectSettings`/`saveProjectSettings` round-trip;
  atomic write leaves no `.`-temp file behind.
- Title-page identity now flows from config: a `config.json` author with no env set reaches
  `writeRTF` output (wire-level check via `resolveSettings`).

`properties_test.go`:
- Save writes only changed stores (edit width only → `.okashi.json` written, `config.json` untouched).
- Read-only Title for a non-manifest dir is skipped in the tab order.
- Width commit rejects out-of-range and reverts.
- `dirty` tracking: editing then reverting a field to its original clears `dirty`.

## Build order (tasks, for the plan)

1. `settings.go` stores + `resolveSettings` + env-tier refactor (with `present` flags) + tests.
2. Wire `resolveSettings` into `initialModel` + project-switch + `export.go` title page + tests.
3. `properties.go` screen + `propertiesModel` (fields, navigation, edit, save, exit-confirm) + tests.
4. Hub `i` entry + hint row + screen dispatch wiring.
5. Docs: README Configuration section (Properties screen; env vars now "defaults, editable in-app");
   CLAUDE.md env-knobs + shipped-features line; help text (`i` in the hub).

## Roadmap note

After this ships, resume **Stage 7 (Snapshots)** then **Stage 8** (Tier-3 bundle) with **8b removed**
(absorbed here). Update `docs/superpowers/specs/2026-07-04-staged-specs-docx-tier2-3.md` §8b to point
here.
