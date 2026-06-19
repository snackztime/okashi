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
	entries  []fileEntry
	selected int
	offset   int // index of the top visible row
	width    int
	height   int
	allowed  map[string]bool
}

func newFilelist() filelist {
	return filelist{
		width:  sidebarWidth - 2,
		height: 1,
		allowed: map[string]bool{
			".md": true, ".txt": true, ".wg": true, ".markdown": true,
		},
	}
}

// SetDir loads dir's entries (filtered, sorted dirs-first) and resets the cursor.
func (f *filelist) SetDir(dir string) {
	f.dir = dir
	f.entries = nil
	f.selected = 0
	f.offset = 0

	if parent := filepath.Dir(dir); parent != dir {
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
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		label = ansi.Truncate(label, f.width, "…")
		if i == f.selected {
			b.WriteString(selectedStyle.Width(f.width).Render(label))
		} else {
			b.WriteString(label)
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
