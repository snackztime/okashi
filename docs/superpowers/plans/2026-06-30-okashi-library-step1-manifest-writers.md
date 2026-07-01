# okashi Library — Step 1: Manifest Writers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make okashi a manifest *writer* — "New Project" creates a real manuscript (folder + `manifest.json` + first chapter you land in), `r` retitles a manifest chapter, and the file pane reads its container's name — then reflect the reversed authority in both repos' contracts.

**Architecture:** Add three pure writers to `manifest.go` (`writeManifest`, `createManuscript`, `renameChapterTitle`) built on the existing `atomicWrite`. Wire them into the two existing UI paths (`confirmCreate` for New Project, the `startRename`/`confirmRename` machinery for retitle), replacing the current "managed by wicklight" refusals. Add a one-method file-pane label. Close with the shared-contract doc updates in okashi **and** wicklight.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), `encoding/json`, existing `atomicWrite` (`atomicwrite.go`), existing `slugify` (`outline.go`), existing `resolveManuscript`/`manuscriptView` (`manuscript.go`).

## Global Constraints

- **Invoke Go as `/opt/homebrew/bin/go`** and gofmt as `/opt/homebrew/bin/gofmt` — neither is on PATH.
- **Manifest schema is EXACTLY v1** — struct is `{schemaVersion:1, title, items:[{file, title}]}` (`manifest`/`manifestItem` in `manifest.go`). No new fields. Any schema change is a HARD-GATE that STOPS work (CLAUDE.md §1 / spec §0).
- **All manifest writes go through `atomicWrite`** (temp-then-rename). Never write a manifest in place.
- **Read-modify-write** before any edit to an *existing* manifest: re-read immediately before writing so okashi rewrites the latest on-disk state (spec §0).
- **Authority (spec §0):** create + chapter-title retitle are **no-confirm** and allowed on **any** source, including the wicklight-shared corpus. Structural edits (reorder/insert/move) are NOT in this step.
- **Default build stays pure-Go.** No new dependencies.
- **Keep it lean** — surface area here propagates to wicklight.

---

### Task 1: Manifest writers (`writeManifest`, `createManuscript`, `renameChapterTitle`)

**Files:**
- Modify: `manifest.go` (append after `readManifest`, line 55)
- Test: `manifest_writers_test.go` (create)

**Interfaces:**
- Consumes: `manifest`, `manifestItem`, `manifestName`, `manifestSchemaVersion`, `hasManifest`, `readManifest` (all `manifest.go`); `atomicWrite` (`atomicwrite.go`); `slugify` (`outline.go`).
- Produces:
  - `func writeManifest(dir string, m manifest) error`
  - `func createManuscript(dir, title, firstChapter string) (firstFile string, err error)`
  - `func renameChapterTitle(dir, file, newTitle string) error`

- [ ] **Step 1: Write the failing tests**

