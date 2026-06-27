# okashi — Manifest Reconciliation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development or
> superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`)
> syntax for tracking.

**Goal:** Reconcile okashi to wicklight's frozen manifest contract. okashi becomes a **reader** of
manuscript structure (`manifest.json` = order + membership + titles, read-only) while remaining a
full **writer of prose**. Drop all structural authority (reorder, insert-into-order, convert,
chapter-title rename). Keep the legacy filename-prefix model as a **read-only display fallback**
so un-migrated corpora still show.

**Source of truth:** `docs/superpowers/specs/2026-06-26-manifest-reconciliation-design.md` and,
upstream, `../inkmere/docs/superpowers/specs/2026-06-26-storage-spine-design.md` §2.1/§2.2/§2.3/§3/§6.

## Global Constraints

- **No manifest writes — ever.** okashi reads `manifest.json`; it never generates or mutates it
  (design §3.1, wicklight §6). No code path may write the file.
- **`schemaVersion` is a HARD GATE.** `manifestSchemaVersion = 1`. A present manifest whose
  version ≠ 1 (or that won't parse) is **refused**: no legacy fallback, no write, files shown
  flat as loose with a status note (design §4.1).
- **Three mutually exclusive folder states** (design §4): manifest manuscript → manifest is the
  SOLE source (filenames opaque); no manifest + numbered files → legacy prefix ordering
  (read-only); neither → category. One resolver feeds sidebar, outline, pager, export.
- **Membership is the manifest, not `sectionOrder`.** A chapter is a file listed in `items`; a
  manifest chapter need not have a numeric prefix (`the-letter.md` is valid). Never infer chapter
  membership from the filename in a manifest folder.
- **Legacy folders keep read-only outline/pager/export** (design §4, wicklight §6) — do not cut
  them to sidebar-only.
- **Markdown flavor unchanged.** goldmark + GFM + Footnote in `export_ast.go` is untouched
  (CLAUDE.md §2).
- **No new dependencies.** Standard-library `encoding/json` only.
- **Green at every task boundary.** Each task ends with BOTH `/opt/homebrew/bin/go build ./...`
  **and** `/opt/homebrew/bin/go test ./...` passing. Every "remove X" step deletes X **and** its
  tests in the same commit, so the build never references a deleted symbol.

**Toolchain:** `go` is not on PATH on this machine — invoke `/opt/homebrew/bin/go` (and
`/opt/homebrew/bin/gofmt`) per CLAUDE.md. Module is flat `package main` at the repo root.

**Working directory:** all `go` and `git` commands run from `/Users/michael/dev/okashi`.

---

### Task 1: Manifest reader (additive, the schemaVersion gate)

**Files:**
- Create: `manifest.go`
- Create: `manifest_test.go`

**Interfaces produced:**
- `type manifestItem struct { File string; Title string }` (json tags `file`, `title`)
- `type manifest struct { SchemaVersion int; Title string; Items []manifestItem }`
  (json tags `schemaVersion`, `title`, `items`)
- `const manifestName = "manifest.json"`, `const manifestSchemaVersion = 1`
- `func hasManifest(dir string) bool`
- `func readManifest(dir string) (m manifest, present bool, err error)` — `present=false, err=nil`
  when absent; `present=true, err!=nil` when present-but-unreadable or version-mismatched.

- [ ] **Step 1: Write the failing tests** — create `manifest_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifestAbsent(t *testing.T) {
	dir := t.TempDir()
	_, present, err := readManifest(dir)
	if present || err != nil {
		t.Fatalf("absent manifest: present=%v err=%v, want false,nil", present, err)
	}
	if hasManifest(dir) {
		t.Fatal("hasManifest should be false with no manifest.json")
	}
}

