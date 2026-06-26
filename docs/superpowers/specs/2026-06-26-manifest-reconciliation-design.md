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

inkmere has now frozen that decision. Per its storage-spine design **§2.1/§6**, manuscript
**structure** — order, membership, and display titles — lives in a per-manuscript
`manifest.json`, and inkmere is the **sole writer** of it. This is the human-approved
resolution of the hard gate (the inkmere spec is "approved design"; this okashi track is the
"independent parallel track" it authorizes in §6). Both repos move together as one coherent
change, never a broken intermediate (inkmere spec §6, final paragraph).

This is a **HARD-GATE shared-contract change** under both repos' `CLAUDE.md`. It has been
confirmed by the user via approval of the inkmere spec; this document records that approval and
specifies okashi's half.

## 2. The frozen contract (restated, not re-decided)

The manifest (inkmere spec §2.1), which okashi treats as **read-only**:

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
  (mirrors inkmere's "refuse to read a mismatched version", §2.1).
- `title` — manuscript display title, independent of the folder name.
- `items` — the **one canonical order**; each `{ file, title }` where `file` is a bare filename
  relative to the folder root (manuscripts are flat in v1) and `title` is free-form, independent
  of the filename and of the file's content.
- **Type signal:** folder **with** `manifest.json` = manuscript; **without** = category
  (inkmere §2.1).
- **Membership is explicit** (inkmere §2.3): a file is a chapter **iff** it is listed in
  `items`. Any unlisted `.md` in a manuscript folder is a **Resource** — shown, never composed,
  never exported.
- **Filenames are birth-stable** (inkmere §2.2): inkmere assigns a filename once and never
  renames it; reorder rewrites `items`, never moves files. okashi must not assume a numeric
  prefix exists on a manifest chapter (`the-letter.md` is a perfectly valid chapter file).

## 3. Division of authority

| Concern | Before (filename-prefix model) | After (this change) |
|---|---|---|
| Manuscript **order** | leading-digit prefix on filenames | `manifest.json` `items` order (read-only) |
| Chapter **membership** | "has a numeric prefix" | listed in `items` (read-only) |
| Chapter **display title** | de-slugged filename | `items[].title` (read-only) |
| Manuscript **title** | de-slugged folder name | `manifest.title` (read-only) |
| **Reorder / insert / convert** | okashi `J/K`, `n`, `ctrl+l`-convert | **dropped** — inkmere owns structure |
| Chapter-title **rename** | okashi `r` (renumber-retitle the file) | **dropped** — title is manifest-owned |
| **Prose** in existing chapters | okashi writes (atomic) | **unchanged** — okashi still writes |
| **Loose / new** standalone files | okashi creates / renames | **unchanged** — okashi still does |
| `manifest.json` itself | did not exist | **never written by okashi** |

**The one-line rule:** okashi gives up the authority to **restructure** a manuscript; it keeps
the authority to **write prose** and to **manage loose files**. Structure is read; prose is
read-write.

### 3.1 okashi never writes the manifest — and migration is not okashi's job

okashi performs **no** manifest generation and **no** order/membership/title writes. A
manifest-less numbered folder is **not** migrated by okashi; inkmere offers that migration on
first open (inkmere spec §6). The consequence inkmere names in §2.3 is honored from this side:
any new `.md` okashi creates inside a manuscript folder is, by inkmere's rules, a Resource until
inkmere promotes it — okashi simply writes the file and leaves `items` untouched.

## 4. The read model — three mutually exclusive folder states

okashi resolves a folder to exactly one of three states (a single resolver feeds the sidebar,
the outline, the pager, and export so they never disagree):

1. **Manifest manuscript** — `manifest.json` present and readable. The manifest is the **sole**
   source of order, titles, and membership; **filenames are opaque** (no prefix parsing). `items`
   in order whose file exists on disk become chapters; every other `.md` is a Resource.
2. **Legacy manuscript** — **no** manifest, but ≥1 numerically-prefixed file. okashi falls back
   to the **filename-prefix convention for ORDERING DISPLAY ONLY** (read-only): order by numeric
   prefix, titles de-slugged from filenames, unnumbered files are loose. This keeps un-migrated
   corpora visible and navigable during the transition (inkmere spec §6). No structural writes
   are offered here either.
3. **Category** — neither a manifest nor numbered files. A plain folder of documents.

States 1 and 2 both render as an ordered manuscript (outline / pager / export work in both);
they differ only in where order/titles come from and whether inkmere "owns" the folder. State 2
is a transitional courtesy, not a parallel authority: the moment inkmere writes a manifest, the
folder becomes state 1 and the prefixes become cosmetic.

### 4.1 Unreadable / unsupported manifest — refuse, don't guess

If `manifest.json` is present but cannot be read (malformed JSON, or `schemaVersion` ≠ the
version okashi supports), okashi **refuses to infer structure**: it does **not** fall back to
legacy prefix ordering, and it **never** writes the file. The folder is still recognized as a
manuscript (the marker file exists), but its `.md` files are shown flat as loose documents
(prose remains fully editable) and the status line surfaces a one-line note
(e.g. "manifest schemaVersion N unsupported — update okashi"). This mirrors inkmere's
refuse-mismatched-version stance (§2.1): better to show files plainly than to invent an order.

### 4.2 Known limitation — okashi does not model "not-yet-downloaded"

inkmere distinguishes *missing* from *not-yet-downloaded* via `NSMetadataQuery` (inkmere §3).
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

### 5.2 Removed (structural authority — moves to inkmere)

| Symbol | File | Role |
|---|---|---|
| `commitReorder` | `outline.go` | `J/K` reorder → renumber-on-disk |
| `planRenames` | `outline.go` | reorder renumber plan |
| `planInsertRenames` | `outline.go` | insert-gap renumber plan |
| `applyRenames` | `outline.go` | two-phase rename executor (only used by removed ops) |
| `existingPrefixWidth`, `padWidth` | `outline.go` | zero-pad width helpers for renumbering |
| `splitPrefix` | `outline.go` | digit/rest split — used only by renumber + retitle + digit display, all removed |
| `commitInsert` | `outline.go` | `n` new-section-into-order |
| `outlineModel` reorder state: `working` vs `disk`, `dirty()`, `confirm`, `pendingOpen`, `moveSection` | `outline.go` | exists only to stage/commit a reorder |
| `commitOutlineOrder`, `outlineLeave` dirty-gate, `leaveOutlinePending`/`finishOutlineOpen` moved-map plumbing | `main.go` | apply/discard gate + open-through-rename |
| `outlineCreating`, `confirmNewSection`, the outline `n` key, the `J/K` keys, the confirm gate | `main.go` | new-section + reorder UI |
| `convertPrompt` field + handler; `hasConvertibleFiles`; `convertToManuscript`; the `ctrl+l` convert branch | `main.go` | "number this plain folder into a manuscript" |
| `planConvert` | `rename.go` | convert renumber plan |
| `sectionRetitle` | `rename.go` | `r` chapter-title rename via filename renumber |

**The outline survives as a read-only navigator:** select/open a chapter, `m` → pager, `ctrl+e`
→ export, `esc`/`enter` to leave/open. It loses all reorder/insert/gate machinery.

### 5.3 Kept unchanged

- `atomicWrite` (`atomicwrite.go`), `save()`, backups for prose, recents — okashi's prose-writing
  obligations (CLAUDE.md atomic-write rule) are untouched.
- `looseRename` (`rename.go`) — loose-file rename stays (see §6).
- `slugify` (`outline.go`) — still used by export to build the output filename.
- The pager's read-through and jump-to-edit; preview (`ctrl+p`); the markdown **flavor**
  (goldmark GFM + Footnote in `export_ast.go`) — **explicitly unchanged** (§7).
- `backup.go` / `backupFiles` — the only callers were the removed structural ops; the helper may
  be left in place (dead but harmless) or removed in a later cleanup. The plan leaves it in
  place to keep this change focused on the contract. *(Open question O3.)*

## 6. Rename behavior decision (the `r` key)

The chapter title now lives in the manifest, which okashi cannot write, and chapter filenames
are birth-stable (inkmere §2.2). Therefore:

- **Chapter in a manifest manuscript → rename is NOT offered.** `r` on a manifest chapter (in the
  sidebar or the outline) does nothing but show a one-line status note pointing at inkmere
  (e.g. "chapter titles are managed by inkmere"). okashi neither renames the file (would break
  the manifest's `file` reference) nor edits the title (can't write the manifest).
- **Numbered file in a *legacy* (manifest-less) folder → rename also NOT offered as a retitle.**
  Renaming it would mutate the legacy ordering/title, which is **structure** okashi no longer
  authors. This is the conservative, consistent choice; it is the one case the inkmere spec is
  silent on, and it is flagged as **Open question O1** for the human to relax if desired.
- **Loose files, category documents, top-level loose files, Resources inside a manuscript, and
  folders → plain rename REMAINS** via `looseRename` (and the existing directory-rename path).
  These are not structure; renaming a loose `.md` is ordinary file management.

**Membership is decided by the manifest, not by `sectionOrder`.** The current guard
(`numbered && isManuscript`, `main.go:1222–1223`) is wrong under the new model: inkmere births
chapters with de-slugged titles and **no** numeric prefix, so `sectionOrder("the-letter.md")` is
false for a real chapter. The new guard asks the resolver: *is this file a chapter of this
folder's manuscript view?* If yes → rename blocked; otherwise → plain rename.

## 7. Non-goals (explicit)

- **No manifest writes, ever.** No generation, no order/membership/title writes, no migration.
  Migration is inkmere's (inkmere §6).
- **No structural editing in okashi.** Reorder, insert-into-order, and convert are gone, not
  merely hidden.
- **Markdown flavor is unchanged.** goldmark + GFM + Footnote (CLAUDE.md §2) stays exactly as
  shipped; this change touches structure only, never the prose syntax.
- **Theme JSON (CLAUDE.md §3) is out of scope** for this change.
- **okashi does not model iCloud download state** (§4.2). It reads materialized files only.
- **No new dependencies.** The manifest reader uses the standard-library `encoding/json` only.

## 8. Open questions for the human

- **O1 — Legacy-folder rename.** §6 disallows retitling a numbered file in a *manifest-less*
  folder (treating legacy order as read-only structure). The inkmere spec is silent here. Relax
  to "plain rename allowed in legacy folders" if you'd rather keep the pre-manifest ergonomics
  during the transition. *(Recommendation: keep it disallowed — consistent and safe.)*
- **O2 — Unreadable-manifest display.** §4.1 shows a version-mismatched manifest's files as flat
  loose docs with a status note. Alternative: hide the folder's contents entirely behind the
  warning. *(Recommendation: show-as-loose — never hide a user's prose.)*
- **O3 — `backup.go` disposition.** After the structural ops are removed, `backupFiles` has no
  callers. Leave it dead-but-present (plan's default) or delete it for tidiness?
- **O4 — CLAUDE.md "Project model" prose.** That section still describes the filename-prefix
  model as "the shipped reality." The plan updates §1 of SHARED CONTRACTS to RESOLVED; confirm
  it should also rewrite the "Project model" narrative to lead with the manifest (legacy as
  fallback). *(Recommendation: yes — update both.)*
</content>
</invoke>