Create `manifest_writers_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateManuscriptRoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-novel")
	first, err := createManuscript(dir, "My Novel", "Untitled")
	if err != nil {
		t.Fatalf("createManuscript: %v", err)
	}
	if first != "01-untitled.md" {
		t.Fatalf("first chapter file = %q, want 01-untitled.md", first)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("first chapter not on disk: %v", err)
	}
	m, present, err := readManifest(dir)
	if err != nil || !present {
		t.Fatalf("readManifest present=%v err=%v", present, err)
	}
	if m.SchemaVersion != manifestSchemaVersion || m.Title != "My Novel" {
		t.Fatalf("manifest = %+v", m)
	}
	if len(m.Items) != 1 || m.Items[0].File != "01-untitled.md" || m.Items[0].Title != "Untitled" {
		t.Fatalf("items = %+v", m.Items)
	}
	// The resolver must see it as an ordered manifest manuscript.
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceManifest || !v.ordered() || len(v.chapters) != 1 {
		t.Fatalf("resolved view = %+v", v)
	}
}

func TestCreateManuscriptRefusesExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dup")
	if _, err := createManuscript(dir, "One", "Untitled"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := createManuscript(dir, "Two", "Untitled"); err == nil {
		t.Fatal("second create should refuse an existing manifest")
	}
}

func TestRenameChapterTitleChangesOnlyTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "book")
	first, _ := createManuscript(dir, "Book", "Untitled")
	if err := renameChapterTitle(dir, first, "Opening"); err != nil {
		t.Fatalf("renameChapterTitle: %v", err)
	}
	m, _, _ := readManifest(dir)
	if m.Items[0].Title != "Opening" {
		t.Fatalf("title = %q, want Opening", m.Items[0].Title)
	}
	if m.Items[0].File != first {
		t.Fatalf("filename changed to %q — must be birth-stable", m.Items[0].File)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("chapter file must NOT be renamed on disk: %v", err)
	}
}

func TestRenameChapterTitlePreservesOrderAndOthers(t *testing.T) {
	dir := t.TempDir()
	m := manifest{SchemaVersion: 1, Title: "T", Items: []manifestItem{
		{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "B"}, {File: "03-c.md", Title: "C"},
	}}
	if err := writeManifest(dir, m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	if err := renameChapterTitle(dir, "02-b.md", "Bee"); err != nil {
		t.Fatalf("renameChapterTitle: %v", err)
	}
	got, _, _ := readManifest(dir)
	want := []manifestItem{{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "Bee"}, {File: "03-c.md", Title: "C"}}
	if len(got.Items) != 3 {
		t.Fatalf("items = %+v", got.Items)
	}
	for i := range want {
		if got.Items[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got.Items[i], want[i])
		}
	}
}

func TestRenameChapterTitleRefusesNonMember(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "book")
	createManuscript(dir, "Book", "Untitled")
	if err := renameChapterTitle(dir, "99-ghost.md", "Nope"); err == nil {
		t.Fatal("retitling a non-member should error")
	}
}

func TestWriteManifestForcesSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "X", Items: []manifestItem{{File: "01-a.md", Title: "A"}}}); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	m, _, err := readManifest(dir)
	if err != nil {
		t.Fatalf("readback rejected (schemaVersion not forced?): %v", err)
	}
	if m.SchemaVersion != manifestSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", m.SchemaVersion, manifestSchemaVersion)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `/opt/homebrew/bin/go test ./... -run 'Manuscript|ChapterTitle|WriteManifest' -v`
Expected: FAIL — `undefined: createManuscript` / `writeManifest` / `renameChapterTitle`.

- [ ] **Step 3: Write the implementation**

Append to `manifest.go` (after line 55). Add `"os"` is already imported; `fmt`, `filepath`, `encoding/json` already imported.

```go
// writeManifest serializes m to dir/manifest.json atomically, pretty-printed with a
// trailing newline. okashi owns manifest writes for its own AND the wicklight-shared
// corpus (design §0); the schema is forced to EXACTLY v1 so wicklight reads it verbatim.
func writeManifest(dir string, m manifest) error {
	m.SchemaVersion = manifestSchemaVersion
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, manifestName), append(data, '\n'), 0o644)
}

// createManuscript makes a brand-new manuscript at dir: the folder, an empty first
// chapter "01-<slug>.md", and a v1 manifest listing it. firstChapter is that chapter's
// display title. It refuses to clobber an existing manifest and returns the first
// chapter's filename so the caller can open it.
func createManuscript(dir, title, firstChapter string) (string, error) {
	if hasManifest(dir) {
		return "", fmt.Errorf("a manuscript already exists at %s", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	file := "01-" + slugify(firstChapter) + ".md"
	if err := atomicWrite(filepath.Join(dir, file), []byte(""), 0o644); err != nil {
		return "", err
	}
	return file, writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         []manifestItem{{File: file, Title: firstChapter}},
	})
}