func TestReadManifestValid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{
		"schemaVersion": 1, "title": "Windermere",
		"items": [ {"file":"opening.md","title":"Chapter One"},
		           {"file":"the-letter.md","title":"The Letter"} ] }`), 0o644)
	m, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("valid manifest: present=%v err=%v, want true,nil", present, err)
	}
	if m.Title != "Windermere" || len(m.Items) != 2 {
		t.Fatalf("decoded = %+v, want title Windermere with 2 items", m)
	}
	if m.Items[0].File != "opening.md" || m.Items[0].Title != "Chapter One" {
		t.Fatalf("item 0 = %+v", m.Items[0])
	}
	if !hasManifest(dir) {
		t.Fatal("hasManifest should be true")
	}
}

func TestReadManifestRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":2,"title":"X","items":[]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 2 must be refused with an error, not silently read")
	}
}

func TestReadManifestRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{ not json`), 0o644)
	_, present, err := readManifest(dir)
	if !present || err == nil {
		t.Fatalf("malformed manifest: present=%v err=%v, want true,non-nil", present, err)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `/opt/homebrew/bin/go test -run TestReadManifest ./...`
  Expected: FAIL — `undefined: readManifest` / `hasManifest` / `manifestName`.

- [ ] **Step 3: Implement** — create `manifest.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const manifestName = "manifest.json"
const manifestSchemaVersion = 1

// manifestItem is one ordered chapter entry: a bare filename + a display title.
type manifestItem struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// manifest is wicklight's per-manuscript order/membership/title file. okashi reads
// it and NEVER writes it (see the reconciliation design, §3.1).
type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Items         []manifestItem `json:"items"`
}

// hasManifest reports whether dir contains a manifest.json — wicklight's manuscript
// marker (design §4: folder with manifest = manuscript).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, manifestName))
	return err == nil
}

// readManifest loads dir/manifest.json. When the file is absent it returns
// present=false, err=nil. A present-but-unreadable manifest (malformed JSON or an
// unsupported schemaVersion) returns present=true with a non-nil err: okashi
// REFUSES to guess structure and NEVER writes the file back (design §4.1).
func readManifest(dir string) (m manifest, present bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(readErr) {
		return manifest{}, false, nil
	}
	if readErr != nil {
		return manifest{}, true, readErr
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, true, err
	}
	if m.SchemaVersion != manifestSchemaVersion {
		return manifest{}, true, fmt.Errorf(
			"unsupported manifest schemaVersion %d (okashi supports %d)",
			m.SchemaVersion, manifestSchemaVersion)
	}
	return m, true, nil
}
```

- [ ] **Step 4: Run to verify pass** — `/opt/homebrew/bin/go test -run TestReadManifest ./...`
  Expected: PASS — 4 tests.

- [ ] **Step 5: Full gate + commit**

```bash
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok  okashi
git add manifest.go manifest_test.go
git commit -m "feat(manifest): read-only manifest.json reader with schemaVersion gate"
```

---

### Task 2: Remove `convert` (number-a-plain-folder)

wicklight owns "make this a manuscript" (it writes the manifest). Drop okashi's convert path.

**Files:** `main.go`, `rename.go`, `rename_wiring_test.go`, `rename_test.go`.

- [ ] **Step 1: Delete the convert tests** so the build won't reference removed symbols:
  - In `rename_test.go`: delete `TestPlanConvertNumbersAndKeepsNames`.
  - In `rename_wiring_test.go`: delete `TestConvertPromptOnPlainFolder`,
    `TestConvertNumbersFilesAndOpensOutline`, `TestConvertTracksOpenFile`.
  - In `rename_wiring_test.go`: **rewrite** `TestCtrlLNoDocsShowsNothingToConvert` →
    `TestCtrlLOnNonManuscriptStaysPut` (the new behavior: `ctrl+l` on a folder that is not a
    manuscript does nothing but set a status, never enters the outline):

```go
func TestCtrlLOnNonManuscriptStaysPut(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	plain := filepath.Join(root, "plain")
	os.MkdirAll(plain, 0o755)
	os.WriteFile(filepath.Join(plain, "a.md"), []byte("x"), 0o644) // unnumbered, no manifest
	m := sidebarModel(t, plain)

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = nm.(model)
	if m.screen == screenOutline {
		t.Fatal("ctrl+l on a non-manuscript folder must not enter the outline")
	}
}
```

- [ ] **Step 2: Remove the implementation**
  - `rename.go`: delete `planConvert`.
  - `main.go`: delete the `convertPrompt` field (struct, ~line 161); delete the `if
    m.convertPrompt { … }` handler block (~lines 367–383); delete `hasConvertibleFiles`
    (~1087) and `convertToManuscript` (~1098–1130); in the `ctrl+l` handler (~523–533) collapse
    the `switch` to: enter the outline when the folder is a manuscript, else set a status. Use
    the resolver gate that lands in Task 4; **for this task** keep the existing
    `isManuscript(m.files.entries)` check (still the legacy `[]fileEntry` form — it is renamed in
    Task 4):

```go
case "ctrl+l":
	if isManuscript(m.files.entries) {
		m.enterOutline()
	} else {
		m.status = "not a manuscript"
	}
	return m, nil
```

- [ ] **Step 3: Gate + commit**

```bash
/opt/homebrew/bin/gofmt -w main.go rename.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok
git add main.go rename.go rename_test.go rename_wiring_test.go
git commit -m "refactor: drop convert (plain-folder numbering) — wicklight owns structure creation"
```

> Note `TestCtrlLEntersOutlineInManuscript` / `TestCtrlLRejectedOutsideManuscript`
> (`outline_wiring_test.go`) still pass: both use legacy numbered fixtures and the kept
> legacy `isManuscript` check.

---

### Task 3: Remove outline reorder + insert (the largest single removal)

The outline becomes a **read-only navigator**: select/open, `m` → pager, `ctrl+e` → export,
`esc`/`enter` to leave/open. All reorder/insert/gate state goes.

**Files:** `outline.go`, `main.go`, `outline_test.go`, `outline_wiring_test.go`.

- [ ] **Step 1: Delete the reorder/insert tests**
  - `outline_test.go`: delete `TestPadWidth`, `TestExistingPrefixWidth`, `TestPlanRenamesReorder`,
    `TestPlanRenamesNoop`, `TestPlanRenamesWidens`, `TestApplyRenamesSwapNoCollision`,
    `TestApplyRenamesRejectsEscape`, `TestCommitReorderBacksUpAndRenames`,
    `TestCommitReorderNoopNoBackup`, `TestOutlineMoveSectionMakesDirty`,
    `TestPlanInsertRenamesShiftsBelow`. **Keep** `TestSectionOrder`, `TestSectionTitle`,
    `TestOrderedSections`, `TestOutlineLoadAndRows`, `TestOutlineViewShowsTitlesCountsAndTotal`
    (these are legacy-display tests and must still pass).
  - `outline_wiring_test.go`: delete `TestOutlineReorderCommitsOnEscConfirm`,
    `TestOutlineReorderDiscard`, `TestOutlineReorderTracksOpenFile`,
    `TestOutlineNewSectionInsertsAfterSelection`, `TestOutlineGateEscKeepsEditing`,
    `TestOutlineReorderApplyOpensSelectedSection`, `TestOutlineReorderDiscardOpensSelectedSection`.
    **Keep** `TestCtrlLEntersOutlineInManuscript`, `TestCtrlLRejectedOutsideManuscript`,
    `TestOutlineEnterOpensSection`, `TestOutlineEscReturnsToEditor`, `TestOutlineHandlesResize`,
    `TestOutlineClickSelectsThenDoubleClickOpens`.

- [ ] **Step 2: Remove the implementation in `outline.go`**
  - Delete `commitReorder`, `planRenames`, `planInsertRenames`, `applyRenames`,
    `existingPrefixWidth`, `padWidth`, `splitPrefix`, `commitInsert`, and the `renameOp` type if
    it has no remaining users (it is used only by the removed renumber ops — confirm with
    `grep -n renameOp *.go`; delete if unused).
  - `outlineModel`: remove fields `disk`, `confirm`, `pendingOpen`; rename `working` → `sections`
    (read-only) or keep the name but treat it as immutable. Remove methods `dirty()`,
    `moveSection()`. `load()` keeps populating `sections` + `loose` from `orderedSections`
    (legacy path; Task 4 swaps the source). `View()` currently calls `splitPrefix` for the digit
    column — replace the per-row left label with the 1-based index or just the title (no prefix),
    e.g. `left := fmt.Sprintf(" %d  %s", i+1, sectionTitle(r.entry.name))`. Keep `slugify` (used
    by export).
  - **Delete `backup.go` + `backup_test.go`** (resolved O3): `backupFiles` was called only by the
    renumber ops removed above. Confirm with `grep -n backupFiles *.go` (no remaining callers),
    then `git rm backup.go backup_test.go`.

- [ ] **Step 3: Remove the wiring in `main.go`**
  - Struct fields: delete `outlineCreating`.
  - Delete the `if m.outlineCreating { … }` handler (~743–759) and `confirmNewSection`
    (~1312–1339).
  - Delete the confirm-gate block (~836–866), `commitOutlineOrder` (~1036–1047),
    `finishOutlineOpen` (~929–940), and the dirty-gate branch in `outlineLeave` (~904–912) —
    `outlineLeave` collapses to `leaveOutlinePending` (open-or-exit, no gate).
  - In the outline key switch (~868–898): delete `J`/`K` (`moveSection`) and `n`
    (`outlineCreating`). Keep `up/k`, `down/j` (`moveSelection`), `enter` (open), `r`
    (`startRenameOutline` — changed in Task 5), `m` (`enterManuscript`), `ctrl+e` (export),
    `esc` (leave).
  - Update the outline status string (~1082) to drop "J/K reorder" and "n new".

- [ ] **Step 4: Rewrite the two kept open-path tests if the gate removal changed their flow.**
  `TestOutlineEnterOpensSection` and `TestOutlineClickSelectsThenDoubleClickOpens` should still
  pass unchanged (no reorder → no gate → Enter opens directly). Run them explicitly:
  `/opt/homebrew/bin/go test -run 'TestOutline' ./...`

- [ ] **Step 5: Gate + commit**

```bash
/opt/homebrew/bin/gofmt -w main.go outline.go
git rm backup.go backup_test.go   # O3: dead after the renumber ops are removed
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok
git add main.go outline.go outline_test.go outline_wiring_test.go
git commit -m "refactor: drop outline reorder + new-section + dead backup.go — outline is a read-only navigator"
```

---

### Task 4: Manifest-aware reads — the resolver wired into sidebar, outline, pager, export

Add the resolver and swap every read path onto it. Legacy folders must render byte-identically to
today; manifest folders gain correct order/titles/membership.

**Files:** Create `manuscript.go` + `manuscript_test.go`; edit `project.go`, `filelist.go`,
`outline.go`, `pager.go`, `export.go`, `export_ast.go`, `project_test.go`, `filelist_test.go`,
`pager_wiring_test.go`, `export_wiring_test.go`.

**Interfaces produced (in `manuscript.go`):**
- `type manuscriptSource int` with `sourceNone`, `sourceManifest`, `sourceLegacy`
- `type chapterRef struct { file string; title string }`
- `type manuscriptView struct { source manuscriptSource; title string; chapters []chapterRef; loose []fileEntry; warning string }`
- `func (v manuscriptView) ordered() bool` — true for manifest or legacy
- `func resolveManuscript(dir string, entries []fileEntry) manuscriptView`
- `func hasNumberedSections(entries []fileEntry) bool` — the **old** `isManuscript` body, renamed
- `func isManuscript(dir string) bool` — now an alias for `hasManifest` (contract-precise marker)

- [ ] **Step 1: Rename the legacy detector and split its test.** In `project.go` rename
  `isManuscript([]fileEntry) bool` → `hasNumberedSections([]fileEntry) bool` (body unchanged).
  In `project_test.go` rename `TestIsManuscript` → `TestHasNumberedSections` (call
  `hasNumberedSections`). Add a manifest-detection test:

```go
func TestIsManuscriptDetectsManifest(t *testing.T) {
	dir := t.TempDir()
	if isManuscript(dir) {
		t.Fatal("no manifest.json -> not a manifest manuscript")
	}
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[]}`), 0o644)
	if !isManuscript(dir) {
		t.Fatal("manifest.json present -> isManuscript true")
	}
}
```

> The two remaining `isManuscript(m.files.entries)` call sites (the `ctrl+l` gate from Task 2 and
> the rename guard at `main.go:~1222`) are updated in Steps 5/Task 5 to use `resolveManuscript`.
> Until then, switch them to `hasNumberedSections(m.files.entries)` so the build stays green at
> this step's commit; Step 5 moves them to the resolver.

- [ ] **Step 2: Write the resolver tests** — create `manuscript_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveManifestOrderAndTitles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("y"), 0o644)
	writeManifest(t, dir, `{"schemaVersion":1,"title":"Windermere","items":[
		{"file":"the-letter.md","title":"The Letter"},
		{"file":"opening.md","title":"Chapter One"}]}`)
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	if v.source != sourceManifest || !v.ordered() {
		t.Fatalf("want manifest source, got %v", v.source)
	}
	if v.title != "Windermere" {
		t.Fatalf("title = %q, want Windermere", v.title)
	}
	// Manifest order wins over filename alpha: the-letter before opening.
	if len(v.chapters) != 2 ||
		v.chapters[0].file != "the-letter.md" || v.chapters[0].title != "The Letter" ||
		v.chapters[1].file != "opening.md" || v.chapters[1].title != "Chapter One" {
		t.Fatalf("chapters = %+v", v.chapters)
	}
}

