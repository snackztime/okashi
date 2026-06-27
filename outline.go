package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
type outlineRow struct {
	entry     fileEntry
	isSection bool
}

// outlineModel is the full-screen manuscript outline. working is the read-only
// section order loaded from disk (as fileEntry for backward compat). titles maps
// each chapter filename to its display title (from the manifest, or de-slugged).
type outlineModel struct {
	dir      string
	working  []fileEntry
	loose    []fileEntry
	titles   map[string]string // chapter file -> display title
	selected int
	width    int
	height   int
	wc       *wordCountCache
}

// load reads dir's chapters (ordered) and loose files into the outline via the
// resolver, so the outline and sidebar never disagree about membership or order.
func (o *outlineModel) load(dir string, wc *wordCountCache) {
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	o.dir = dir
	o.working = make([]fileEntry, 0, len(v.chapters))
	o.titles = make(map[string]string, len(v.chapters))
	for _, ch := range v.chapters {
		o.working = append(o.working, fileEntry{name: ch.file})
		o.titles[ch.file] = ch.title
	}
	o.loose = v.loose
	o.selected = 0
	o.wc = wc
}

// chapterTitle returns the display title for a chapter filename, looking it up from
// the resolved titles map. Falls back to sectionTitle (for zero-view or legacy).
func (o outlineModel) chapterTitle(name string) string {
	if t, ok := o.titles[name]; ok {
		return t
	}
	return sectionTitle(name)
}

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

// rows returns the selectable rows: working sections, then loose files.
func (o outlineModel) rows() []outlineRow {
	rows := make([]outlineRow, 0, len(o.working)+len(o.loose))
	for _, e := range o.working {
		rows = append(rows, outlineRow{entry: e, isSection: true})
	}
	for _, e := range o.loose {
		rows = append(rows, outlineRow{entry: e, isSection: false})
	}
	return rows
}

// moveSelection moves the cursor by d, clamped across all rows.
func (o *outlineModel) moveSelection(d int) {
	n := len(o.working) + len(o.loose)
	if n == 0 {
		return
	}
	o.selected += d
	if o.selected < 0 {
		o.selected = 0
	}
	if o.selected >= n {
		o.selected = n - 1
	}
}

// selectedRow returns the row under the cursor.
func (o outlineModel) selectedRow() (outlineRow, bool) {
	rows := o.rows()
	if o.selected < 0 || o.selected >= len(rows) {
		return outlineRow{}, false
	}
	return rows[o.selected], true
}

// View renders the outline: a header line, then one row per section/loose file.
func (o outlineModel) View() string {
	title := projectTitle(filepath.Base(o.dir))
	total := projectWordCount(o.dir, o.working, o.wc)
	head := fmt.Sprintf("%s · %sw · %d sections", title, commafy(total), len(o.working))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(head, o.width, "…")))
	b.WriteString("\n\n") // outlineHeaderHeight = 2 rows

	rows := o.rows()
	for i, r := range rows {
		var line string
		if r.isSection {
			count := commafy(o.wc.count(filepath.Join(o.dir, r.entry.name))) + "w"
			left := fmt.Sprintf(" %d  %s", i+1, o.chapterTitle(r.entry.name))
			maxLeft := o.width - lipgloss.Width(count) - 1
			if maxLeft < 1 {
				maxLeft = 1
			}
			left = ansi.Truncate(left, maxLeft, "…")
			gap := o.width - lipgloss.Width(left) - lipgloss.Width(count)
			if gap < 1 {
				gap = 1
			}
			line = left + strings.Repeat(" ", gap) + count
		} else {
			line = ansi.Truncate(" "+r.entry.name, o.width, "…")
		}
		switch {
		case i == o.selected:
			b.WriteString(selectedStyle.Width(o.width).Render(ansi.Truncate(line, o.width, "…")))
		case !r.isSection:
			b.WriteString(lipgloss.NewStyle().Foreground(subtle).Render(line))
		default:
			b.WriteString(line)
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
