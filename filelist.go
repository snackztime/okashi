package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type fileEntry struct {
	name  string
	isDir bool
}

// filelist is a minimal, mouse-friendly file browser we fully own.
type filelist struct {
	dir      string
	root     string
	entries  []fileEntry
	selected int
	offset   int // index of the top visible row
	width    int
	height   int
	allowed  map[string]bool
	icons    iconSet
}

func newFilelist() filelist {
	return filelist{
		width:  sidebarWidth - 2,
		height: 1,
		allowed: map[string]bool{
			".md": true, ".txt": true, ".wg": true, ".markdown": true,
		},
		icons: resolveIcons(),
	}
}

// withinRoot reports whether dir is root or a descendant of it.
func withinRoot(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

// SetDir loads dir's entries (filtered, sorted dirs-first) and resets the cursor.
func (f *filelist) SetDir(dir string) {
	if f.root != "" && !withinRoot(dir, f.root) {
		dir = f.root
	}
	f.dir = dir
	f.entries = nil
	f.selected = 0
	f.offset = 0

	showParent := filepath.Dir(dir) != dir // not at filesystem root
	if f.root != "" {
		showParent = dir != f.root // confined: only below the workspace root
	}
	if showParent {
		f.entries = append(f.entries, fileEntry{name: "..", isDir: true})
	}

	items, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var dirs, files []fileEntry
	for _, it := range items {
		name := it.Name()
		if strings.HasPrefix(name, ".") {
			continue // hidden
		}
		if it.IsDir() {
			dirs = append(dirs, fileEntry{name: name, isDir: true})
			continue
		}
		if f.allowed[strings.ToLower(filepath.Ext(name))] {
			files = append(files, fileEntry{name: name})
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	f.entries = append(f.entries, dirs...)
	f.entries = append(f.entries, files...)
}

// View renders the visible window of entries, highlighting the selection.
func (f filelist) View() string {
	if len(f.entries) == 0 {
		return lipgloss.NewStyle().Foreground(subtle).Render("(empty)")
	}
	end := f.offset + f.height
	if end > len(f.entries) {
		end = len(f.entries)
	}
	var b strings.Builder
	for i := f.offset; i < end; i++ {
		e := f.entries[i]
		head := " " + f.icons.icon(e) // one-column gutter, then the icon
		full := head + e.name
		switch {
		case i == f.selected:
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(full, f.width, "…")))
		case e.isDir:
			b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(full, f.width, "…")))
		default:
			// Non-selected file: dim the extension when the whole row fits.
			ext := filepath.Ext(e.name)
			if ext != "" && lipgloss.Width(full) <= f.width {
				stem := head + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(full, f.width, "…"))
			}
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (f *filelist) moveBy(n int) {
	if len(f.entries) == 0 {
		return
	}
	f.selected += n
	if f.selected < 0 {
		f.selected = 0
	}
	if f.selected >= len(f.entries) {
		f.selected = len(f.entries) - 1
	}
	f.scrollIntoView()
}

func (f *filelist) scrollIntoView() {
	if f.selected < f.offset {
		f.offset = f.selected
	} else if f.height > 0 && f.selected >= f.offset+f.height {
		f.offset = f.selected - f.height + 1
	}
	if f.offset < 0 {
		f.offset = 0
	}
}

// selectRow sets the selection from a row index within the visible window.
func (f *filelist) selectRow(visibleRow int) {
	if visibleRow < 0 {
		return
	}
	idx := f.offset + visibleRow
	if idx >= len(f.entries) {
		return
	}
	f.selected = idx
}

// activate acts on the selected entry: directories (and "..") navigate and
// return ok=false; a file returns its absolute path with ok=true.
func (f *filelist) activate() (string, bool) {
	if len(f.entries) == 0 {
		return "", false
	}
	e := f.entries[f.selected]
	if e.isDir {
		if e.name == ".." {
			f.SetDir(filepath.Dir(f.dir))
		} else {
			f.SetDir(filepath.Join(f.dir, e.name))
		}
		return "", false
	}
	return filepath.Join(f.dir, e.name), true
}

// breadcrumb is the current path relative to the workspace root, e.g.
// "okashi" at the root or "okashi / Book Name" inside a project.
func (f filelist) breadcrumb() string {
	base := filepath.Base(f.root)
	rel, err := filepath.Rel(f.root, f.dir)
	if err != nil || rel == "." || rel == "" {
		return base
	}
	parts := strings.Split(rel, string(filepath.Separator))
	return base + " / " + strings.Join(parts, " / ")
}

// has reports whether an entry with the given name is currently listed.
func (f filelist) has(name string) bool {
	for _, e := range f.entries {
		if e.name == name {
			return true
		}
	}
	return false
}

// selectName moves the selection to the entry with the given name, if present.
func (f *filelist) selectName(name string) {
	for i, e := range f.entries {
		if e.name == name {
			f.selected = i
			f.scrollIntoView()
			return
		}
	}
}

type breadcrumbSeg struct {
	label string
	path  string
}

// segHit is a clickable column range [start,end) in the breadcrumb row.
type segHit struct {
	start, end int
	path       string
}

// breadcrumbSegments returns the segments from the workspace root (base name
// first) down to the current dir, each with its target path.
func (f filelist) breadcrumbSegments() []breadcrumbSeg {
	segs := []breadcrumbSeg{{label: filepath.Base(f.root), path: f.root}}
	rel, err := filepath.Rel(f.root, f.dir)
	if err == nil && rel != "." && rel != "" {
		cur := f.root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			segs = append(segs, breadcrumbSeg{label: part, path: cur})
		}
	}
	return segs
}

