# Project Properties Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** An in-app editable Properties screen for project + personal metadata, replacing env-only
config and feeding the title page and live editor.

**Architecture:** Three storage tiers resolved per field (file → env → default): personal
`~/.config/okashi/config.json` (author/contact), per-project `<dir>/.okashi.json` (width/smartquotes),
and `manifest.json` title (already okashi-owned). A `resolveSettings(dir)` overlay feeds startup, the
hub→writing transition, and the export title page. A dedicated `screenProperties` edits and saves.

**Tech Stack:** Go 1.25, Bubble Tea, `bubbles/textinput`, the vendored `internal/textarea`,
`atomicWrite`.

## Global Constraints

- Reference spec: `docs/superpowers/specs/2026-07-04-project-properties-design.md`.
- **No `manifest.json` schema change** (author/contact never go in the manifest) — the shared-contract
  HARD GATE stays untriggered.
- All writes atomic (`atomicWrite`); config/settings loads are tolerant (missing/corrupt → env/default,
  never an error), mirroring `recent.json`.
- Per-field precedence: file value wins **if the JSON key is present**, else env if set/valid, else the
  built-in default (width 72, smartquotes on). Env-only setups must keep working unchanged.
- Width valid range [20,200]; out-of-range on commit reverts.
- `go build ./...`, `go vet ./...`, `go test ./...` green after every task. `go` is `/opt/homebrew/bin/go`.

---

### Task 1: Settings stores + resolver

**Files:** Create `settings.go`, `settings_test.go`. Modify `main.go:70-88` (env resolvers → add
`present` flag variants).

**Interfaces produced:**
- `type userConfig struct { Author, Contact string }`
- `type projectSettings struct { Width *int `json:"width,omitempty"`; Smartquotes *bool `json:"smartquotes,omitempty"` }`
- `type effectiveSettings struct { Author, Contact string; Width int; Smartquotes bool }`
- `userConfigPath() string`, `loadUserConfig(path) userConfig`, `saveUserConfig(path, userConfig) error`
- `projectSettingsPath(dir) string`, `loadProjectSettings(dir) projectSettings`,
  `saveProjectSettings(dir, projectSettings) error`
- `resolveSettings(dir) effectiveSettings`
- Refactor: `resolveColumnWidthEnv() (int, bool)` and `resolveSmartQuotesEnv() (bool, bool)` (value +
  present), with `resolveColumnWidth()`/`resolveSmartQuotes()` kept as thin wrappers returning just the
  value (so existing callers compile) OR replaced at their two call sites — pick the smaller diff.

- [ ] **Step 1 — tests:** precedence (file>env>default per field; missing key falls through; corrupt
  file → env/default); pointer semantics (`smartquotes:false` present ≠ nil); round-trip
  save/load for both stores; atomic write leaves no dot-temp.
- [ ] **Step 2 — verify fail** (`/opt/homebrew/bin/go test -run Settings ./...`).
- [ ] **Step 3 — implement** `settings.go` + env-resolver refactor.
- [ ] **Step 4 — verify pass**, `go vet`, full `go test ./...`.
- [ ] **Step 5 — commit.**

### Task 2: Wire resolver into startup, project switch, and the title page

**Files:** Modify `main.go` (initialModel:334-335; add `applyProjectSettings` helper + width
re-apply), `home.go` (the 6 hub→`screenWriting` transitions: 201, 208, 1211, 1241, 1251, 1269),
`export.go:45` (title-page identity via `resolveSettings`). Test: extend `export_titlepage_test.go`
or add to `settings_test.go`.

**Interfaces consumed:** `resolveSettings` (Task 1).
**Interfaces produced:** `func (m *model) applyProjectSettings()` — `eff := resolveSettings(m.files.dir)`;
sets `m.colWidth`, `m.smartQuotes`, and re-applies width to the editor/preview (reuse the existing
`SetWidth` path, e.g. by invoking the same code layout() runs, or `m.editor.SetWidth(m.colWidth)` +
`m.preview.Width`). Called after each hub `SetDir(...)`+`screenWriting`.

