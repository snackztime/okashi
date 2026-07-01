# okashi Library — sources + manifest creation + home Miller — design

**Date:** 2026-06-30
**Status:** Approved (direction)
**Context:** Turn okashi's single-folder home into a real **librarian surface** that mirrors
wicklight's approved *Multi-Source Library* + *Project Model* (`../inkmere/docs/superpowers/
specs/2026-06-27-{multi-source-library,project-model}-design.md`), and let okashi **create and
name** its own manuscripts (so "New Project" is real). Builds on the shipped Miller home.

---

## 0. Manifest authority — okashi is a manifest writer (with confirm) — SHARED-CONTRACT CHANGE

**okashi may write any project's `manifest.json`**, including the wicklight-shared corpus. The
safety model is the **same one okashi already uses for prose chapters** in that corpus: atomic
temp-then-rename (no corruption, ever) + iCloud `NSFileVersion` (a rare concurrent overwrite is
recoverable). A manifest is just another file in the corpus okashi already writes to.

- **No confirm (low-stakes):** **create** a new manuscript's manifest (brand-new folder — no
  other writer); **retitle** a chapter (`items[].title` — cosmetic, single-field).
- **Confirm (consequential):** **structural** edits — reorder / insert / remove / move — prompt
  a confirmation ("Move chapter 3 to the end? Rewrites manifest.json"). This mirrors wicklight's
  own confirm sheet (multi-source §6). Structural edits are the "structuring mode" ([[roadmap-
  backlog]]); create/retitle land in this cycle.
- **Concurrency discipline:** **read-modify-write** — re-read the manifest immediately before
  writing so okashi rewrites the latest on-disk state (minimizes the lost-update window). Atomic
  write. `NSFileVersion` is the recovery net for the rare simultaneous-edit race.
- **HARD-GATE (both repos):** this reverses wicklight's "okashi never writes manifests; the only
  writers are wicklight's `ManuscriptStore`" line. Reflect the new authority in **both** — okashi
  `CLAUDE.md` §1 and wicklight `SPEC.md` / multi-source §4 / project-model §4. **No wicklight code
  change expected** (it already reloads externally-changed files via its file-presenter/iCloud
  coordination) — but **verify** wicklight picks up an okashi manifest write. Schema stays v1
  (unchanged), so this is an *authority* gate, not a *schema* gate.

---

## 1. Source model (mirror wicklight §2)

The library is a **list of sources**, not one folder — exactly wicklight's model:

- **`sourcePrimary`** — today's `writingDir()` (`OKASHI_DIR`, else the iCloud `okashi` folder).
  Always present, never removable, the default home for new documents. (= wicklight `.primary`.)
- **`sourceFolder(path)`** — user-added local/cloud folders (`~/Documents/writing`, a Dropbox
  folder = a synced local dir). (= wicklight `.folder(path:)`.)

```go
type sourceKind int // sourceKindPrimary | sourceKindFolder
type source struct {
    ID   string     // stable id (primary = "primary")
    Name string     // display name (folder base, editable)
    Kind sourceKind
    Path string     // resolved root ("" for primary → writingDir())
}
```
- **`sources.json`** in okashi's config dir (`os.UserConfigDir()/okashi/sources.json`) persists
  **user-added** sources only; the primary is synthesized at load (mirrors wicklight's
  `SourceStore`; the primary is never stored). Shape mirrors wicklight's `Source` (id/name/kind)
  so the mental model matches, though each app keeps its own file.
- **Per-source isolation:** an unreachable folder source (deleted/offline) is shown with an
  error affordance and skipped — never blocks the others or launch (wicklight §8).
- **Standalone use (the TUI-only writer):** just add a folder source (or set `OKASHI_DIR`); no
  wicklight/iCloud needed. A wicklight user has the shared corpus as one source among several.

## 2. Project model (align with wicklight project-model §2)

okashi already classifies dirs (`classifyLibrary` → manuscript vs category). Keep that; align terms:
- **Manuscript** — folder with `manifest.json` (or legacy numbered files). okashi's "**Project**".
- **Category** — plain folder (no manifest); nests freely. okashi's "**Folder**".
- **Loose / Resources** — unlisted `.md` at a source root or beside chapters (project-model §21:
  "unlisted `.md` = Resources"). okashi shows them as a source's "**Loose**" group.
- okashi **ignores wicklight's "project overlay"** (the marker/icon that groups manuscripts into a
  named Project) — wicklight itself says okashi treats a project as "a category-of-manuscripts it
  already handles" (project-model §4). No hard-gate change from the overlay; okashi stays lean.

## 3. Manifest writing (`manifest.go`) — the narrowed authority

Add writers alongside the existing reader, schema **exactly** v1 (`schemaVersion`,`title`,`items`):

```go
func writeManifest(dir string, m manifest) error       // MarshalIndent + atomicWrite
func createManuscript(dir, title, firstChapter string) error // new folder + manifest + first .md
func renameChapterTitle(dir, file, newTitle string) error    // edit items[].title only; keep order
```
- **`createManuscript`**: make `dir`, write `01-<slug>.md` (an empty first chapter) + a manifest
  `{schemaVersion:1, title, items:[{file:"01-<slug>.md", title:firstChapter}]}`. Pretty-printed
  JSON, atomic. (Matches wicklight's New-Project flow "container + first manuscript", adapted:
  okashi makes the *manuscript* directly — lone manuscripts are allowed, project-model §"Lone".)
- **`renameChapterTitle`**: read → set the matching `items[].title` → write back atomically;
  **never reorders or adds/removes items**. Refuse if `file` isn't a member.
- Everything else (reorder/insert/move/convert) is NOT added — wicklight-owned.

## 4. Home layout (Recent on top + inline `+`, source-aware)

Centered logo, then a full-width **RECENT** strip, then the **LIBRARY → FILES** Miller pair,
then a single **Browse** action. The active **source** is a compact header on LIBRARY (a picker,
NOT a 3rd column — the terminal can't spare the width; wicklight's Sources-column becomes a
one-line switch here).

```
                       o k a s h i
                       ───────────
  ╭ RECENT ─────────────────────────────────────╮
  │ › ch3.md    › notes.md    › draft.md          │
  ╰───────────────────────────────────────────────╯
  ╭ LIBRARY · Wicklight ▾ ── + ╮  ╭ FILES ───── + ╮
  │ ◦ Loose                    │  │ 01-opening 1204│
  │ PROJECTS                   │  │   The fog rol… │
  │ › my-novel                 │  │ 02-arrival  892│
  │ FOLDERS                    │  │   He stepped…  │
  │ › research                 │  ╰────────────────╯
  ╰────────────────────────────╯
                    Browse all files
