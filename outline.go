package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// renameOp is a single base-name rename within a manuscript dir.
type renameOp struct {
	from, to string
}

// splitPrefix splits name into its leading run of digits and the remainder
// (everything after the digits, verbatim). "02-the-letter.md" -> ("02",
// "-the-letter.md"); "notes.md" -> ("", "notes.md"). Renumbering keeps rest
// untouched, so the title slug, separator, and extension survive losslessly.
func splitPrefix(name string) (digits, rest string) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	return name[:i], name[i:]
}

// existingPrefixWidth returns the widest leading-digit-run length among the
// sections (0 if none). Used so renumbering never shrinks the pad width.
func existingPrefixWidth(sections []fileEntry) int {
	w := 0
	for _, s := range sections {
		if d, _ := splitPrefix(s.name); len(d) > w {
			w = len(d)
		}
	}
	return w
}

// padWidth picks the zero-pad width for count sections: at least 2, at least the
// digits needed for count, and never narrower than the existing width.
func padWidth(count, existingWidth int) int {
	w := 2
	if d := len(fmt.Sprintf("%d", count)); d > w {
		w = d
	}
	if existingWidth > w {
		w = existingWidth
	}
	return w
}

// planRenames maps an ordered section list onto contiguous, zero-padded prefixes
// of the given width, keeping everything after the old digit run verbatim. Ops
// whose name is already correct are omitted.
func planRenames(ordered []fileEntry, width int) []renameOp {
	var ops []renameOp
	for i, e := range ordered {
		_, rest := splitPrefix(e.name)
		next := fmt.Sprintf("%0*d", width, i+1) + rest
		if next != e.name {
			ops = append(ops, renameOp{from: e.name, to: next})
		}
	}
	return ops
}

// projectTitle de-slugs a manuscript folder name for display: drop a trailing
// extension if any, turn -/_ into spaces. Unlike sectionTitle it does NOT strip a
// leading digit run ("2024-trip-journal" -> "2024 trip journal").
func projectTitle(name string) string {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// applyRenames performs ops within dir using a two-phase temp pass so that order
// swaps (01<->02) don't collide. All targets are validated to stay inside dir
// BEFORE any rename happens; on a mid-operation failure it makes a best-effort
// rollback of files still parked under temp names. (The caller snapshots to
// .backup/ before calling, so a phase-2 failure remains recoverable.)
func applyRenames(dir string, ops []renameOp) error {
	// Preflight: validate every target before touching disk.
	for _, op := range ops {
		if !withinRoot(filepath.Join(dir, op.to), dir) {
			return fmt.Errorf("rename target escapes project: %s", op.to)
		}
	}
	type pend struct{ tmp, final, orig string }
	var pending []pend
	for i, op := range ops {
		orig := filepath.Join(dir, op.from)
		tmp := filepath.Join(dir, fmt.Sprintf(".okashi-renumber-%d.tmp", i))
		if err := os.Rename(orig, tmp); err != nil {
			for _, p := range pending { // roll back temps to their originals
				_ = os.Rename(p.tmp, p.orig)
			}
			return err
		}
		pending = append(pending, pend{tmp: tmp, final: filepath.Join(dir, op.to), orig: orig})
	}
	for idx, p := range pending {
		if err := os.Rename(p.tmp, p.final); err != nil {
			for _, q := range pending[idx:] { // roll back the unfinalized temps
				_ = os.Rename(q.tmp, q.orig)
			}
			return err
		}
	}
	return nil
}

// commitReorder snapshots the section files, then renumbers them on disk to match
// the working order. Returns old->new absolute paths for moved files (nil if the
// order was already correct). stamp is supplied by the caller.
func commitReorder(dir string, working []fileEntry, stamp string) (map[string]string, error) {
	width := padWidth(len(working), existingPrefixWidth(working))
	ops := planRenames(working, width)
	if len(ops) == 0 {
		return nil, nil
	}
	var paths []string
	for _, w := range working {
		paths = append(paths, filepath.Join(dir, w.name))
	}
	if err := backupFiles(dir, stamp, paths); err != nil {
		return nil, err
	}
	if err := applyRenames(dir, ops); err != nil {
		return nil, err
	}
	moved := make(map[string]string, len(ops))
	for _, op := range ops {
		moved[filepath.Join(dir, op.from)] = filepath.Join(dir, op.to)
	}
	return moved, nil
}