func TestResolveManifestUnlistedIsLoose(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644)
	writeManifest(t, dir, `{"schemaVersion":1,"title":"N","items":[{"file":"a.md","title":"One"}]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.chapters) != 1 || v.chapters[0].file != "a.md" {
		t.Fatalf("chapters = %+v, want only a.md", v.chapters)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("loose = %+v, want notes.md", v.loose)
	}
}

func TestResolveManifestAbsentFileOmitted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644) // gone.md never written
	writeManifest(t, dir, `{"schemaVersion":1,"title":"N","items":[
		{"file":"a.md","title":"One"},{"file":"gone.md","title":"Lost"}]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.chapters) != 1 || v.chapters[0].file != "a.md" {
		t.Fatalf("a truly-absent item must be omitted from display, got %+v", v.chapters)
	}
}

func TestResolveLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("z"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceLegacy || !v.ordered() {
		t.Fatalf("numbered files + no manifest -> legacy, got %v", v.source)
	}
	if v.chapters[0].file != "01-a.md" || v.chapters[1].file != "02-b.md" {
		t.Fatalf("legacy order = %+v, want numeric", v.chapters)
	}
	if v.chapters[0].title != "a" { // de-slugged
		t.Fatalf("legacy title = %q, want de-slugged 'a'", v.chapters[0].title)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("legacy loose = %+v", v.loose)
	}
}

