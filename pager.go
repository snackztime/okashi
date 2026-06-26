package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// pagerLine is one display row of the manuscript pager: text already wrapped to
// the measure width, plus the source it maps back to. Header (chapter-rule) lines
// carry their section file with src = -1.
type pagerLine struct {
	text     string
	file     string // section base name ("" only if there are no sections)
	src      int    // 0-based source line within file; -1 for a header line
	header   bool
	cumWords int // running word count from the manuscript start through this line
}

// pagerModel is the full-screen read-through pager: a manual scroller that owns
// the cursor line and the line→source map. Built once by load.
type pagerModel struct {
	dir    string
	lines  []pagerLine
	total  int
	cursor int
	offset int
	width  int
	height int
}

// load concatenates the dir's ordered sections (loose excluded) into wrapped,
// mapped lines. Each section contributes a "── Title ──" header line then its
// body, each source line wrapped to width. Reads each file exactly once.
func (p *pagerModel) load(dir string, width int) {
	if width < 1 {
		width = 1
	}
	entries := readEntries(dir) // markdown/text files, dirs excluded (from outline.go)
	sections, _ := orderedSections(entries)

	p.dir = dir
	p.lines = nil
	p.cursor = 0
	p.offset = 0

	running := 0
	for _, sec := range sections {
		p.lines = append(p.lines, pagerLine{
			text:     "── " + sectionTitle(sec.name) + " ──",
			file:     sec.name,
			src:      -1,
			header:   true,
			cumWords: running,
		})
		data, err := os.ReadFile(filepath.Join(dir, sec.name))
		if err != nil {
			continue
		}
		body := strings.TrimSuffix(string(data), "\n")
		for srcIdx, srcLine := range strings.Split(body, "\n") {
			for _, row := range strings.Split(ansi.Wrap(srcLine, width, ""), "\n") {
				running += wordCount(row)
				p.lines = append(p.lines, pagerLine{
					text:     row,
					file:     sec.name,
					src:      srcIdx,
					header:   false,
					cumWords: running,
				})
			}
		}
	}
	p.total = running
}
