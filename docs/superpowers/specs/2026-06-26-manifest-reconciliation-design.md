# okashi — Manifest Reconciliation — Design

> Status: design spec for human review. Implementation plan: `docs/superpowers/plans/2026-06-26-manifest-reconciliation.md`.
> Authoritative upstream contract: `../inkmere/docs/superpowers/specs/2026-06-26-storage-spine-design.md`
> (especially **§2.1 manifest schema**, **§2.2 canonical order / stable filenames**,
> **§2.3 explicit membership**, **§3 reconciliation**, and **§6 "What this spec freezes for the
> parallel okashi track"**). Where this document and that spec ever disagree, that spec wins.
> This spec does **not** re-decide the manifest format; it reconciles okashi to it.

---

## 1. Why this exists (the resolved hard gate)

okashi's `CLAUDE.md` SHARED CONTRACTS **§1 (Manuscript ordering & membership)** has stood open
since the file was reconciled on 2026-06-26: it recorded the filename-prefix convention as
okashi's authority *and* flagged that, once the sibling macOS app (`../inkmere`) landed, the
ordering decision would be **dictated from there** and brought back to reconcile.

wicklight has now frozen that decision. Per its storage-spine design **§2.1/§6**, manuscript
**structure** — order, membership, and display titles — lives in a per-manuscript
`manifest.json`, and wicklight is the **sole writer** of it. This is the human-approved
resolution of the hard gate (the wicklight spec is "approved design"; this okashi track is the
"independent parallel track" it authorizes in §6). Both repos move together as one coherent
change, never a broken intermediate (wicklight spec §6, final paragraph).

This is a **HARD-GATE shared-contract change** under both repos' `CLAUDE.md`. It has been
confirmed by the user via approval of the wicklight spec; this document records that approval and
specifies okashi's half.

## 2. The frozen contract (restated, not re-decided)

The manifest (wicklight spec §2.1), which okashi treats as **read-only**:

```json
{
  "schemaVersion": 1,
  "title": "Windermere",
  "items": [
    { "file": "opening.md",    "title": "Chapter One" },
    { "file": "the-letter.md", "title": "The Letter"  }
  ]
}
```

- `schemaVersion` (int) — okashi **refuses** a version it does not support rather than guess
  (mirrors wicklight's "refuse to read a mismatched version", §2.1).
- `title` — manuscript display title, independent of the folder name.
- `items` — the **one canonical order**; each `{ file, title }` where `file` is a bare filename
  relative to the folder root (manuscripts are flat in v1) and `title` is free-form, independent
  of the filename and of the file's content.
- **Type signal:** folder **with** `manifest.json` = manuscript; **without** = category
  (wicklight §2.1).
- **Membership is explicit** (wicklight §2.3): a file is a chapter **iff** it is listed in
  `items`. Any unlisted `.md` in a manuscript folder is a **Resource** — shown, never composed,
  never exported.
- **Filenames are birth-stable** (wicklight §2.2): wicklight assigns a filename once and never
  renames it; reorder rewrites `items`, never moves files. okashi must not assume a numeric
  prefix exists on a manifest chapter (`the-letter.md` is a perfectly valid chapter file).

## 3. Division of authority

| Concern | Before (filename-prefix model) | After (this change) |
|---|---|---|
| Manuscript **order** | leading-digit prefix on filenames | `manifest.json` `items` order (read-only) |
| Chapter **membership** | "has a numeric prefix" | listed in `items` (read-only) |
| Chapter **display title** | de-slugged filename | `items[].title` (read-only) |
| Manuscript **title** | de-slugged folder name | `manifest.title` (read-only) |
| **Reorder / insert / convert** | okashi `J/K`, `n`, `ctrl+l`-convert | **dropped** — wicklight owns structure |
| Chapter-title **rename** | okashi `r` (renumber-retitle the file) | **dropped for manifest chapters** (title is manifest-owned); **retained for legacy folders** (resolved O1) |
| **Prose** in existing chapters | okashi writes (atomic) | **unchanged** — okashi still writes |
| **Loose / new** standalone files | okashi creates / renames | **unchanged** — okashi still does |
| `manifest.json` itself | did not exist | **never written by okashi** |

**The one-line rule:** okashi gives up the authority to **restructure** a manuscript; it keeps
the authority to **write prose** and to **manage loose files**. Structure is read; prose is
read-write.

### 3.1 okashi never writes the manifest — and migration is not okashi's job

okashi performs **no** manifest generation and **no** order/membership/title writes. A
manifest-less numbered folder is **not** migrated by okashi; wicklight offers that migration on
first open (wicklight spec §6). The consequence wicklight names in §2.3 is honored from this side:
any new `.md` okashi creates inside a manuscript folder is, by wicklight's rules, a Resource until
wicklight promotes it — okashi simply writes the file and leaves `items` untouched.

## 4. The read model — three mutually exclusive folder states

okashi resolves a folder to exactly one of three states (a single resolver feeds the sidebar,
the outline, the pager, and export so they never disagree):

1. **Manifest manuscript** — `manifest.json` present and readable. The manifest is the **sole**
   source of order, titles, and membership; **filenames are opaque** (no prefix parsing). `items`
   in order whose file exists on disk become chapters; every other `.md` is a Resource.
2. **Legacy manuscript** — **no** manifest, but ≥1 numerically-prefixed file. okashi falls back
   to the **filename-prefix convention for ORDERING DISPLAY ONLY** (read-only): order by numeric
   prefix, titles de-slugged from filenames, unnumbered files are loose. This keeps un-migrated
   corpora visible and navigable during the transition (wicklight spec §6). No structural writes
   are offered here either.
3. **Category** — neither a manifest nor numbered files. A plain folder of documents.

States 1 and 2 both render as an ordered manuscript (outline / pager / export work in both);
they differ only in where order/titles come from and whether wicklight "owns" the folder. State 2
is a transitional courtesy, not a parallel authority: the moment wicklight writes a manifest, the
folder becomes state 1 and the prefixes become cosmetic.

### 4.1 Unreadable / unsupported manifest — refuse, don't guess

If `manifest.json` is present but cannot be read (malformed JSON, or `schemaVersion` ≠ the
version okashi supports), okashi **refuses to infer structure**: it does **not** fall back to
legacy prefix ordering, and it **never** writes the file. The folder is still recognized as a
manuscript (the marker file exists), but its `.md` files are shown flat as loose documents
(prose remains fully editable) and the status line surfaces a one-line note
(e.g. "manifest schemaVersion N unsupported — update okashi"). This mirrors wicklight's
refuse-mismatched-version stance (§2.1): better to show files plainly than to invent an order.

### 4.2 Known limitation — okashi does not model "not-yet-downloaded"

wicklight distinguishes *missing* from *not-yet-downloaded* via `NSMetadataQuery` (wicklight §3).
okashi has no such signal: it reads what is materialized on disk via `os.ReadDir`. A manifest
`items` entry whose file is not present on disk (including an iCloud placeholder okashi can't
see) is simply **omitted from okashi's display** for that session. This is **display-only and
read-only** — okashi never writes the manifest, so it can never *prune* the entry or lose data;
the chapter reappears once the file materializes. This asymmetry is acceptable and is named as a
non-goal (§7), not a bug.

## 5. Per-function impact map (by file, against the code as it stands)

### 5.1 Replaced / repurposed (read paths rewired to the manifest, legacy kept as fallback)

| Symbol | File | Today | After |
|---|---|---|---|
| `isManuscript([]fileEntry) bool` | `project.go` | true iff any numbered file | **renamed** `hasNumberedSections([]fileEntry)`; kept only as the **legacy** test. A new `hasManifest(dir)` answers the contract-precise "is a manifest manuscript". Pane/outline/pager/export gate on the resolver's `ordered()` (manifest **or** legacy). |
| `orderedSections([]fileEntry)` | `project.go` | splits/sorts by prefix | **kept** but demoted to the **legacy** ordering path inside the resolver only. |
| `sectionOrder` / `sectionTitle` | `project.go` | prefix → order / de-slug title | **kept** for the legacy path and for de-slugging loose/Resource display names. |
| `projectTitle(name)` | `outline.go` | de-slug folder name | **kept**; used for legacy/category titles and as the fallback when `manifest.title` is empty. |
| `projectWordCount` | `project.go` | sums section files | **kept**; now sums the resolver's chapter files (manifest or legacy). |
| `filelist.SetDir` / `filelist.View` / `sectionRow` | `filelist.go` | order + render via `orderedSections`/`isManuscript`/`sectionTitle` | **rewired** to the resolver: chapter set, order, and titles come from `manuscriptView`. Legacy folders render byte-identically to today. |
| `outlineModel.load` / `outlineModel.View` | `outline.go` | `orderedSections` + `sectionTitle`/`splitPrefix` digits | **rewired** to the resolver for chapters + titles. (Reorder state removed — see §5.2.) |
| `pagerModel.load` | `pager.go` | `orderedSections` + `sectionTitle` | **rewired** to the resolver's ordered chapters + titles. |
| `runExport` / `manuscriptDoc` | `export.go` / `export_ast.go` | `orderedSections` + `sectionTitle` | **rewired** to the resolver. Export stays read-only (emits RTF/PDF, never structure). |

### 5.2 Removed (structural authority — moves to wicklight)

| Symbol | File | Role |
|---|---|---|
| `commitReorder` | `outline.go` | `J/K` reorder → renumber-on-disk |
| `planRenames` | `outline.go` | reorder renumber plan |
| `planInsertRenames` | `outline.go` | insert-gap renumber plan |
| `applyRenames` | `outline.go` | two-phase rename executor (only used by removed ops) |
| `existingPrefixWidth`, `padWidth` | `outline.go` | zero-pad width helpers for renumbering |
| `splitPrefix` | `outline.go` | digit/rest split — used only by renumber + retitle + digit display, all removed |
| `commitInsert` | `outline.go` | `n` new-section-into-order |
| `backupFiles` + `backup_test.go` | `backup.go` | snapshot files before reorder/insert renumber — dead once those ops are removed (resolved O3: **delete the file**) |
| `outlineModel` reorder state: `working` vs `disk`, `dirty()`, `confirm`, `pendingOpen`, `moveSection` | `outline.go` | exists only to stage/commit a reorder |
| `commitOutlineOrder`, `outlineLeave` dirty-gate, `leaveOutlinePending`/`finishOutlineOpen` moved-map plumbing | `main.go` | apply/discard gate + open-through-rename |
| `outlineCreating`, `confirmNewSection`, the outline `n` key, the `J/K` keys, the confirm gate | `main.go` | new-section + reorder UI |
| `convertPrompt` field + handler; `hasConvertibleFiles`; `convertToManuscript`; the `ctrl+l` convert branch | `main.go` | "number this plain folder into a manuscript" |
| `planConvert` | `rename.go` | convert renumber plan |

**The outline survives as a read-only navigator:** select/open a chapter, `m` → pager, `ctrl+e`
→ export, `esc`/`enter` to leave/open. It loses all reorder/insert/gate machinery.

### 5.3 Kept unchanged

- `atomicWrite` (`atomicwrite.go`), `save()`, backups for prose, recents — okashi's prose-writing
  obligations (CLAUDE.md atomic-write rule) are untouched.
- `looseRename` (`rename.go`) — loose-file rename stays (see §6).
- `slugify` (`outline.go`) — still used by export to build the output filename.
- The pager's read-through and jump-to-edit; preview (`ctrl+p`); the markdown **flavor**
  (goldmark GFM + Footnote in `export_ast.go`) — **explicitly unchanged** (§7).
- `sectionRetitle` (`rename.go`) — **RETAINED, but scoped to legacy (manifest-less) numbered
  folders only** (resolved O1). It preserves okashi's pre-manifest prefix-preserving `r` retitle
  for un-migrated corpora; it is **not** offered in manifest manuscripts (titles are
  manifest-owned there). The membership guard (§6) routes `r` to it only when the file is a
  chapter of a *legacy* manuscript view.
- (`backup.go` is **deleted** — see §5.2, resolved O3.)

## 6. Rename behavior decision (the `r` key)

The chapter title now lives in the manifest, which okashi cannot write, and chapter filenames
are birth-stable (wicklight §2.2). Therefore:

- **Chapter in a manifest manuscript → rename is NOT offered.** `r` on a manifest chapter (in the
  sidebar or the outline) does nothing but show a one-line status note pointing at wicklight
  (e.g. "chapter titles are managed by wicklight"). okashi neither renames the file (would break
  the manifest's `file` reference) nor edits the title (can't write the manifest).
- **Numbered file in a *legacy* (manifest-less) folder → retitle REMAINS offered** (resolved O1).
  okashi retains its pre-manifest prefix-preserving retitle (`sectionRetitle`) for **legacy
  folders only**, so un-migrated corpora keep their familiar `r` ergonomics. The moment wicklight
  writes a manifest for that folder it becomes a manifest manuscript (state 1) and retitle is no
  longer offered there. (The human chose to preserve legacy ergonomics over strict consistency;
  legacy folders are not yet manifest-owned, so okashi retaining their pre-existing behavior is
  safe.)
- **Loose files, category documents, top-level loose files, Resources inside a manuscript, and
  folders → plain rename REMAINS** via `looseRename` (and the existing directory-rename path).
  These are not structure; renaming a loose `.md` is ordinary file management.

**Membership is decided by the manifest, not by `sectionOrder`.** The current guard
(`numbered && isManuscript`, `main.go:1222–1223`) is wrong under the new model: wicklight births
chapters with de-slugged titles and **no** numeric prefix, so `sectionOrder("the-letter.md")` is
false for a real chapter. The new guard asks the resolver: *is this file a chapter of this
folder's manuscript view?* If yes → rename blocked; otherwise → plain rename.

## 7. Non-goals (explicit)

- **No manifest writes, ever.** No generation, no order/membership/title writes, no migration.
  Migration is wicklight's (wicklight §6).
- **No structural editing in okashi.** Reorder, insert-into-order, and convert are gone, not
  merely hidden.
- **Markdown flavor is unchanged.** goldmark + GFM + Footnote (CLAUDE.md §2) stays exactly as
  shipped; this change touches structure only, never the prose syntax.
- **Theme JSON (CLAUDE.md §3) is out of scope** for this change.
- **okashi does not model iCloud download state** (§4.2). It reads materialized files only.
- **No new dependencies.** The manifest reader uses the standard-library `encoding/json` only.

## 8. Resolved decisions (human-confirmed 2026-06-26)

All four open questions were decided by the user. The spec above already reflects them; recorded
here for provenance.

- **O1 — Legacy-folder rename → ALLOWED (deviation from the plan's default).** okashi **retains**
  its pre-manifest prefix-preserving retitle (`sectionRetitle`) for **legacy (manifest-less)
  numbered folders only**, preserving familiar `r` ergonomics for un-migrated corpora. Retitle is
  still **dropped** inside manifest manuscripts. (§3, §5.3, §6.)
- **O2 — Unreadable / unsupported manifest → SHOW-AS-LOOSE.** Files are shown flat as loose
  documents (prose stays fully editable) with a one-line status note; the folder's contents are
  never hidden. (§4.1.)
- **O3 — `backup.go` → DELETE.** `backupFiles` (and `backup_test.go`) are removed; their only
  callers were the deleted structural ops. (§5.2.)
- **O4 — CLAUDE.md → REWRITE BOTH.** The reconciliation updates SHARED CONTRACTS §1 to RESOLVED
  **and** rewrites the "Project model" narrative to lead with the manifest (filename-prefix as the
  legacy read-only fallback). The mirrored SHARED CONTRACTS §1 in `../inkmere/CLAUDE.md` flips in
  lockstep, landing in the **same coherent change** when this okashi work is implemented and
  merged (never before — both move together per the hard gate).

## 9. Plan sync note

This design doc is now final. The implementation plan
(`docs/superpowers/plans/2026-06-26-manifest-reconciliation.md`) must be reconciled to these
decisions before/while it is executed:
- Keep `sectionRetitle` (gated to legacy folders) instead of removing it; route `r` via the
  membership guard (O1).
- Add a task to delete `backup.go` + `backup_test.go` (O3).
- The CLAUDE.md task rewrites the "Project model" narrative too, and includes the lockstep
  `../inkmere/CLAUDE.md` SHARED CONTRACTS §1 flip (O4).