func TestResolveCategory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "on-silence.md"), []byte("x"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceNone || v.ordered() {
		t.Fatalf("plain folder -> category, got %v", v.source)
	}
}

func TestResolveUnreadableManifestRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644) // numbered, but...
	writeManifest(t, dir, `{"schemaVersion":2,"title":"N","items":[]}`)
	v := resolveManuscript(dir, readEntries(dir))
	// Refuse to guess: NOT legacy, files shown flat as loose, warning set.
	if v.source != sourceManifest {
		t.Fatalf("unreadable manifest still marks the folder a manuscript, got %v", v.source)
	}
	if len(v.chapters) != 0 {
		t.Fatalf("must not invent chapters from a bad manifest, got %+v", v.chapters)
	}
	if v.warning == "" {
		t.Fatal("an unreadable manifest must surface a warning")
	}
	if len(v.loose) != 1 || v.loose[0].name != "01-a.md" {
		t.Fatalf("files shown flat as loose, got %+v", v.loose)
	}
}
```

- [ ] **Step 3: Run to verify failure** — `/opt/homebrew/bin/go test -run TestResolve ./...`
  Expected: FAIL — `undefined: resolveManuscript` / `manuscriptView` / `sourceManifest`.

- [ ] **Step 4: Implement `manuscript.go`:**

```go
package main

