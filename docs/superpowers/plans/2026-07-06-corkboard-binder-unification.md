# Corkboard / Binder Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the corkboard the one manuscript navigator — a synopsis+chapter view shown compactly
in the left pane (toggle) and expanded full-screen — with staged reorder, first-line synopsis
fallback, resources as folder-groups, and a `ctrl+n` chapter|resource prompt; retire the pop-down
binder and standalone structure modal.

**Architecture:** The left file pane (`filelist.go`) gains a list↔corkboard mode. The corkboard
(pane + full-screen `corkboard.go`) reuses the existing staged `structure*` buffer + `commitStructure`
for reorder/add/remove behind one confirm; synopsis edits write `.okashi-synopsis.json` immediately.
The pop-down binder (`screenOutline`) and standalone structure modal (`screenStructure`) are removed;
their entry points move to the sidebar/pane.

**Tech Stack:** Go 1.25, Bubble Tea (Model/Update/View), lipgloss, the vendored `internal/textarea`.
`go` is `/opt/homebrew/bin/go`.

## Global Constraints

- Reference spec: `docs/superpowers/specs/2026-07-06-corkboard-binder-unification-design.md`.
- **Corkboard IS the synopsis view** — one concept, two densities (pane + full-screen). No separate
  "synopsis mode"; synopses live only in the corkboard.