- [ ] **Step 1 — test:** a `config.json` author (no `OKASHI_AUTHOR` env) reaches `writeRTF` title-page
  output via `resolveSettings`; startup honors `.okashi.json` width over the default.
- [ ] **Step 2 — verify fail.**
- [ ] **Step 3 — implement:** initialModel uses `resolveSettings(writingDir())`; add helper; call it at
  the 6 transitions; export.go reads `resolveSettings(dir).Author/.Contact`.
- [ ] **Step 4 — verify pass**, vet, full test.
- [ ] **Step 5 — commit.**

### Task 3: Properties screen + model

**Files:** Create `properties.go`, `properties_test.go`. Modify `main.go` (add `screenProperties`
const; model field `properties propertiesModel`; dispatch `Update`/`View` for the screen).

**Interfaces produced:**
- `type propertiesModel struct { dir string; isManuscript bool; titleInput, authorInput, widthInput
  textinput.Model; contactArea textarea.Model; smartquotes bool; focus int; editing bool; dirty,
  confirmExit bool; orig effectiveSettings; origTitle string }`
- `func newPropertiesModel(dir string) propertiesModel` — loads via `resolveSettings` + manifest title;
  seeds inputs; `isManuscript = hasManifest(dir)`.
- `func (p propertiesModel) update(msg tea.Msg) (propertiesModel, tea.Cmd)` — nav (↑/↓/tab), `⏎` to
  edit/commit, smartquotes toggle, `ctrl+s` save, `esc`/confirm-exit; recomputes `dirty` vs `orig`.
- `func (p propertiesModel) view(width, height int) string` — header, field rows (read-only dimmed
  Title for non-manuscript, skipped in tab order), footer hints.
- `func (p *propertiesModel) save() error` — writes only changed stores: `saveUserConfig` (author/
  contact), `saveProjectSettings` (width/smartquotes), `writeManifest` title (manuscript only, RMW).
  Returns a fields-changed summary for the caller to apply live + status.

- [ ] **Step 1 — tests:** save writes only changed stores (edit width only → `.okashi.json` written,
  `config.json` untouched); non-manifest Title read-only + skipped in tab order; width commit rejects
  out-of-range and reverts; edit-then-revert clears `dirty`.
- [ ] **Step 2 — verify fail.**
- [ ] **Step 3 — implement** `properties.go` + screen const/field/dispatch in main.go.
- [ ] **Step 4 — verify pass**, vet, full test.
- [ ] **Step 5 — commit.**

### Task 4: Hub `i` entry + apply-on-save wiring

**Files:** Modify `home.go` (library key handler: `case "i"` opens Properties for the selected
project/folder → `m.properties = newPropertiesModel(path); m.screen = screenProperties`; add
`i properties` to the hint row). Modify `main.go` (on Properties save/exit, if width/smartquotes
changed for the *current* project, apply live; return to hub on `esc`).

- [ ] **Step 1 — test:** opening Properties from a selected library item constructs the model for that
  dir (unit-level on the handler helper if practical; else a focused model test).
- [ ] **Step 2 — verify fail** (or reasoned skip if purely wiring — note it).
- [ ] **Step 3 — implement** entry + hint + save-apply.
- [ ] **Step 4 — verify pass**, vet, full test; manual smoke via `go run .` optional.
- [ ] **Step 5 — commit.**

### Task 5: Docs

**Files:** Modify `README.md` (Configuration: Properties screen; env vars reframed as "defaults,
editable in-app"; `i` in the hub), `CLAUDE.md` (shipped-features + env-knobs line note that
author/contact/width/smartquotes are Properties-editable; `.okashi.json` + `config.json` mentioned),
`main.go` help text if a hub hint belongs there.

- [ ] **Step 1 — edit docs.**
- [ ] **Step 2 — build (docs-only, sanity) + commit.**

---

## Self-review notes
- Spec coverage: Tasks 1–5 map to spec §Storage/§New Go surface (T1), §Integration (T2), §UI (T3/T4),
  §Tests (T1/T3), docs. ✓
- Type consistency: `effectiveSettings`, `projectSettings` (pointer fields), `propertiesModel` names
  used identically across tasks. ✓
- Precedence + no-schema-change constraints restated in Global Constraints. ✓