import "path/filepath"

type manuscriptSource int

const (
	sourceNone     manuscriptSource = iota // category: no manifest, no numbered files
	sourceManifest                         // manifest.json present and readable
	sourceLegacy                           // no manifest, numbered files (read-only fallback)
)

// chapterRef is one ordered chapter in a manuscript view: a base filename plus its
// resolved display title (from the manifest, or de-slugged in the legacy path).
type chapterRef struct {
	file  string
	title string
}

// manuscriptView is a folder's resolved structure. One resolver feeds the sidebar,
// outline, pager, and export so they never disagree (design §4).
type manuscriptView struct {
	source   manuscriptSource
	title    string
	chapters []chapterRef
	loose    []fileEntry // resources / loose files, in orderedSections' loose order
	warning  string      // non-empty when a manifest was present but unreadable (§4.1)
}

// ordered reports whether the folder renders as an ordered manuscript (manifest or
// legacy). Categories and the resolver's never-ordered states return false.
func (v manuscriptView) ordered() bool {
	return v.source == sourceManifest || v.source == sourceLegacy
}

// docEntries returns the non-dir document entries (loose display order: alpha,
// matching orderedSections' loose sort).
func docEntries(entries []fileEntry) []fileEntry {
	_, loose := orderedSections(filterFiles(entries))
	return loose
}

// filterFiles drops directories, keeping document fileEntry values.
func filterFiles(entries []fileEntry) []fileEntry {
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir {
			out = append(out, e)
		}
	}
	return out
}

// resolveManuscript decides a folder's structure into one of three mutually
// exclusive states (design §4). A readable manifest is the SOLE source of order,
// titles, and membership (filenames opaque). Absent a manifest, numbered files fall
// back to legacy prefix ordering (read-only). Otherwise the folder is a category.
func resolveManuscript(dir string, entries []fileEntry) manuscriptView {
	m, present, err := readManifest(dir)
	if present {
		if err != nil {
			// Refuse to guess (§4.1): show .md files flat as loose, prose still editable.
			return manuscriptView{
				source:  sourceManifest,
				title:   projectTitle(filepath.Base(dir)),
				loose:   docEntries(entries),
				warning: err.Error(),
			}
		}
		return manifestView(dir, m, entries)
	}
	if hasNumberedSections(entries) {
		sections, loose := orderedSections(filterFiles(entries))
		chapters := make([]chapterRef, 0, len(sections))
		for _, s := range sections {
			chapters = append(chapters, chapterRef{file: s.name, title: sectionTitle(s.name)})
		}
		return manuscriptView{
			source:   sourceLegacy,
			title:    projectTitle(filepath.Base(dir)),
			chapters: chapters,
			loose:    loose,
		}
	}
	return manuscriptView{
		source: sourceNone,
		title:  projectTitle(filepath.Base(dir)),
		loose:  docEntries(entries),
	}
}