- **Reorder/add/remove/retitle are STAGED + committed behind one confirm** ("apply changes? y apply ·
  esc discard"), reusing the existing `structure*` buffer + `commitStructure`. A lone `ctrl+n`
  chapter append writes immediately. **Synopsis edits (`e`) write the sidecar immediately.**
- **No manifest schema change; no new sidecar; no per-resource metadata; no auto-created resource
  folders/templates.** Storage: `manifest.json` + `.okashi-synopsis.json` only.
- CLAUDE.md §1 stays accurate (okashi reorders behind a confirm) — HARD GATE untriggered.
- `View()` stays O(visible) — window every list/card render (reuse `homeWindowOffset`).
- Atomic writes for all structural writes (`writeManifest`, `saveSynopses` — both already atomic).
- `go build ./...`, `go vet ./...`, `go test ./...` green after every task.

## File structure

- `filelist.go` — pane render; add corkboard-mode render (Chapters + Resources sections),
  `firstProseLine`, mode flag accessor. (Largest change.)
- `synopsis.go` — storage (built); add `firstProseLine` here or in filelist. No schema change.
- `corkboard.go` — full-screen corkboard; source dir from `m.files.dir`; add `a`/`x`/`r`; reuse
  staged buffer + `commitStructure`; keep windowed cards.
- `structure.go` — keep `structure*` buffer + `commitStructure` + `exitStructure`; delete
  `enterStructure` + the standalone `screenStructure` update/view; add `addChapterStaged` /
  `removeChapterStaged` helpers if not present (structure.go already has add/remove).
- `main.go` — model fields (`corkMode bool` pane toggle; reuse `structure*` for staged reorder from
  the pane); key handlers (`ctrl+k` toggle, `c` expand, sidebar `J`/`K`/`e`/`m`, `ctrl+n` prompt);
  retire `screenOutline`/`screenStructure` dispatch; F1 help; sidebar status.
- README.md, CLAUDE.md — docs.

---

### Task 1: First-line fallback + corkboard-mode render in the pane

**Files:** Modify `filelist.go` (View + a new corkboard render path, mode flag), `synopsis.go` (add
`firstProseLine`). Test: `filelist_corkboard_test.go` (new).

**Interfaces produced:**
- `func firstProseLine(path string) string` — first non-blank, non-`#`-heading line of a file, "" if
  none. (In `synopsis.go`.)
- `filelist` gains `corkMode bool` (false = list, true = corkboard) + a setter; `View` renders
  corkboard rows when `corkMode && f.view.ordered()`.
- Corkboard render: for each chapter, a compact block — `NN  Title            412w` then an indented
  2-line synopsis preview (`synopses[file]` or dimmed `firstProseLine`), wrapped to the inner width
  via the existing `wrapClamp` (move `wrapClamp` from `corkboard.go` to a shared spot or reuse). A
  `RESOURCES` subheader then the `view.loose` + subfolder groups.
- The pane needs the synopses map: load via `loadSynopses(f.dir)` on `SetDir` (store `f.synopses
  map[string]string` on `filelist`), so render is O(visible) without per-frame disk reads.

**Test outline (write first, watch fail, implement, pass, commit):**
- `firstProseLine`: file `"# Heading\n\nFor thirty-one winters…"` → `"For thirty-one winters…"`;
  empty/whitespace/heading-only → `""`.
- Corkboard render: a filelist with `corkMode=true` over a 3-chapter manifest + 1 loose resource +
  a `Characters/` subfolder → output contains each chapter title, its word count, its synopsis (or
  first-line fallback for a synopsis-less chapter), a `RESOURCES` heading, the loose file, and the
  `Characters/` group. Windowed: with height < needed, only the visible window renders.

### Task 2: Pane chapter actions — staged reorder + immediate synopsis edit

**Files:** Modify `main.go` (sidebar key handler: `J`/`K`/`e`; `ctrl+k` toggle; `c` expand),
`corkboard.go` or a small `pane_corkboard.go` (reorder/synopsis helpers that reuse `structure*`).
Test: extend `filelist_corkboard_test.go` / a `pane_corkboard_test.go`.

**Interfaces consumed:** `structure*` buffer + `commitStructure` (structure.go), `saveSynopses`
(synopsis.go), `firstProseLine`/corkMode (Task 1).
**Interfaces produced:**
- `ctrl+k` toggles `f.corkMode` (manuscript only; no-op + status on non-manuscript/legacy).
- Sidebar `J`/`K` on a chapter: enter/continue a staged reorder over `m.structureItems` (seeded from
  the manifest on first move), set `structureDirty`, show a `-- REORDER --` status; `esc` while dirty
  → the existing confirm (`y` apply via `commitStructure`, `esc` discard). No-op on Resource/`..`.
- Sidebar `e` on a chapter: open a synopsis popup (`textarea`), commit on `esc` → `saveSynopses(dir,
  …, chapterSet)` immediately (reuse the corkboard's `commitSynopsis` path); does not set
  `structureDirty`.
- Sidebar `c`: `m.enterCorkboard()` (full-screen).

**Test outline:**
- `ctrl+k` toggles corkMode on a manuscript; no-op (status) on a category folder.
- `J`/`K` from the pane stages a reorder (assert `structureItems` order + `structureDirty`), writes
  nothing to disk; `esc`→`y` commits (assert on-disk manifest order); `esc`→`esc` discards.
- `e` writes the sidecar immediately and does not set `structureDirty`.
- `J`/`K` no-op when the selection is a Resource or `..`.

### Task 3: Full-screen corkboard = the structural spread

**Files:** Modify `corkboard.go` (`enterCorkboard` sources `m.files.dir`; add `a`/`x`/`r` handlers on
the staged buffer; keep card view + windowing + esc-confirm), `main.go` (dispatch already exists).
Test: extend `corkboard_test.go`.

**Interfaces produced:**
- `enterCorkboard` reads the manifest from `m.files.dir` (not `m.outline.dir`), seeds `structure*`.
- `updateCorkboard` gains: `a` → add picker (new blank chapter / promote a Resource — reuse
  structure.go's `structureAdd*`), `x` → remove (demote to Resource, structure.go's remove), `r` →
  retitle (reuse `structureRenaming`), `enter` → open the selected chapter + exit, `J`/`K`/`e` as now.
- Commit on `esc` when `structureDirty` (existing confirm), else exit to the pane/writing.

**Test outline:**
- `enterCorkboard` from `m.files.dir` seeds the 3 chapters.
- `a` (new chapter) then commit → the manifest gains the chapter (file created); `x` then commit →
  chapter demoted to Resource (file still on disk, unlisted); `r` retitles; each staged then one
  commit.
- `enter` opens the selected chapter and leaves the corkboard.

### Task 4: `ctrl+n` chapter|resource prompt + resource-folder targeting

**Files:** Modify `main.go` (the create flow: in a manuscript, `ctrl+n` opens a chapter|resource
picker; resource → loose or a subfolder pick/create). Test: `create_prompt_test.go` (new).

**Interfaces produced:**
- In a manuscript, `ctrl+n` sets a small picker state (`createKind`: chapter/resource) before the
  name prompt. Chapter → create `.md` at root + append to `items` immediately (atomic
  `writeManifest`) + optional synopsis. Resource → create unlisted `.md`, loose or into a chosen/new
  subfolder (`MkdirAll` the folder).
- Outside a manuscript: unchanged create flow.

**Test outline:**
- Manuscript `ctrl+n` → chapter → the new `.md` exists and is appended to `items`; synopsis optional.
- `ctrl+n` → resource → loose → unlisted `.md` at root (not in `items`).
- `ctrl+n` → resource → folder "Characters" → `Characters/<name>.md` created; the folder made if new.
- Non-manuscript dir: `ctrl+n` behaves as before.

### Task 5: Retire the binder + standalone structure modal

**Files:** Modify `main.go` (remove `screenOutline` binder entry `ctrl+k` → now the corkMode toggle;
remove `screenStructure` standalone entry `s`; move pager to sidebar `m`; drop the binder/structure
screen dispatch if unused elsewhere), `structure.go` (delete `enterStructure` + the standalone
`updateStructure`/`structureView` if fully replaced by the corkboard; keep `commitStructure`/
`exitStructure`/staged helpers), `outline.go` (the binder screen — remove or repoint). Test: extend
existing screen tests + a retirement test.

**Interfaces produced:**
- `ctrl+k` = corkMode toggle (Task 2), no longer opens `screenOutline`.
- `m` from the sidebar opens the pager (`enterManuscript`/`screenManuscript`).
- `s` no longer enters structure mode (folded into the corkboard); the binder's `s`/`c`/`m` dispatch
  is gone.

**Test outline:**
- `ctrl+k` no longer sets `screen == screenOutline`; it toggles corkMode.
- `s` from the sidebar is a no-op / unbound (not entering structure mode).
- `m` from the sidebar opens the pager (`screen == screenManuscript`).
- Build has no references to the removed screens; `go vet` clean.

### Task 6: Docs — README, CLAUDE.md, F1 help, spec status

**Files:** Modify `main.go` (F1 `helpText` MANUSCRIPT group: `ctrl+k` corkboard toggle, `c` expand,
`J`/`K` reorder, `e` synopsis, `m` pager, `ctrl+n` chapter|resource; drop `s`/binder), `README.md`
(the manuscript navigator + corkboard + resources + ctrl+n), `CLAUDE.md` (shipped-features updated;
§1 unchanged; retired keys note), the spec `Status:` → shipped.

**Test outline:** F1 render test asserts the new MANUSCRIPT keys present and the retired ones absent;
docs build (no code). Commit.

---

## Self-review

- **Spec coverage:** Task 1 → pane corkboard render + first-line fallback + resources sections;
  Task 2 → pane reorder (staged) + synopsis (immediate) + toggles; Task 3 → full-screen structural
  spread (a/x/r); Task 4 → ctrl+n chapter|resource + resource folders; Task 5 → retire binder +
  structure modal, pager→`m`; Task 6 → docs. All spec sections covered.
- **Placeholders:** none — where exact code is deferred it names the concrete existing function to
  reuse (`commitStructure`, `saveSynopses`, `structureAdd*`, `homeWindowOffset`, `wrapClamp`).
- **Type consistency:** `corkMode bool`, `f.synopses map[string]string`, `firstProseLine(path)
  string`, reuse of `structure*`/`commitStructure` used consistently across tasks.

## Risks / review focus

- **Shared-state reuse:** the pane reorder and the full-screen corkboard both drive `structure*`;
  ensure entering one resets state (the corkboard-harden pass already reset sub-flags — extend to the
  pane path). Adversarial review this like the earlier corkboard pass.
- **Retirement dead-code:** removing `screenOutline`/`screenStructure` may leave dangling refs
  (`m.outline`, dispatch cases) — `go vet` + a grep sweep in Task 5.
