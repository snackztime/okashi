package main

import (
	"os"
	"path/filepath"
	"strings"
)

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

// projectTitle de-slugs a manuscript folder name for display: drop a trailing
// extension if any, turn -/_ into spaces. Unlike sectionTitle it does NOT strip a
// leading digit run ("2024-trip-journal" -> "2024 trip journal").
func projectTitle(name string) string {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// slugify turns a typed section title into a filename slug: lowercase, spaces and
// underscores to hyphens, stripped of other punctuation.
func slugify(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "section"
	}
	return s
}

const outlineHeaderHeight = 2 // title line + blank spacer

// outlineRow is one selectable row: a numbered section or a loose file.

// readEntries lists dir's non-hidden document files (allowedDocExts) as fileEntry
// values (dirs excluded). Same filter as the sidebar, so the outline and the
// pane show the same files for a folder.
func readEntries(dir string) []fileEntry {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") || it.IsDir() {
			continue
		}
		if !allowedDocExts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		out = append(out, fileEntry{name: name})
	}
	return out
}
