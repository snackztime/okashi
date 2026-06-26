# Atomic writes: design

**Date:** 2026-06-26
**Status:** Approved (pending spec review)
**Context:** Honors the `CLAUDE.md` invariant "write atomically (temp file + rename); never
write in place." okashi runs outside the macOS sandbox (no `NSFileCoordinator`), so atomic
writes are how it avoids corrupting the iCloud-synced shared corpus.

## Goal

Replace every in-place `os.WriteFile` in okashi with a single atomic helper that writes to a
temp file in the same directory, fsyncs it, then renames it over the target — so a crash or
a concurrent reader never sees a truncated file.

## The helper

`func atomicWrite(path string, data []byte, perm os.FileMode) error`:
1. `tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")` — temp
   in the **same directory** (so the final `os.Rename` is a same-volume atomic operation),
   **dot-prefixed** (hidden from the file pane; never mistaken for a section/loose file).
2. `defer os.Remove(tmp.Name())` — cleans up the temp on any error path; a harmless no-op
   after a successful rename.
3. Write `data`; on error, close and return.
4. `tmp.Sync()` (fsync — bytes durable before the rename), then `tmp.Close()`.
5. `os.Chmod(tmp.Name(), perm)` (preserve the intended mode).
6. `os.Rename(tmp.Name(), path)` — atomic replace.

Errors from any step return immediately; the original file at `path` is untouched until the
final rename, and the temp is removed.

## Call sites (replace `os.WriteFile` → `atomicWrite`)

- `main.go` `save()` — the editor save (the prose; highest value).
- `export.go` — the RTF and PDF writes.
- `backup.go` `backupFiles` — the snapshot copies.
- `outline.go` `commitInsert` — the new empty section file.
- `recent.go` — the recents store.

**Leave as-is:** the existing `os.Rename` two-phase renumber (`applyRenames`) and the rename
feature are already atomic per file.

## Testing

- `atomicWrite` creates a new file with the exact content and the given mode (`0o644`).
- Overwriting an existing file replaces it; reading the target during/after never yields a
  truncated/partial file (assert content == new bytes; and that on a forced mid-write failure
  the original bytes survive).
- No temp file is left in the directory after a success, and none after an error.
- The temp is created in the target's own directory (cross-volume-safe rename).

## Out of scope

- Directory fsync (the rename's parent-dir durability) — fsyncing the file is the pragmatic
  bar; add dir-fsync later only if a real durability gap shows up.
- The shared-corpus iCloud `NSFileVersion` integration (that's the macOS app's side).