// manifestView projects a readable manifest onto on-disk entries: items in manifest
// order whose file exists become chapters; every other .md is loose (a Resource).
func manifestView(dir string, m manifest, entries []fileEntry) manuscriptView {
	onDisk := map[string]bool{}
	for _, e := range entries {
		if !e.isDir {
			onDisk[e.name] = true
		}
	}
	listed := map[string]bool{}
	chapters := make([]chapterRef, 0, len(m.Items))
	for _, it := range m.Items {
		listed[it.File] = true
		if onDisk[it.File] { // omit a truly-absent file from display (§4.2)
			chapters = append(chapters, chapterRef{file: it.File, title: it.Title})
		}
	}
	var loose []fileEntry
	for _, e := range entries {
		if !e.isDir && !listed[e.name] {
			loose = append(loose, e)
		}
	}
	title := m.Title
	if title == "" {
		title = projectTitle(filepath.Base(dir))
	}
	return manuscriptView{source: sourceManifest, title: title, chapters: chapters, loose: loose}
}
```

> `readEntries(dir)` (in `outline.go`) already filters to non-hidden `allowedDocExts` files and
> excludes dirs, so `manifest.json` (a `.json`, not an `allowedDocExts`) never appears as a loose
> entry. Confirm `allowedDocExts` excludes `.json` (`grep -n allowedDocExts *.go`); if it does
> not, add a `name == manifestName` skip in `manifestView`'s loose loop and in `docEntries`.

- [ ] **Step 5: Wire the read consumers onto the resolver.** Edit each call site; legacy folders
  must stay byte-identical (the existing legacy tests are the regression guard).

  - **`filelist.go`** — `SetDir` orders entries dirs-first, then resolved chapters in view order,
    then loose. `View`/`sectionRow` decide "is this row a chapter?" from the view's chapter-file
    set and pull the row title from `chapterRef.title` (not `sectionTitle`). Compute the view once
    per `SetDir` and store it on the `filelist` (e.g. a `view manuscriptView` field) to avoid
    re-reading the manifest every frame (keep lipgloss/IO out of the hot path, CLAUDE.md
    performance model).
  - **`outline.go`** — `load` populates `sections`/`loose` from `resolveManuscript(dir,
    readEntries(dir))` (chapters → `fileEntry{name: c.file}`, plus a parallel title lookup);
    `View` renders `chapter.title` from the view. Legacy path yields the same titles as today.
  - **`pager.go`** — `load` iterates the view's chapters (in order) instead of
    `orderedSections`; the header label uses `chapterRef.title`.
  - **`export.go` / `export_ast.go`** — `runExport` (outline scope) and `manuscriptDoc` walk the
    view's chapters; `Section.Title` comes from `chapterRef.title`. Keep `parseSection` and the
    goldmark flavor untouched (CLAUDE.md §2).

- [ ] **Step 6: Add manifest-fixture wiring tests** (one per consumer) so the manifest path is
  proven end-to-end, e.g. in `filelist_test.go`:

```go
func TestSidebarRendersManifestTitleAndOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("a b"), 0o644)
	os.WriteFile(filepath.Join(dir, manifestName), []byte(
		`{"schemaVersion":1,"title":"Windermere","items":[`+
			`{"file":"the-letter.md","title":"The Letter"},`+
			`{"file":"opening.md","title":"Chapter One"}]}`), 0o644)
	f := newFilelist()                  // the filelist_test constructor idiom
	f.root = ""
	f.width, f.height = 60, 12
	f.SetDir(dir)
	view := f.View()
	if !strings.Contains(view, "The Letter") || !strings.Contains(view, "Chapter One") {
		t.Fatalf("sidebar should show manifest titles:\n%s", view)
	}
	// Manifest order: "The Letter" precedes "Chapter One" despite filename alpha.
	if strings.Index(view, "The Letter") > strings.Index(view, "Chapter One") {
		t.Fatalf("sidebar must honor manifest order, not filename order:\n%s", view)
	}
}
```

  Add analogous `TestPager…Manifest`, `TestOutline…Manifest`, and an export test asserting the
  exported doc walks manifest order with manifest titles. **Match each file's existing test
  constructor idiom** (`setupManuscript`, `manuscriptModel`, the `filelist` field setup in
  `filelist_test.go`) — read the neighbours before writing.

- [ ] **Step 7: Gate + commit**

```bash
/opt/homebrew/bin/gofmt -w *.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok
git add manuscript.go manuscript_test.go project.go project_test.go filelist.go filelist_test.go \
        outline.go pager.go pager_wiring_test.go export.go export_ast.go export_wiring_test.go main.go