// breadcrumbBar renders the breadcrumb head-truncated to width, with a
// right-aligned "sel/total" indicator when the list overflows, and returns the
// clickable column ranges of the visible segments (the "…" is not clickable).
func (f filelist) breadcrumbBar(width int) (string, []segHit) {
	segs := f.breadcrumbSegments()

	ind := ""
	if f.height > 0 && len(f.entries) > f.height {
		ind = fmt.Sprintf("%d/%d", f.selected+1, len(f.entries))
	}
	avail := width
	if ind != "" {
		avail -= lipgloss.Width(ind) + 1
	}
	if avail < 1 {
		avail = 1
	}

	const sep = " / "
	labels := make([]string, len(segs))
	for i, s := range segs {
		labels[i] = s.label
	}

	var visible []breadcrumbSeg
	if lipgloss.Width(strings.Join(labels, sep)) <= avail {
		visible = segs
	} else {
		root := segs[0]
		used := lipgloss.Width(root.label) + lipgloss.Width(sep) + lipgloss.Width("…")
		var tail []breadcrumbSeg
		for i := len(segs) - 1; i >= 1; i-- {
			w := lipgloss.Width(sep) + lipgloss.Width(segs[i].label)
			if used+w > avail {
				break
			}
			used += w
			tail = append([]breadcrumbSeg{segs[i]}, tail...)
		}
		visible = append([]breadcrumbSeg{root, {label: "…", path: ""}}, tail...)
	}

	var b strings.Builder
	var hits []segHit
	col := 0
	for i, s := range visible {
		if i > 0 {
			b.WriteString(sep)
			col += lipgloss.Width(sep)
		}
		start := col
		b.WriteString(s.label)
		col += lipgloss.Width(s.label)
		if s.path != "" {
			hits = append(hits, segHit{start: start, end: col, path: s.path})
		}
	}
	left := b.String()

	if lipgloss.Width(left) > avail {
		left = ansi.Truncate(left, avail, "…")
		visibleW := lipgloss.Width(left)
		kept := hits[:0]
		for _, h := range hits {
			if h.end <= visibleW {
				kept = append(kept, h)
			}
		}
		hits = kept
	}

	if ind == "" {
		return left, hits
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(ind)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + ind, hits
}