```

- **Source picker:** the LIBRARY title shows the active source (`LIBRARY · Wicklight ▾`); a key
  (e.g. `s`) or clicking the `▾` cycles/opens the source list (add/remove/switch). Switching a
  source repopulates LIBRARY (its projects/folders/loose) and FILES.
- **LIBRARY** (per active source): a leading **◦ Loose** entry (the source root's unfiled docs),
  then **PROJECTS** (manuscripts), then **FOLDERS** (categories). Selecting any drives FILES.
- **FILES:** the selected library item's documents (chapter titles + Resources / category docs /
  loose), each `name + word-count` + a dim 1-line snippet (as shipped).
- **Inline `+`:**
  - **LIBRARY `+`** → in-place name prompt; **trailing-slash trigger**: `My Novel` →
    `createManuscript` (a real Project); `Research/` → a plain category folder. Hint in the prompt.
  - **FILES `+`** → a new document in the **selected** project/folder (in-place, as the file pane).
- **Actions:** just **Browse all files** (New-doc/New-project are now the `+`s). Horizontal.
- **Recent** spans sources (each recent remembers its source/path).

## 5. Suggestions (requested)

1. **Source as a picker, not a column (lean).** Wicklight has screen width for a Sources column;
   the terminal doesn't. A one-line switch keeps the 2-column browse roomy. Recommended.
2. **New Project makes a first chapter and opens it** — so `+ My Novel` lands you writing, not on
   an empty folder. (Matches wicklight's "container + first manuscript".)
3. **Pinning as a fast-follow** (wicklight's next slice): a top **Pinned** strip on home for
   deeply-nested projects/categories, app-side (`pins.json`), no core change. Leave room now.
4. **Multi-source search:** okashi already SHIPPED `ctrl+f` (wicklight defers search). Extend the
   engine to optionally search **all sources** (a scope beyond Project/Document) — okashi leads here.
5. **Keep `sources.json`/`pins.json` shapes aligned** with wicklight's `Source`/pin refs so the two
   apps stay conceptually identical (separate files, same fields).
6. **Don't build the project overlay** (icon/accent/named project container) — wicklight-app-only;
   okashi treats it as a category-of-manuscripts. Stay lean.
7. **`r` retitle on manifest chapters** is now allowed (edits `items[].title`) — remove the current
   block for manifest chapters (it existed only because okashi couldn't write the manifest).

## 6. Out of scope

- Reorder / insert / move / convert (wicklight-owned, unchanged).
- The project overlay marker (icon/accent/named Project container).
- Security-scoped bookmarks (okashi isn't sandboxed; folder sources use plain paths).
- Drag operations (no drag in a TUI).

## 7. Testing

- `sources`: load/merge (primary synthesized + user folders), persist add/remove, unreachable
  folder skipped, standalone (no primary corpus) still works.
- manifest writers: `createManuscript` round-trips through the existing reader (schema v1, first
  chapter listed); `renameChapterTitle` changes only the title, preserves order/membership,
  refuses non-members; atomic (no partial file).
- home: source switch repopulates LIBRARY/FILES; `◦ Loose` shows root docs; LIBRARY `+` with/without
  trailing `/` makes manuscript vs folder; FILES `+` makes a doc in the selection; render==hit-test
  across sources; `r` retitles a manifest chapter.
- contract: a manifest okashi writes is byte-shape-identical to wicklight's v1 (schemaVersion/title/
  items ordering) so wicklight reads it verbatim.

## 8. Sequencing (for the plan)

1. **Manifest writers** (`createManuscript`/`renameChapterTitle`/`writeManifest`) + the
   shared-contract doc updates in **BOTH** repos (okashi `CLAUDE.md` §1 **and** wicklight
   `SPEC.md` / multi-source §4 / project-model §4 — reverse the committed "okashi never writes
   manifests; the only writers are wicklight's `ManuscriptStore`" line). No wicklight *code*
   change, but **verify** wicklight reloads an okashi-written manifest (§0). Wire "New Project"
   (folder+manifest+first chapter) + `r` retitle for manifest chapters — on **any** source
   incl. the wicklight-shared corpus (create/retitle are no-confirm per §0), and **remove the
   current `r`-block on manifest chapters** (§5.7).
2. **Source model** (`source`/`sources.json`/load-merge) + the LIBRARY source picker + `◦ Loose`.
3. **Home layout** (Recent-on-top strip + inline `+` on LIBRARY/FILES + trailing-slash trigger +
   Browse-only actions). (Reuses the shipped Miller render/hit-test/nav.)
4. *(fast-follow, separate)* Pinned strip; multi-source search scope.