git commit -m "feat(manifest): resolve order/titles/membership from manifest.json; legacy prefix as read-only fallback"
```

---

### Task 5: Rename behavior — chapters are not renamable; loose files are

Chapter titles live in the manifest okashi can't write, and chapter filenames are birth-stable
(design §6). Membership is decided by the resolver, not by `sectionOrder`.

**Files:** `rename.go`, `main.go`, `rename_test.go`, `rename_wiring_test.go`.

- [ ] **Step 1: Update the tests to the new contract**
  - `rename_test.go`: **KEEP** `TestSectionRetitleKeepsPrefixAndExt` — `sectionRetitle` is
    **retained** for legacy folders (resolved O1). **Keep** `TestLooseRenameKeepsExtensionWhenOmitted`.
  - `rename_wiring_test.go`: delete `TestSidebarRenameSectionTitleOnly` and
    `TestOutlineRenameSectionTitle`; replace with "rename is refused for a chapter":

```go
func TestRenameRefusedForManifestChapter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "the-letter.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":1,"title":"N","items":[{"file":"the-letter.md","title":"The Letter"}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("the-letter.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if m.renaming {
		t.Fatal("r on a manifest chapter must NOT start a rename (title is manifest-owned)")
	}
	if _, err := os.Stat(filepath.Join(proj, "the-letter.md")); err != nil {
		t.Fatalf("the chapter file must be untouched: %v", err)
	}
}

func TestRenameAllowedForResourceInManuscript(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "novel")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(proj, "notes.md"), []byte("y"), 0o644) // unlisted = Resource
	os.WriteFile(filepath.Join(proj, manifestName), []byte(
		`{"schemaVersion":1,"title":"N","items":[{"file":"a.md","title":"One"}]}`), 0o644)
	m := sidebarModel(t, proj)
	m.files.selectName("notes.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a Resource (unlisted file) should start a plain rename")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "scratch")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "scratch.md")); err != nil {
		t.Fatalf("Resource rename should work like a loose-file rename: %v", err)
	}
}
```

  - Add a test proving legacy retitle still works (resolved O1) — a numbered file in a
    **manifest-less** folder retitles via `sectionRetitle` (prefix preserved):

```go
func TestRenameAllowedForLegacySection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OKASHI_DIR", root)
	proj := filepath.Join(root, "legacy")
	os.MkdirAll(proj, 0o755)
	os.WriteFile(filepath.Join(proj, "01-opening.md"), []byte("x"), 0o644) // numbered, no manifest
	m := sidebarModel(t, proj)
	m.files.selectName("01-opening.md")

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = nm.(model)
	if !m.renaming {
		t.Fatal("r on a legacy numbered section should start a retitle (O1: legacy ergonomics kept)")
	}
	m.nameInput.SetValue("")
	m = typeInto(t, m, "the dawn")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(model)
	if _, err := os.Stat(filepath.Join(proj, "01-the-dawn.md")); err != nil {
		t.Fatalf("legacy retitle must preserve the numeric prefix: %v", err)
	}
}
```

  - **Keep** `TestSidebarRenameLooseFileKeepsExt`, `TestSidebarRenameFolder`,
    `TestSidebarRenameRefusesCollision`, `TestSidebarRenameTracksOpenFile`.

- [ ] **Step 2: Implement the behavior change**
  - `rename.go`: **KEEP** `sectionRetitle` (retained for legacy folders, O1) and `looseRename`.
  - `main.go` `renameTarget`: **KEEP** the `section bool` field — it now flags the legacy-retitle path.
  - `main.go` `startRename` (~1214) and `startRenameOutline` (~1233): compute chapter membership
    from the resolver (not `sectionOrder`) and branch on the manuscript source:

```go
func (m *model) startRename() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	v := resolveManuscript(m.files.dir, m.files.entries)
	if isChapterOf(v, e.name) {
		if v.source == sourceManifest {
			// manifest manuscript: titles are manifest-owned; okashi can't write them.
			m.status = "chapter titles are managed by wicklight"
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
	m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir}, e.name)
}
```

   where a small helper (in `manuscript.go`) decides membership:

```go
// isChapterOf reports whether name is a chapter of the given manuscript view (a
// manifest item, or — in a legacy folder — a numbered section). Manifest chapters
// are not renamable; legacy chapters retitle via sectionRetitle (resolved O1).
func isChapterOf(v manuscriptView, name string) bool {
	for _, c := range v.chapters {
		if c.file == name {
			return true
		}
	}
	return false
}
```

  - `startRenameOutline`: the outline lists chapters + loose; mirror `startRename` — a **manifest**
    chapter row shows the status note (no rename); a **legacy** chapter row starts a `section:true`
    retitle; a **loose** row starts a plain rename.
  - `confirmRename` (~1247): **KEEP** the `if t.section { newName = sectionRetitle(...) }` branch —
    it serves the legacy retitle path; the non-section path stays `looseRename`/dir rename.
  - Move the Task-2/Task-4 `ctrl+l` gate and any remaining `hasNumberedSections(m.files.entries)`
    pane check to `resolveManuscript(m.files.dir, m.files.entries).ordered()` so a manifest folder
    with no numbered files still enters the outline:

```go
case "ctrl+l":
	if resolveManuscript(m.files.dir, m.files.entries).ordered() {
		m.enterOutline()
	} else {
		m.status = "not a manuscript"
	}
	return m, nil
