# Corkboard: full-screen only Implementation Plan

> **For agentic workers:** executed inline, task-by-task with TDD, one commit per task on a feature branch, adversarial review before merge. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Remove the in-pane corkboard mode; make the corkboard a full-screen-only surface reached by `ctrl+k` (and `c`), and enrich it with a manuscript progress header, a "you-are-here" marker on the open chapter, and a dimmed first-line synopsis fallback.

**Design (settled in conversation):** The left pane goes back to being a clean, fast chapter list. The corkboard — synopses + reorder + add/remove/retitle — is the one dedicated full-screen planning surface. This removes the pane's card cramping, the nav/render order mismatch, and the pane-reorder modal complexity, and it deletes code. The outline (`ctrl+l`, free-form `outline.md`) stays a separate surface; an outline→chapter "promote a bullet" bridge is parked as a follow-on.

**Tech Stack:** Go 1.25 (`/opt/homebrew/bin/go`), Bubble Tea / lipgloss, vendored `internal/textarea`. Flat `package main`.

## Global Constraints

- Invoke Go as `/opt/homebrew/bin/go` (not on PATH).
- `View()` stays O(visible) — the corkboard already windows via `homeWindowOffset`; keep it that way.
- No manifest **shape** change (shared-contract HARD GATE untouched). Synopses stay in `.okashi-synopsis.json`; order stays in `manifest.json`.
- Atomic writes only (already true via `saveSynopses`/`writeManifest`).
- Keep it lean: no phantom/placeholder cards, no per-card status/labels.
- The full-screen corkboard is **manifest-only** (it edits `manifest.json`). Legacy numbered manuscripts (no manifest) do **not** get a corkboard — `ctrl+k` shows a clear status note. This is an accepted, documented behavior change (legacy is a read-only transitional courtesy).

---

### Task 1: Point `ctrl+k` at the full-screen corkboard

**Files:**
- Modify: `main.go` (the `ctrl+k` case, ~1486–1490)
- Modify: `corkboard.go` (`enterCorkboard` legacy message, ~17–23)
- Test: `corkboard_test.go`

**Interfaces:**
- Consumes: `enterCorkboard()` (existing, `corkboard.go:17`) — sets `m.screen = screenCorkboard` when a manifest is present.
- Produces: `ctrl+k` opens the full-screen corkboard for a manifest manuscript; a clear status note otherwise.

- [ ] **Step 1: Write the failing test** — in `corkboard_test.go`, add a test that a model whose `files.dir` is a manifest manuscript enters `screenCorkboard` via the `ctrl+k` path. Use the existing corkboard test helpers/fixtures (mirror how `corkboard_test.go` already builds a manuscript temp dir). Assert `m.screen == screenCorkboard` after dispatching `tea.KeyMsg{Type: tea.KeyCtrlK}` through `Update` from the writing screen with focus on the sidebar.

- [ ] **Step 2: Run it, expect FAIL** (ctrl+k still toggles the pane).
Run: `/opt/homebrew/bin/go test ./... -run Corkboard`

