package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

const outlineHeaderHeight = 2 // title line + blank spacer

// outlineRow is one selectable row: a numbered section or a loose file.
type outlineRow struct {
	entry     fileEntry
	isSection bool
}

// outlineModel is the full-screen manuscript outline. working is the (possibly
// reordered) section order; disk is the on-disk order, for dirty detection.
type outlineModel struct {
	dir         string
	working     []fileEntry
	disk        []fileEntry
	loose       []fileEntry
	selected    int
	width       int
	height      int
	wc          *wordCountCache
	confirm     bool // apply/discard gate visible
	pendingOpen bool // the pending leave is an open (Enter), not a back (esc)
}

// load reads dir's sections (ordered) and loose files into the outline.
func (o *outlineModel) load(dir string, wc *wordCountCache) {
	entries := readEntries(dir)
	sections, loose := orderedSections(entries)
	o.dir = dir
	o.working = sections
	o.disk = append([]fileEntry(nil), sections...)
	o.loose = loose
	o.selected = 0
	o.wc = wc
	o.confirm = false
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

// dirty reports whether the working order differs from the on-disk order.
func (o outlineModel) dirty() bool {
	if len(o.working) != len(o.disk) {
		return true
	}
	for i := range o.working {
		if o.working[i].name != o.disk[i].name {
			return true
		}
	}
	return false
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

// moveSection moves the selected section by d within the working order (no-op
// unless the selection is a section). The selection follows the moved section.
func (o *outlineModel) moveSection(d int) {
	i := o.selected
	if i < 0 || i >= len(o.working) {
		return // selection is a loose row (or empty): not reorderable
	}
	j := i + d
	if j < 0 || j >= len(o.working) {
		return
	}
	o.working[i], o.working[j] = o.working[j], o.working[i]
	o.selected = j
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
	if o.dirty() {
		head += "   ● unsaved order"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(accent).Render(ansi.Truncate(head, o.width, "…")))
	b.WriteString("\n\n") // outlineHeaderHeight = 2 rows

	rows := o.rows()
	for i, r := range rows {
		var line string
		if r.isSection {
			digits, _ := splitPrefix(r.entry.name)
			count := commafy(o.wc.count(filepath.Join(o.dir, r.entry.name))) + "w"
			left := " " + digits + "  " + sectionTitle(r.entry.name)
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
