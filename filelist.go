package main

import (
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
