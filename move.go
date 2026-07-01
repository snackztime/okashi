package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// moveDocument relocates document `file` from srcDir to dstDir. It moves the file, removes it from
// the source manifest if it was a chapter there, and — if dstDir is a manuscript and asChapter is
// true — appends it to the destination manifest; otherwise it lands as a loose Resource. Refuses a
// no-op (same folder), a destination name collision, and an unreadable manifest on either side.
func moveDocument(srcDir, file, dstDir string, asChapter bool) error {
	if srcDir == dstDir {
		return fmt.Errorf("source and destination are the same folder")
	}
	// Refuse to touch a manuscript whose manifest is present-but-unreadable (don't guess).
	if _, present, err := readManifest(srcDir); present && err != nil {
		return fmt.Errorf("source manifest unreadable: %w", err)
	}
	if _, present, err := readManifest(dstDir); present && err != nil {
		return fmt.Errorf("destination manifest unreadable: %w", err)
	}
	dst := filepath.Join(dstDir, file)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", file)
	}

	// Was it a listed chapter of the source manuscript?
	wasChapter := false
	if sm, present, err := readManifest(srcDir); err == nil && present {
		for _, it := range sm.Items {
			if it.File == file {
				wasChapter = true
				break
			}
		}
	}

	// Move the file first, so a failed move never leaves dangling manifest edits.
	if err := safeMove(filepath.Join(srcDir, file), dst); err != nil {
		return err
	}

	// Source manifest: drop the chapter (read-modify-write).
	if wasChapter {
		if sm, present, err := readManifest(srcDir); err == nil && present {
			if err := writeManifest(srcDir, manifestRemove(sm, file)); err != nil {
				return err
			}
		}
	}

	// Destination manifest: append as a chapter when requested and the dest is a manuscript.
	if asChapter && hasManifest(dstDir) {
		dm, present, err := readManifest(dstDir)
		if err != nil {
			return err
		}
		if present {
			if err := writeManifest(dstDir, manifestInsert(dm, file, sectionTitle(file), len(dm.Items))); err != nil {
				return err
			}
		}
	}
	return nil
}

// safeMove moves a file from src to dst. It tries os.Rename (fast, same-volume) and, only on a
// cross-device error, falls back to copy-then-remove. dst's parent directory must already exist.
func safeMove(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// Cross-volume: copy the bytes atomically to dst, then remove the source.
	data, rerr := os.ReadFile(src)
	if rerr != nil {
		return rerr
	}
	if werr := atomicWrite(dst, data, 0o644); werr != nil {
		return werr
	}
	return os.Remove(src)
}
