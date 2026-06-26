package main

import (
	"fmt"
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