- [ ] **Step 3: Rewire `ctrl+k`.** Replace the body of the `case "ctrl+k":` block in `main.go`:
```go
		case "ctrl+k":
			// The corkboard is full-screen: ctrl+k (and `c` from the sidebar) open it.
			m.enterCorkboard()
			return m, nil
```
Improve `enterCorkboard`'s legacy/no-manifest message (`corkboard.go`) so it reads as navigation guidance, not a reorder error, **and flush the outgoing buffer on entry** (mirrors `ctrl+l`; closes a data-loss hole — opening the currently-open chapter from the board takes `loadFile`'s `currentFile == path` branch, which skips the save and reloads from disk, clobbering unsaved edits):
```go
func (m *model) enterCorkboard() {
	m.save() // flush the current buffer before the board can reload a chapter over it
	dir := m.files.dir
	sm, present, err := readManifest(dir)
	if !present || err != nil {
		m.status = "corkboard needs a manifest — legacy numbered manuscripts show as a plain list"
		return
	}
```
(`save()` no-ops when not dirty / no current file, so it's safe on every entry.)

- [ ] **Step 4: Run the test, expect PASS.**
Run: `/opt/homebrew/bin/go test ./... -run Corkboard`

- [ ] **Step 5: Commit** — `git commit -am "corkboard: ctrl+k opens the full-screen board"`

---

### Task 2: Remove the in-pane corkboard entirely

**Files:**
- Delete: `pane_corkboard.go`, `filelist_corkboard.go`, `pane_corkboard_test.go`, `filelist_corkboard_test.go`
- Modify: `filelist.go` (drop `corkMode`, `synopses`, `proseCache` fields + their `SetDir` loads + the `corkView` branch in the render)
- Modify: `main.go` (drop `paneReorderDirty`/`paneReorderConfirm`/`paneSynEditing` fields; the three modal blocks ~938–996; the sidebar `e`/`J`/`K` bindings ~1637–1643; the `paneSynEditing` view block ~1812; the `m.files.corkMode` click-guard clause ~1375)
- Modify: `export.go` (drop the `m.files.corkMode && …` clause, line 13)
- Test: whole suite must stay green

**Interfaces:**
- Consumes: nothing new.
- Produces: the pane is a plain chapter list again; `synEditing`/`structure*` (full-screen corkboard state) are untouched. `firstProseLine` (synopsis.go) survives — Task 4 reuses it.

- [ ] **Step 1: Delete the four files.**
```bash
git rm pane_corkboard.go filelist_corkboard.go pane_corkboard_test.go filelist_corkboard_test.go
```

- [ ] **Step 2: Strip `filelist.go`.** Remove the `corkMode`, `synopses`, and `proseCache` struct fields; remove `f.synopses = loadSynopses(dir)` and `f.proseCache = map[string]string{}` in `SetDir`; remove the render branch:
```go
	if f.corkMode && f.view.ordered() && editRow == -1 {
		return f.corkView()
	}
```

- [ ] **Step 3: Strip `main.go`.** Remove the `paneReorderDirty`, `paneReorderConfirm`, `paneSynEditing` model fields; delete the `if m.paneSynEditing { … }`, `if m.paneReorderConfirm { … }`, and `if m.paneReorderDirty { … }` blocks in `Update`; delete the sidebar `case "e":` (startPaneSynopsis), `case "J", "shift+down":` and `case "K", "shift+up":` (paneReorder) bindings; delete the `if m.paneSynEditing { … }` view block; and drop the `m.files.corkMode ||` clause from the click guard so it reads:
```go
		if !inSidebar || msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
			return m, nil
		}
```

- [ ] **Step 4: Strip `export.go`.** The whole-manuscript scope is now just the corkboard screen:
```go
func (m model) exportIsWholeManuscript() bool { // keep the real name/signature from the file
	return m.screen == screenCorkboard
}
```
(Preserve the existing function name/comment; only remove the `|| (m.files.corkMode && m.files.view.ordered())` term.)

- [ ] **Step 5: Build + full test suite, expect PASS/GREEN.**
Run: `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./...`
Expected: compiles clean (no unused-symbol errors), all tests pass. Fix any now-orphaned references the grep missed.

- [ ] **Step 6: Commit** — `git commit -am "corkboard: remove the in-pane corkboard mode"`

---

### Task 3: Manuscript progress header on the corkboard

**Files:**
- Modify: `corkboard.go` (add `corkboardStatusLine`; render it above the cards in `corkboardView`)
- Test: `corkboard_test.go`

**Interfaces:**
- Consumes: `m.structureItems` ([]manifestItem), `m.structureDir`, `m.files.wc` (`*wordCountCache`), `m.goalsAll[m.structureDir].applyEnvDefaults()` (`projectGoals` → `.ProjectGoal`, `.Deadline`), `commafy`, `strconv`.
- Produces: `corkboardStatusLine(items, dir, wc, pg) string`.

- [ ] **Step 1: Write the failing test** for `corkboardStatusLine`:
```go
func TestCorkboardStatusLine(t *testing.T) {
	items := []manifestItem{{File: "a.md"}, {File: "b.md"}}
	// wc == nil → total counts as 0; still reports chapter count.
	got := corkboardStatusLine(items, "/x", nil, projectGoals{})
	if !strings.Contains(got, "2 chapters") {
		t.Fatalf("want chapter count, got %q", got)
	}
	if strings.Contains(got, "/") {
		t.Fatalf("no goal set → no target fragment, got %q", got)
	}
	withGoal := corkboardStatusLine(items, "/x", nil, projectGoals{ProjectGoal: 80000, Deadline: "2026-03-01"})
	if !strings.Contains(withGoal, "/ 80,000") || !strings.Contains(withGoal, "by 2026-03-01") {
		t.Fatalf("want target + deadline, got %q", withGoal)
	}
	one := corkboardStatusLine(items[:1], "/x", nil, projectGoals{})
	if !strings.Contains(one, "1 chapter ") {
		t.Fatalf("want singular 'chapter', got %q", one)
	}
}
```

- [ ] **Step 2: Run it, expect FAIL** (undefined `corkboardStatusLine`).
Run: `/opt/homebrew/bin/go test ./... -run CorkboardStatusLine`

- [ ] **Step 3: Implement** in `corkboard.go`:
```go
// corkboardStatusLine summarizes the manuscript above the cards: chapter count, total words,
// and — when a project goal is set — progress toward it with an optional deadline.
func corkboardStatusLine(items []manifestItem, dir string, wc *wordCountCache, pg projectGoals) string {
	total := 0
	if wc != nil {
		for _, it := range items {
			total += wc.count(filepath.Join(dir, it.File))
		}
	}
	unit := "chapters"
	if len(items) == 1 {
		unit = "chapter"
	}
	line := strconv.Itoa(len(items)) + " " + unit + " · " + commafy(total) + " words"
	if pg.ProjectGoal > 0 {
		line += " · " + commafy(total) + " / " + commafy(pg.ProjectGoal)
		if pg.Deadline != "" {
			line += " by " + pg.Deadline
		}
	}
	return line
}
```
Add `"strconv"` to the imports. In `corkboardView`, render the line centered above the board (dim), and reduce the board height budget by its row so cards still fit:
```go
	hdr := lipgloss.NewStyle().Foreground(subtle).Render(
		corkboardStatusLine(m.structureItems, m.structureDir, m.files.wc, m.goalsAll[m.structureDir].applyEnvDefaults()))
	b.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, hdr) + "\n")
	b.WriteString(lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, board))
```
(Change the existing `lipgloss.Place(m.width, m.height-1, …)` to `m.height-2` to account for the header row, and account for the extra row in the `vis` calc: change `vis := (m.height - 4) / perCard` to `(m.height - 5) / perCard`.)

- [ ] **Step 4: Run the test, expect PASS.**
Run: `/opt/homebrew/bin/go test ./... -run CorkboardStatusLine`

- [ ] **Step 5: Build + full suite green.**
Run: `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...`

- [ ] **Step 6: Commit** — `git commit -am "corkboard: progress header (chapters · words · target)"`

---

### Task 4: "You-are-here" marker + dimmed first-line synopsis fallback

**Files:**
- Modify: `corkboard.go` (`enterCorkboard` preload; `corkboardView` card build)
- Modify: `main.go` (add the `corkFirstLines map[string]string` model field)
- Test: `corkboard_test.go`

**Interfaces:**
- Consumes: `m.currentFile` (absolute path of the open file), `firstProseLine(path) string` (synopsis.go), `m.corkFirstLines` (preloaded fallback map).
- Produces: the card for the currently-open chapter shows an "open" marker; a chapter with no authored synopsis shows its dimmed first prose line instead of "(no synopsis — e to add)".

**Performance note:** `firstProseLine` is disk I/O and MUST NOT run in the render path — `corkboardView` fires on every keystroke, and on an iCloud corpus a not-yet-materialized file read can stall. Preload the fallbacks **once** in `enterCorkboard` (next to `loadSynopses`), into `m.corkFirstLines`, and have the card loop read from that map only. This keeps `View()` I/O-free (matching the `proseCache` guarantee Task 2 removed).

- [ ] **Step 1: Write the failing test.** Add a pure `corkboardCardMeta(it manifestItem, isCurrent bool, syn, firstLine string) (openMark string, body string, dim bool)` helper and test it: (a) `isCurrent == true` → `openMark` non-empty (contains the open glyph); `isCurrent == false` → `openMark == ""`; (b) `syn == ""`, `firstLine == "The sea..."` → `dim == true`, body contains the first line; (c) `syn != ""` → `dim == false`, body is the synopsis; (d) `syn == "" && firstLine == ""` → body is the `(no synopsis — e to add)` placeholder.

- [ ] **Step 2: Run it, expect FAIL** (undefined helper).
Run: `/opt/homebrew/bin/go test ./... -run CorkboardCard`

- [ ] **Step 3: Implement.**
  - Add `corkFirstLines map[string]string` to the model struct (`main.go`).
  - In `enterCorkboard`, after `m.synopses = loadSynopses(dir)`, preload: `m.corkFirstLines = map[string]string{}; for _, it := range sm.Items { if m.synopses[it.File] == "" { m.corkFirstLines[it.File] = firstProseLine(filepath.Join(dir, it.File)) } }`.
  - Add the pure `corkboardCardMeta` helper (marker precedence: **selection keeps the `▸ ` prefix slot; the open-chapter marker is a separate accent `● ` inserted before the title**, so a card that is both selected and current shows `▸ 1 · ● Title` — they never contend for the same column).
  - Wire it into `corkboardView`'s card loop, passing `m.corkFirstLines[it.File]` as `firstLine` (never calling `firstProseLine` from the loop). Style the fallback body dim; style `● ` with `accent`.

- [ ] **Step 4: Run the test, expect PASS.**
Run: `/opt/homebrew/bin/go test ./... -run CorkboardCard`

- [ ] **Step 5: Build + full suite green.**
Run: `/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./...`

- [ ] **Step 6: Commit** — `git commit -am "corkboard: open-chapter marker + first-line synopsis fallback"`

---

### Task 5: Docs + in-app help

**Files:**
- Modify: `main.go` (help text ~46, ~52–53)
- Modify: `README.md` (`ctrl+k` shortcut row ~77–78; the "The corkboard" section ~136–159)

**Interfaces:** none (copy only).

- [ ] **Step 1: Update `main.go` help strings.** `ctrl+k corkboard` should read as the full-screen board; drop "corkboard view / c full-screen" toggle language and the pane `J/K` reorder line (reorder now lives in the board). Reflect: `ctrl+k`/`c` open the corkboard; inside it `J/K` reorder, `e` synopsis, `a` add, `x` remove, `r` retitle, `⏎` open.

- [ ] **Step 2: Update `README.md`.** Change the shortcut row to `| ctrl+k | Corkboard (full-screen manuscript navigator) |` and drop the separate `c` row (or note it as the sidebar alias). Rewrite the "The corkboard" section: the left pane is the chapter list; `ctrl+k` (or `c`) opens the full-screen corkboard — cards with word count + synopsis (dimmed first line until you author one) + progress header; `J/K` reorder (staged, commit-on-exit), `e` synopsis, `a` add/promote, `x` remove, `r` retitle, `⏎` open, `ctrl+e` export, `m` pager. Note the outline (`ctrl+l`) is a separate free-form planning doc.

- [ ] **Step 3: Build (docs-only, but confirm nothing references removed help symbols).**
Run: `/opt/homebrew/bin/go build ./...`

- [ ] **Step 4: Commit** — `git commit -am "docs: corkboard is full-screen only"`

---

## After all tasks

- Adversarial self-review of the whole branch (fresh-eyes pass: dead code, orphaned state, help/README drift, O(visible) preserved, legacy-manuscript path).
- `finishing-a-development-branch`: verify `/opt/homebrew/bin/go test ./...` green, then present merge options.
