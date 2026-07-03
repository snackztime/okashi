# Tier-0 data-safety — design

**Date:** 2026-07-03
**Status:** Approved (direction) — from the 2026-07-03 full review; the two critical bugs are code-verified.
**Context:** okashi builds serious writing tooling on a save model that can silently drop work. The
review found (and the controller verified in-code) two CRITICAL silent-data-loss paths, plus two
adjacent gaps. This batch closes them. Autosave only flushes after **2 s idle** (`autosaveDue`,
main.go:462), so any path that exits or clobbers the buffer inside that window loses the last burst.

Build order = severity: save-on-quit → save-on-switch → backup-before-save → external-change guard.

---

## 1. Save on quit (CRITICAL — verified)

**Bug:** the writing-screen `ctrl+c` is `return m, tea.Quit` (main.go:1211) — no save. Every other
`tea.Quit` site (modals, home, etc.) is the same. Quit right after typing → the last burst is gone.

**Fix:** a single **global `ctrl+c` intercept** placed before the screen dispatch (next to the global
help handler already there), so it fires from every screen/state and the per-site `ctrl+c` handlers
become dead:
```go
// Global quit: always flush unsaved work first.
if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
	m.saveIfDirty()
	return m, tea.Quit
}
```
Helper (also flushes the goals/active-time store — review risk #9, ≤60 s otherwise lost on quit):
```go
func (m *model) saveIfDirty() {
	if m.dirty && m.currentFile != "" {
		m.save()
	}
	if m.goalsAll != nil {
		saveGoals(goalsPath(), m.goalsAll)
	}
}
```
The existing `case "ctrl+c": return m, tea.Quit` sites are now unreachable; leave them (harmless) or
delete in passing. The help-overlay dismiss block's `ctrl+c` is superseded by the global one.

## 2. Save on switch (CRITICAL — verified)

**Bug:** `loadFile` does `SetValue(...)` then `m.dirty = false` with **no save of the outgoing
buffer** (main.go ~1859); its callers (sidebar double-click 1197 / Enter 1399, pager 1770/1800,
outline 1680/1707) don't save first. Open A, type, switch to B within 2 s → A's edits are gone.

**Fix:** one guard at the top of `loadFile` covers every caller:
```go
func (m *model) loadFile(path string) {
	if m.dirty && m.currentFile != "" && m.currentFile != path {
		m.save() // flush the outgoing buffer before clobbering it
	}
	data, err := os.ReadFile(path)
	// … unchanged …
}
```

## 3. Backup-before-save (escape hatch for accidental overwrite)

**Gap:** no backup/snapshot exists; an accidental select-all-delete + autosave, or a bad
spell-replace, is unrecoverable (no undo either — that's a separate spec).

**Design — one snapshot per file per app-session** (NOT every save; autosave fires every ~2 s):
the first time a file is saved in this run, copy its **prior on-disk** version (the state as you
opened it) into a hidden sibling, then let the save proceed. Cheap (one extra write per file per
session), and gives a "restore to how it was when I opened it" hatch; because most users restart
daily, it also yields a rough daily snapshot.
```go
// model field:
backedUp map[string]bool // files snapshotted this session

// in save(), before atomicWrite:
if m.backedUp == nil {
	m.backedUp = map[string]bool{}
}
if !m.backedUp[m.currentFile] {
	backupBeforeSave(m.currentFile) // best-effort; never blocks the save
	m.backedUp[m.currentFile] = true
}

// backupBeforeSave copies the current on-disk file into <dir>/.okashi-bak/<base> (dot-prefixed →
// excluded from the pane + manuscript detection). Best-effort: any error is ignored.
func backupBeforeSave(path string) {
	data, err := os.ReadFile(path)
	if err != nil { // new file or unreadable → nothing to back up
		return
	}
	bakDir := filepath.Join(filepath.Dir(path), ".okashi-bak")
	if os.MkdirAll(bakDir, 0o755) != nil {
		return
	}
	_ = atomicWrite(filepath.Join(bakDir, filepath.Base(path)), data, 0o644)
}
```
*(A timestamped multi-version ring — real version history — is a deliberate Tier-2 follow-up; this
is the minimal, lean escape hatch.)*

## 4. External-change guard (iCloud / other-device lost-update)

**Gap:** `save()` overwrites unconditionally. On the shared iCloud corpus, if the companion app or
another device syncs a newer version while okashi holds the file open, the next save silently
clobbers it (lost update). No `loadedMtime` exists today.

**Design — never overwrite an externally-changed file; divert to a conflict copy (no modal):**
- Add `loadedMtime map[string]time.Time`. In `loadFile`, record `os.Stat(path).ModTime()`.
- In `save()`, before writing `currentFile`: `os.Stat` it; if its mtime is **after** the recorded
  load mtime, the file changed underneath us. Instead of overwriting, write the buffer to a sibling
  `<name>.conflict-<YYYYMMDD-HHMMSS>.md`, **repoint `m.currentFile` at that conflict file** (so the
  session continues on your divergent copy and the external version is left intact), record the new
  file's mtime, clear `dirty`, and set a status warning: `⚠ <base> changed on disk — your edits are
  now in <conflict base>`. Both versions are preserved; the user reconciles manually.
- On a normal (unchanged) save, update `loadedMtime[currentFile]` to the just-written file's mtime.

**Tests:**
- save-on-quit: a dirty buffer + the global ctrl+c path calls save (buffer flushed to disk).
- save-on-switch: dirty A, `loadFile(B)` writes A first, then loads B.
- backup: first save of an existing file creates `.okashi-bak/<base>` with the pre-edit content;
  a second save does not re-snapshot.
- external-change: a save where on-disk mtime advanced writes a `.conflict-*.md` and does NOT
  overwrite the original; `currentFile` repoints; the original on disk is unchanged.

## Out of scope (separate specs / tiers)
- **Undo/redo** (Tier-2; the vendored textarea has no undo stack — a checkpoint ring is its own design).
- Timestamped multi-version snapshot history + a manual `b` backup key (Tier-2).
- A Trash-instead-of-delete for `confirmDelete` (Tier-3 / review risk #7).
- `commitStructure` / `moveDocument` operation-ordering hardening (review risks #5/#6 — Medium; a
  small follow-up: write the manifest first / reorder the move so a partial failure stays consistent).

## Sequencing (for the plan)
1. **Save on quit** — `saveIfDirty` + global ctrl+c intercept (main.go). [P0]
2. **Save on switch** — `loadFile` guard (main.go). [P0]
3. **Backup-before-save** — `backedUp` map + `backupBeforeSave` in `save()` (main.go).
4. **External-change guard** — `loadedMtime` + conflict-divert in `save()`/`loadFile` (main.go).
