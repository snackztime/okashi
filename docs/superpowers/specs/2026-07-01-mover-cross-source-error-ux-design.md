# Mover fast-follow: cross-source destinations + persistent error UX — design

**Date:** 2026-07-01
**Status:** Approved (direction)
**Context:** The file mover shipped v1 as contextual entry + standalone source picker, **same-source
moves only**, with a **transient** error (`m.status`, overwritten on the next keystroke, and the
mover closes on failure). Two fast-follows, approved together:
1. **Cross-source destinations** — let the destination browser reach any library source (Option A:
   a "Sources" level above the source roots).
2. **Persistent move-error UX** — on failure keep the mover open with a sticky error until the user
   navigates, retries, or cancels.

No new shared-contract surface (moves are path-based; manifests already update on manuscript
boundaries via the chunk-1 engine). Build order: engine safety (moveFolder cross-volume message) →
cross-source browsing → persistent error UX.

---

## 1. Cross-volume folder move — a clear error (engine)

`safeMove` (used by `moveDocument`) already falls back to copy+remove on `EXDEV`, so **files move
across volumes**. `moveFolder` uses a raw `os.Rename`, which fails with a cryptic
`rename ...: cross-device link` when the destination is on a different volume (a real possibility
once destinations can be other sources, e.g. iCloud ↔ local).

**Change (`move.go`, `moveFolder`):** detect `EXDEV` and return a clear, user-facing message
instead of the raw syscall error. No recursive dir-copy in v1 (deferred).

```go
import "syscall" // already imported by safeMove; "errors" too

func moveFolder(srcDir, dstParent string) error {
	base := filepath.Base(srcDir)
	dst := filepath.Join(dstParent, base)
	if srcDir == dst {
		return fmt.Errorf("%s already lives there", base)
	}
	if withinRoot(dstParent, srcDir) {
		return fmt.Errorf("can't move a folder into itself")
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", base)
	}
	if err := os.Rename(srcDir, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("can't move a folder across volumes yet — move its files individually")
		}
		return err
	}
	return nil
}
```

**Test (`move_test.go`):** a same-volume folder move still succeeds (regression) and returns nil;
the collision / into-itself / no-op guards still fire. (EXDEV itself isn't simulable in a unit test
without two volumes; the friendly-message branch is covered by inspection.)

## 2. Cross-source destinations — a "Sources" level (Option A)

**Model.** Add nothing but reuse `m.sources` (the library source list) and `activeSourceRoot()`.
The sentinel **`m.moverDestDir == ""`** means "showing the sources list".

**New entry kind (`mover.go`):** `moverSource` — a row that is a library source root.
```go
const (
	moverMoveHere moverEntryKind = iota
	moverUp
	moverFolder
	moverFile
	moverMoveThis
	moverSource // a library source root (destination target)
)
```

**Helper (`mover.go`):** the source that bounds a destination dir, for `..` bounding + the header.
```go
// moverBoundingSource returns the reachable library source whose root contains (or equals) dir.
func (m model) moverBoundingSource(dir string) (source, bool) {
	for _, s := range m.sources {
		if !s.reachable() {
			continue
		}
		r := s.root()
		if dir == r || withinRoot(dir, r) {
			return s, true
		}
	}
	return source{}, false
}
```

**`moverReload()` rewrite:**
```go
func (m *model) moverReload() {
	var rows []moverEntry
	if m.moverDestDir == "" {
		// The sources list: one row per reachable source; no "move here", no "..".
		for _, s := range m.sources {
			if s.reachable() {
				rows = append(rows, moverEntry{name: s.Name, path: s.root(), kind: moverSource})
			}
		}
	} else {
		rows = append(rows, moverEntry{name: filepath.Base(m.moverDestDir), path: m.moverDestDir, kind: moverMoveHere})
		// ".." goes to the parent, bounded by the source root; at a source root it goes to the sources list ("").
		if src, ok := m.moverBoundingSource(m.moverDestDir); ok {
			up := filepath.Dir(m.moverDestDir)
			if m.moverDestDir == src.root() {
				up = "" // step up to the sources list
			}
			rows = append(rows, moverEntry{name: "..", path: up, kind: moverUp})
		}
		if ents, err := os.ReadDir(m.moverDestDir); err == nil {
			var dirs []moverEntry
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, moverEntry{name: e.Name(), path: filepath.Join(m.moverDestDir, e.Name()), kind: moverFolder})
				}
			}
			sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
			rows = append(rows, dirs...)
		}
	}
	m.moverEntries = rows
	if m.moverSel >= len(rows) {
		m.moverSel = len(rows) - 1
	}
	if m.moverSel < 0 {
		m.moverSel = 0
	}
}
```
Note: contextual/standalone entry still defaults `moverDestDir = activeSourceRoot()` — the common
case is unchanged; the sources list is only reached by `..` from a source root.

