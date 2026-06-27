package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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
	s := name[i:]
	s = strings.TrimSuffix(s, filepath.Ext(s))
	s = strings.TrimLeft(s, "-_. ")
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

// hasNumberedSections reports whether any non-dir entry is a numbered section.
func hasNumberedSections(entries []fileEntry) bool {
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

// wordCountCache memoizes per-file word counts keyed by path + modtime so the
// sidebar and rollups don't re-read unchanged files every render.
type wordCountCache struct {
	entries map[string]wcEntry
}

type wcEntry struct {
	mod   time.Time
	words int
}

func newWordCountCache() *wordCountCache {
	return &wordCountCache{entries: map[string]wcEntry{}}
}

// count returns the word count of the file at path, reading it only when its
// modtime has changed since the last read. Missing/unreadable files count 0.
func (c *wordCountCache) count(path string) int {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if e, ok := c.entries[path]; ok && e.mod.Equal(info.ModTime()) {
		return e.words
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	w := wordCount(string(data))
	c.entries[path] = wcEntry{mod: info.ModTime(), words: w}
	return w
}

// projectWordCount sums the word counts of the ordered sections in dir.
func projectWordCount(dir string, sections []fileEntry, c *wordCountCache) int {
	total := 0
	for _, s := range sections {
		total += c.count(filepath.Join(dir, s.name))
	}
	return total
}
