package main

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sectionOrder parses the leading run of digits in name as an integer. ok is
// false when name has no leading digit (a loose file). "1", "01", "001" all
// yield 1, so sorting by n orders 2 before 10.
func sectionOrder(name string) (int, bool) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(name[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// sectionTitle is the display title for a section file: the leading digits and
// one separator stripped, the extension dropped, and -/_ turned into spaces.
// "02-the-letter.md" -> "the letter".
func sectionTitle(name string) string {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	s := strings.TrimLeft(name[i:], "-_. ")
	s = strings.TrimSuffix(s, filepath.Ext(s))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// orderedSections splits file entries (non-dir) into numbered sections (sorted
// by their numeric prefix, then name) and loose files (alphabetical).
func orderedSections(files []fileEntry) (sections, loose []fileEntry) {
	for _, f := range files {
		if _, ok := sectionOrder(f.name); ok {
			sections = append(sections, f)
		} else {
			loose = append(loose, f)
		}
	}
	sort.SliceStable(sections, func(i, j int) bool {
		ni, _ := sectionOrder(sections[i].name)
		nj, _ := sectionOrder(sections[j].name)
		if ni != nj {
			return ni < nj
		}
		return sections[i].name < sections[j].name
	})
	sort.Slice(loose, func(i, j int) bool { return loose[i].name < loose[j].name })
	return sections, loose
}

// isManuscript reports whether any non-dir entry is a numbered section.
func isManuscript(entries []fileEntry) bool {
	for _, e := range entries {
		if e.isDir {
			continue
		}
		if _, ok := sectionOrder(e.name); ok {
			return true
		}
	}
	return false
}