// renameChapterTitle edits ONLY the items[].title of the chapter file in dir's manifest,
// preserving order and membership; the filename is birth-stable (design §5.7). It
// read-modify-writes (re-reads immediately before writing, §0) and refuses a file that is
// not a listed chapter or a dir without a readable manifest.
func renameChapterTitle(dir, file, newTitle string) error {
	m, present, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("no manifest in %s", dir)
	}
	found := false
	for i := range m.Items {
		if m.Items[i].File == file {
			m.Items[i].Title = newTitle
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s is not a chapter of %s", file, dir)
	}
	return writeManifest(dir, m)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'Manuscript|ChapterTitle|WriteManifest' -v`
Expected: PASS (all 6). Then `/opt/homebrew/bin/gofmt -l manifest.go manifest_writers_test.go` prints nothing.

- [ ] **Step 5: Commit**

```bash
git add manifest.go manifest_writers_test.go
git commit -m "feat: manifest writers (writeManifest, createManuscript, renameChapterTitle)"
```

---

### Task 2: Wire "New Project" to create a real manuscript

**Files:**
- Modify: `main.go` — `confirmCreate`, the `if folder {` block (currently lines 1739-1756)
- Test: `manifest_writers_test.go` (add a wiring test)

**Interfaces:**
- Consumes: `createManuscript` (Task 1); `m.files.SetDir`, `m.loadFile`, `m.files.selectName` (existing); `m.creatingFolder`/`m.creatingInPane`/`m.creatingFile` flags (existing).
- Produces: New Project now yields a manifest manuscript with a first chapter and opens it.

**Context:** `confirmCreate` handles two folder cases. `explicitFolder` (the "New project" action / `m.creatingFolder`) previously made a *plain* folder and entered it — a category mislabeled "new project". The `name/` trailing-slash convention makes a plain category and stays. This task changes ONLY the `explicitFolder` branch to build a manuscript; the `name/` category branch is unchanged.

- [ ] **Step 1: Write the failing test**

Add to `manifest_writers_test.go`:

```go
func TestConfirmCreateNewProjectMakesManuscript(t *testing.T) {
	root := t.TempDir()
	m := initialModel() // constructor used by all model tests (e.g. smoke_test.go:369)
	m.files.root = ""   // allow an arbitrary temp dir as root (smoke_test.go pattern)
	m.files.SetDir(root)
	m.creatingFile = true
	m.creatingFolder = true // the New-Project action
	m.creatingInPane = true
	m.nameInput.SetValue("My Novel")

	m.confirmCreate()

	dir := filepath.Join(root, "My Novel")
	if !hasManifest(dir) {
		t.Fatalf("New Project should create a manifest at %s", dir)
	}
	if m.files.dir != dir {
		t.Fatalf("pane dir = %q, want %q (should enter the project)", m.files.dir, dir)
	}
	if filepath.Base(m.currentFile) != "01-untitled.md" {
		t.Fatalf("currentFile = %q, want the opened first chapter", m.currentFile)
	}
	if m.focus != focusEditor {
		t.Fatalf("focus = %v, want focusEditor (land writing)", m.focus)
	}
}
```

**Model construction:** the codebase constructor is `initialModel()` (no `newTestModel` helper exists). Tests that use a temp dir set `m.files.root = ""` then `m.files.SetDir(dir)` — see `smoke_test.go:369-372` (`TestNewFileDoesNotAutosaveEmpty`), which drives the very same `confirmCreate` prompt flow. Mirror that exactly.

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestConfirmCreateNewProject -v`
Expected: FAIL — no manifest created (current code makes a plain folder).

- [ ] **Step 3: Replace the `if folder {` block**

In `main.go` `confirmCreate`, replace the whole `if folder { ... }` block (currently lines 1739-1756) with:

```go
	if folder {
		dir := filepath.Join(m.files.dir, name)
		if explicitFolder {
			// New Project → a real manuscript (folder + manifest + first chapter you land in).
			first, err := createManuscript(dir, name, "Untitled")
			if err != nil {
				m.status = "couldn't create project: " + err.Error()
				return
			}
			m.files.SetDir(dir)
			m.loadFile(filepath.Join(dir, first))
			m.focus = focusEditor
			m.editor.Focus()
			m.status = "new project " + name + " — start writing"
			return
		}
		// "name/" convention → a plain category folder; refresh and stay.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.status = "couldn't create folder: " + err.Error()
			return
		}
		m.files.SetDir(m.files.dir)
		m.files.selectName(name)
		m.status = "created folder " + name
		m.focus = focusSidebar
		m.editor.Blur()
		return
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'ConfirmCreate|Manuscript' -v`
Expected: PASS. Then the full suite `/opt/homebrew/bin/go test ./...` (guard the category/`name/` path — the existing create smoke tests must still pass). `/opt/homebrew/bin/go vet ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add main.go manifest_writers_test.go
git commit -m "feat: New Project creates a manuscript and opens its first chapter"
```

---

### Task 3: Retitle a manifest chapter (edit items[].title, remove the r-block)

**Files:**
- Modify: `main.go` — `renameTarget` struct (line 149-154), `startRename` (~1812-1817), `startRenameOutline` (~1942-1946), `confirmRename` (~1957-1968)
- Test: `manifest_writers_test.go` (add retitle-wiring tests)

**Interfaces:**
- Consumes: `renameChapterTitle` (Task 1); `m.files.chapterTitle`, `m.outline.chapterTitle` (existing); `m.beginRename`, `m.refreshAfterRename`, `renameTarget` (existing).
- Produces: `renameTarget` gains a `manifestChapter bool`; manifest chapters retitle instead of refusing.

**Context:** Manifest chapters currently REFUSE rename ("chapter titles are managed by wicklight") in two entry points. Legacy chapters do a *file* rename via `sectionRetitle` (prefix-preserving) — that path is unchanged. The manifest path is different in kind: edit the title field, keep the filename. A new `manifestChapter` flag routes `confirmRename` to `renameChapterTitle`.

- [ ] **Step 1: Add the flag to `renameTarget`**

`main.go` line 149-154, add one field:

```go
type renameTarget struct {
	dir             string // directory containing the item
	name            string // current base name
	isDir           bool
	section         bool // a numbered section -> title-only rename (legacy file rename)
	manifestChapter bool // a manifest chapter -> edit items[].title, filename birth-stable
}
```

- [ ] **Step 2: Write the failing test**

Add to `manifest_writers_test.go`:

```go
func TestStartRenameManifestChapterRetitles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "book")
	first, _ := createManuscript(dir, "Book", "Untitled")

	m := initialModel() // same construction as Task 2 (smoke_test.go:369)
	m.files.root = ""
	m.files.SetDir(dir)
	m.files.selectName(first)

	m.startRename()
	if !m.renaming || !m.renameTarget.manifestChapter {
		t.Fatalf("startRename on a manifest chapter should begin a manifestChapter rename; target=%+v renaming=%v", m.renameTarget, m.renaming)
	}
	if got := m.nameInput.Value(); got != "Untitled" {
		t.Fatalf("prefill = %q, want the current chapter title 'Untitled'", got)
	}

	m.nameInput.SetValue("The Opening")
	m.confirmRename()

	mf, _, _ := readManifest(dir)
	if mf.Items[0].Title != "The Opening" {
		t.Fatalf("title = %q, want 'The Opening'", mf.Items[0].Title)
	}
	if mf.Items[0].File != first {
		t.Fatalf("filename changed to %q — must stay birth-stable", mf.Items[0].File)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("chapter file must not be renamed on disk: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestStartRenameManifestChapter -v`
Expected: FAIL — `startRename` still sets the refuse status, `renameTarget.manifestChapter` is false.

- [ ] **Step 4: Replace the refuse block in `startRename`**

`main.go` ~1812-1824. Replace the manifest branch inside `if isChapterOf(v, e.name) {`:

```go
	if isChapterOf(v, e.name) {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.renamingInPane = true
			m.nameInput.Width = m.files.width
			m.beginRename(renameTarget{dir: m.files.dir, name: e.name, manifestChapter: true},
				m.files.chapterTitle(e.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.renamingInPane = true
		m.nameInput.Width = m.files.width
		m.beginRename(renameTarget{dir: m.files.dir, name: e.name, isDir: e.isDir, section: true},
			sectionTitle(e.name))
		return
	}
```

- [ ] **Step 5: Replace the refuse block in `startRenameOutline`**

`main.go` ~1941-1950. Replace the manifest branch inside `if row.isSection {`:

```go
	if row.isSection {
		if v.source == sourceManifest {
			// manifest manuscript: retitle the manifest entry; filename is birth-stable (§5.7).
			m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, manifestChapter: true},
				m.outline.chapterTitle(row.entry.name))
			return
		}
		// legacy (manifest-less) folder: retain pre-manifest prefix-preserving retitle (O1).
		m.beginRename(renameTarget{dir: m.outline.dir, name: row.entry.name, isDir: false, section: true},
			sectionTitle(row.entry.name))
		return
	}
```

- [ ] **Step 6: Route `confirmRename` to `renameChapterTitle`**

`main.go` in `confirmRename`, immediately after the `if typed == "" { ... }` guard (after line 1967, before `var newName string`):

```go
	if t.manifestChapter {
		if err := renameChapterTitle(t.dir, t.name, typed); err != nil {
			m.status = "retitle failed: " + err.Error()
		} else {
			m.status = "retitled to " + typed
		}
		m.refreshAfterRename()
		return
	}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'Rename|Manuscript|ChapterTitle' -v`
Expected: PASS, including any existing legacy-rename tests (regression: legacy chapters still file-rename). Full suite + vet clean.

- [ ] **Step 8: Commit**

```bash
git add main.go manifest_writers_test.go
git commit -m "feat: retitle manifest chapters via manifest items[].title"
```

---

### Task 4: File-pane header reads the container name

**Files:**
- Create method in: `filelist.go` (near `breadcrumb`, ~line 281)
- Modify: `main.go` — the sidebar `title` computation (lines 1365-1368)
- Test: `filelist_test.go` (add a paneLabel test)

**Interfaces:**
- Consumes: `f.dir`, `f.root`, `f.view` (`manuscriptView` with `.ordered()` and `.title`), `filepath.Base` (existing).
- Produces: `func (f filelist) paneLabel() string`.

**Context:** The sidebar is framed by `framedPanel(title, ...)` in `main.go:1375`. Today `title` is `filepath.Base(m.files.dir)` with a `"Files"` fallback only when `dir == ""` (never in practice). The ask: project name for a manuscript (its `manifest.title`, which can differ from the folder), folder name for a category, and `"Files"` at the source **root** (loose docs).

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go` (mirror how existing filelist tests build a `filelist` + tmp dirs):

```go
func TestPaneLabel(t *testing.T) {
	root := t.TempDir()
	// A manuscript whose title differs from its folder name.
	proj := filepath.Join(root, "novel-dir")
	if _, err := createManuscript(proj, "The Real Title", "Untitled"); err != nil {
		t.Fatal(err)
	}
	// A plain category folder.
	cat := filepath.Join(root, "research")
	os.MkdirAll(cat, 0o755)

	var f filelist
	f.SetDir(root) // if SetDir needs a root set first, mirror the existing test setup
	f.root = root

	f.SetDir(root)
	if got := f.paneLabel(); got != "Files" {
		t.Fatalf("root paneLabel = %q, want Files", got)
	}
	f.SetDir(proj)
	if got := f.paneLabel(); got != "The Real Title" {
		t.Fatalf("manuscript paneLabel = %q, want the manifest title", got)
	}
	f.SetDir(cat)
	if got := f.paneLabel(); got != "research" {
		t.Fatalf("category paneLabel = %q, want the folder name", got)
	}
}
```

**Note:** if `filelist`'s zero value can't `SetDir` cleanly, copy the construction the nearest existing `filelist_test.go` case uses (e.g. the breadcrumb tests around line 158). `f.root` must be set so the root comparison works.

- [ ] **Step 2: Run test to verify it fails**

Run: `/opt/homebrew/bin/go test ./... -run TestPaneLabel -v`
Expected: FAIL — `f.paneLabel undefined`.

- [ ] **Step 3: Add `paneLabel`**

In `filelist.go`, after `breadcrumb` (~line 289):

```go
// paneLabel is the file-pane header: "Files" at the source root, the manuscript title
// for a manuscript (manifest or legacy), else the folder name for a category.
func (f filelist) paneLabel() string {
	if f.dir == "" || f.dir == f.root {
		return "Files"
	}
	if f.view.ordered() {
		return f.view.title
	}
	return filepath.Base(f.dir)
}
```

- [ ] **Step 4: Use it in the View**

`main.go` lines 1365-1368, replace:

```go
		title := filepath.Base(m.files.dir)
		if m.files.dir == "" {
			title = "Files"
		}
```

with:

```go
		title := m.files.paneLabel()
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `/opt/homebrew/bin/go test ./... -run 'PaneLabel|Breadcrumb|Filelist' -v`
Expected: PASS. Full suite + vet + gofmt clean.

- [ ] **Step 6: Commit**

```bash
git add filelist.go main.go filelist_test.go
git commit -m "feat: file-pane header reads project/folder name or Files"
```

---

### Task 5: Shared-contract doc updates (BOTH repos) + round-trip verification

**Files:**
- Modify: `CLAUDE.md` (okashi) — Project model + SHARED CONTRACTS §1
- Modify (wicklight, `../inkmere`): `SPEC.md`, `docs/superpowers/specs/2026-06-27-multi-source-library-design.md` (§4), `docs/superpowers/specs/2026-06-27-project-model-design.md` (§4) — wherever the "okashi never writes manifests" line lives
- No test (docs + manual verification)

**Interfaces:**
- Consumes: nothing (documentation). This is the HARD-GATE mirror step required by spec §0.
- Produces: both repos' contracts state okashi is a manifest writer (create/retitle no-confirm; structural edits confirm, deferred).

**Context:** okashi's code now writes manifests. Both repos currently say it never does. spec §0 mandates reversing this in BOTH repos together. This is documentation only — **no wicklight code change** — but you MUST verify wicklight picks up an okashi-written manifest.

- [ ] **Step 1: Find every "okashi never writes" assertion in both repos**

Run:
```bash
grep -rn "never writes\|okashi reads\|only writers\|ManuscriptStore\|wicklight owns it\|manifest is managed" /Users/michael/dev/okashi/CLAUDE.md /Users/michael/dev/inkmere/SPEC.md /Users/michael/dev/inkmere/docs/superpowers/specs/2026-06-27-multi-source-library-design.md /Users/michael/dev/inkmere/docs/superpowers/specs/2026-06-27-project-model-design.md
```
Record each hit; each must be reconciled below.

- [ ] **Step 2: Update okashi `CLAUDE.md`**

In the Project model section and SHARED CONTRACTS §1, change the authority lines. The manuscript bullet currently reads "okashi reads it; **wicklight owns it**" and §1 says okashi "reads this and **never writes it**." Replace with the spec §0 model:
- okashi **is a manifest writer**: create (New Project) and chapter-title retitle are **no-confirm**, allowed on any source including the wicklight-shared corpus, safe via atomic-write + `NSFileVersion` + read-modify-write.
- Structural edits (reorder/insert/remove/move) are **confirm-gated** and NOT yet shipped (a later cycle / structuring mode).
- Keep the **schema HARD-GATE**: any change to manifest *shape* still STOPS and updates both repos.

Keep the "MIRROR THIS BLOCK IN `../inkmere`" instruction. Edit the mirrored block to the new authority text.

- [ ] **Step 3: Update wicklight docs to the identical authority**

In each wicklight file from Step 1, replace "okashi never writes manifests; the only writers are wicklight's `ManuscriptStore`" (and equivalents) with the reciprocal: okashi may write manifests (create + chapter-title retitle no-confirm; structural edits confirm-gated), the shared safety model is atomic-write + `NSFileVersion`, wicklight reloads externally-changed manifests via its file-presenter and rebuilds its per-source index. Mirror the exact authority wording used in okashi's `CLAUDE.md`.

- [ ] **Step 4: Verify the round-trip**

With a wicklight-owned manuscript folder (or a hand-written v1 manifest), run okashi, create/retitle, then confirm:
```bash
# okashi writes a manifest okashi itself re-reads cleanly:
/opt/homebrew/bin/go test ./... -run 'Manuscript|ChapterTitle' -v
```
Then manually: point okashi at a folder wicklight manages, do a `r` retitle, and confirm wicklight (its resolver/index) reflects the new title on next read. Note the result in the commit body. If wicklight does NOT reload, STOP and escalate — that is a contract gap, not a doc fix.

- [ ] **Step 5: Commit (okashi) and commit (wicklight) separately**

```bash
git -C /Users/michael/dev/okashi add CLAUDE.md
git -C /Users/michael/dev/okashi commit -m "docs: okashi is a manifest writer (create/retitle) — mirror the reversed authority"
git -C /Users/michael/dev/inkmere add SPEC.md docs/superpowers/specs/2026-06-27-multi-source-library-design.md docs/superpowers/specs/2026-06-27-project-model-design.md
git -C /Users/michael/dev/inkmere commit -m "docs: okashi may write manifests (create/retitle no-confirm, structural confirm) — mirror okashi CLAUDE.md §1"
```

---

## Self-Review

**Spec coverage (against `2026-06-30-okashi-library-sources-manifest-design.md` §8.1):**
- Manifest writers `writeManifest`/`createManuscript`/`renameChapterTitle` → Task 1. ✅ (spec §3)
- New Project = folder+manifest+first chapter, opens it → Task 2. ✅ (spec §5.2)
- `r` retitle for manifest chapters, remove the block → Task 3. ✅ (spec §5.7)
- Any source incl. wicklight (no source gating) → Tasks 2/3 use plain paths, no source filter. ✅ (spec §0)
- File-pane label (folded-in consideration) → Task 4. ✅
- Both-repos doc update + verify wicklight reload → Task 5. ✅ (spec §0 HARD-GATE)
- NOT in this step (correctly deferred): sources model / `sources.json` (step 2), home Recent-on-top + inline `+` + trailing-slash (step 3), structural edits / structuring mode / file mover (roadmap).

**Type consistency:** `createManuscript` returns `(string, error)` in Task 1 and is consumed as `first, err :=` in Task 2 — matches. `renameTarget.manifestChapter` defined in Task 3 Step 1, read in Task 3 Step 6 — matches. `paneLabel()` defined Task 4 Step 3, used Task 4 Step 4 — matches. `renameChapterTitle(dir, file, newTitle)` signature identical across Tasks 1, 3, 5.

**Placeholder scan:** no TBD/"handle errors"/"similar to" — every code step is complete. Two tasks (2, 4) reference "mirror the existing test setup helper" for model/filelist construction rather than inventing one — that is a deliberate instruction to follow the codebase's established test pattern, with the exact file/line to copy from, not a placeholder.

**Open follow-through for the executor:** Tasks 2 and 4 depend on an existing test-model / filelist construction pattern (`smoke_test.go:376`, `filelist_test.go:158`). Confirm those helpers before writing the tests; if the model constructor differs, adapt the two new tests to match — the assertions stay identical.