```

- [ ] **Step 3: Gate + commit**

```bash
/opt/homebrew/bin/gofmt -w main.go rename.go manuscript.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok
git add main.go rename.go manuscript.go rename_test.go rename_wiring_test.go
git commit -m "feat(manifest): chapters are not renamable (manifest-owned titles); loose/Resource rename stays"
```

> O1 (resolved — ALLOWED): legacy-folder retitle is kept. Rename is blocked only for
> `sourceManifest` chapters; `sourceLegacy` chapters route to the retained `sectionRetitle`
> (prefix-preserving), preserving pre-manifest ergonomics. (Design §6/§8.)

---

### Task 6: Update `CLAUDE.md` — mark the contract RESOLVED (doc-only)

**Files:** `CLAUDE.md`.

- [ ] **Step 1: Rewrite SHARED CONTRACTS §1** ("Manuscript ordering & membership") from
  "HARD GATE + OPEN CROSS-APP ITEM" to **RESOLVED**: order + membership + display titles live in
  wicklight's per-manuscript `manifest.json` (cite
  `../inkmere/docs/superpowers/specs/2026-06-26-storage-spine-design.md` §2.1/§6); okashi treats
  structure as **read-only** (reads the manifest; legacy filename-prefix is a read-only display
  fallback; never writes the manifest); wicklight owns reorder/insert/convert and chapter-title
  rename. Keep the standing rule: any change to the manifest **shape** is a hard gate that moves
  **both** repos together.

- [ ] **Step 2: Update the "Project model (the shipped reality)"** bullets to lead with the
  manifest (manuscript = folder with `manifest.json`; category = without; chapter = listed in
  `items`; Resource = unlisted `.md`) and demote the filename-prefix description to the legacy
  read-only fallback. Remove "outline `J/K` reorder", "`n` new section", "`r` rename", and
  "convert" from the shipped-features list; the outline is a read-only navigator. (See design §8
  O4.)

- [ ] **Step 3: Mirror-block reminder.** This `CLAUDE.md` block is mirrored in
  `../inkmere/CLAUDE.md`. Note in the commit body that wicklight's mirror of §1 must be updated to
  RESOLVED in the same coherent change (it is a HARD-GATE shared-contract edit — both move
  together). Do not edit the wicklight repo from this okashi branch.

- [ ] **Step 4: Gate + commit** (doc-only; build/test unaffected but run them to be safe)

```bash
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...   # expect: ok (unchanged)
git add CLAUDE.md
git commit -m "docs(claude): mark ordering/membership contract RESOLVED — manifest is the source, okashi reads it"
```

---

## Self-Review

- **Contract coverage:** manifest schema/version gate → Task 1; three-state read model + legacy
  fallback + unreadable-refusal → Task 4 (`TestResolve*`); membership-is-explicit → Task 4
  (`TestResolveManifestUnlistedIsLoose`) and Task 5 (`isChapterOf`); drop reorder/insert/convert →
  Tasks 2–3; never-write-manifest → enforced by absence (no writer added) and stated in Global
  Constraints; rename behavior (design §6) → Task 5; flavor unchanged → `export_ast.go` untouched;
  CLAUDE.md RESOLVED → Task 6.
- **Green-at-each-boundary:** every removal task deletes the symbol **and** its tests in the same
  commit; the legacy read tests (`TestSectionOrder/Title`, `TestOrderedSections`,
  `TestOutlineLoadAndRows`, the existing sidebar/pager/export legacy fixtures) survive Tasks 1–6
  unchanged and guard against legacy regressions.
- **Real symbols only:** every named function/field (`commitReorder`, `commitInsert`,
  `planConvert`, `sectionRetitle`, `moveSection`, `isManuscript`, `orderedSections`,
  `sectionTitle`, `sectionOrder`, `projectTitle`, `splitPrefix`, `convertPrompt`,
  `outlineCreating`, `confirmNewSection`, `commitOutlineOrder`, `finishOutlineOpen`,
  `readEntries`, `manuscriptDoc`, `looseRename`) was read in the current sources before being
  cited; line numbers are approximate (`~`) because edits shift them.
- **Open items surfaced, not silently resolved:** O1 (legacy rename) and §4.1/§4.2 behaviors are
  flagged in the design and echoed at their task; the executor follows the recommended default
  unless the human says otherwise.
- **Toolchain:** all `go`/`gofmt` calls use `/opt/homebrew/bin/...` per CLAUDE.md.
</content>