**`updateMover` pick-dest `enter` (add the `moverSource` case; clear the error on nav):**
```go
case "enter":
	if m.moverSel < 0 || m.moverSel >= len(m.moverEntries) {
		return m, nil
	}
	e := m.moverEntries[m.moverSel]
	switch e.kind {
	case moverUp, moverFolder, moverSource:
		m.moverError = ""       // navigation dismisses a sticky error
		m.moverDestDir = e.path // moverUp may set "" (sources list); moverSource sets a source root
		m.moverSel = 0
		m.moverReload()
	case moverMoveHere:
		m.moverAsChapter = true
		m.moverConfirm = true
	}
	return m, nil
```

**`moverView` right pane (header + the `moverSource` row):**
- Header: `TO · SOURCES` when `m.moverDestDir == ""`; else `TO · <SourceName>` at a source root, or
  `TO · <SourceName>/<base(dir)>` deeper (source name from `moverBoundingSource`).
- Row text for `moverSource`: `"◆ " + e.name` (distinct from `▸ ` folders). Selected row keeps the
  existing `selectedStyle`.
- Footer gains a hint: `↑↓ browse · enter drill/select · .. → sources · esc cancel`.

**Move execution is unchanged** — `applyMove` calls `moveDocument`/`moveFolder` with absolute
paths, so a cross-source destination is just a different path. Files cross volumes (safeMove);
cross-volume folders return the clear error from §1, surfaced by §3.

### Tests (`mover_test.go`)
- `moverBoundingSource`: returns the primary for a dir at/under the primary root; a folder source
  for a dir under it; `ok=false` for an unrelated path.
- `moverReload` with `moverDestDir == ""` on a model with 2 reachable sources → 2 `moverSource`
  rows, no `moverMoveHere`/`moverUp`.
- `moverReload` at a source root → first row `moverMoveHere`, second row `moverUp` with `path == ""`.
- `moverReload` one level below a source root → `moverUp.path == filepath.Dir(dir)` (not "").

## 3. Persistent move-error UX

**Model field (`main.go`, beside the other `mover*` fields):** `moverError string`.

**`updateMover` confirm branch (`y`/`enter`) — stay open on failure:**
```go
case "y", "enter":
	if err := m.applyMove(); err != nil {
		m.moverError = "move failed: " + err.Error()
		m.moverConfirm = false
		m.moverReload() // nothing moved; refresh the destination view
		return m, nil   // STAY in the mover so the error is visible
	}
	m.moverError = ""
	m.status = "moved " + filepath.Base(m.moverSource)
	m.moverConfirm = false
	m.files.SetDir(m.files.dir)
	m.screen = m.moverReturn
	return m, nil
```

**Clear the error** on any navigation (the pick-dest `enter` above already does; also clear on
pick-source navigation and on the pick-source `esc`), and reset it fresh in `enterMover` /
`enterMoverStandalone` (`m.moverError = ""`).

**`moverView` — a sticky error line** (shown when `m.moverError != ""` and not confirming), in
place of the footer:
```go
if m.moverError != "" && !m.moverConfirm {
	errLine := lipgloss.NewStyle().Foreground(errColor).Render("⚠ " + m.moverError + "   (browse to dismiss · esc cancel)")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, errLine))
	return b.String()
}
// …existing footer…
```

**Style (`styles.go`):** promote the existing red to a semantic name:
```go
errColor = lipgloss.Color("#ff5555") // red (Dracula) — used by iconPdfColor too
```
(Keep `iconPdfColor` pointing at the same value, or set `iconPdfColor = errColor`.)

### Tests (`mover_test.go`)
- A failing move keeps the mover open with the error set: construct a model in pick-dest confirm
  with a source whose move will fail (a name collision at the destination — create the colliding
  file in a temp dir), send `enter`, and assert `m.moverError != ""`, `m.screen` unchanged (still
  `screenMover`), and `m.moverConfirm == false`.
- A subsequent navigation `enter` (into a folder) clears `m.moverError`.

## 4. Out of scope
- Recursive dir-copy for cross-volume folder moves (v1 shows the clear error instead).
- Moving multiple items at once; drag-and-drop.
- Any manifest-shape change (moves are path-based; the chunk-1 engine already updates manifests on
  manuscript boundaries).

## 5. Sequencing (for the plan)
1. **Engine message** — `moveFolder` EXDEV → clear error (`move.go`) + regression test.
2. **Cross-source browsing** — `moverSource` kind, `moverBoundingSource`, `moverReload` rewrite,
   `updateMover` enter case, `moverView` header/row/footer (`mover.go`) + reload tests.
3. **Persistent error** — `moverError` field, confirm-branch stay-open, clear-on-nav, sticky error
   line, `errColor` (`main.go`/`mover.go`/`styles.go`) + behavior test.
