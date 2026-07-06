package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// enterOutline opens the full-screen outline mode on the current folder's outline.md (created seeded
// if missing), remembering the file to return to on esc.
func (m *model) enterOutline() {
	m.save() // flush the current chapter before loadFile reloads over it
	outlinePath := filepath.Join(m.files.dir, "outline.md")
	if _, err := os.Stat(outlinePath); err != nil {
		if werr := atomicWrite(outlinePath, []byte("- \n"), 0o644); werr != nil {
			m.status = "couldn't create outline: " + werr.Error()
			return
		}
		m.files.SetDir(m.files.dir) // surface outline.md in the sidebar
	}
	m.outlineReturnFile = m.currentFile
	m.loadFile(outlinePath)
	m.editor.Dim = false // no sentence-dim in the outline
	m.screen = screenOutline
	m.focus = focusEditor
	m.editor.Focus()
}

// exitOutline saves outline.md, reflects any promoted chapters, and returns to the prior file.
func (m *model) exitOutline() {
	m.save()
	m.files.SetDir(m.files.dir) // reflect promoted chapters in the pane/manifest
	if m.outlineReturnFile != "" {
		m.loadFile(m.outlineReturnFile)
	}
	m.syncDim() // restore the writing-screen dim setting
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
}

func (m model) updateOutline(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		m.layout()
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.exitOutline()
		return m, nil
	case "alt+up":
		m.moveOutlineBeat(-1)
		return m, nil
	case "alt+down":
		m.moveOutlineBeat(1)
		return m, nil
	case "alt+enter", "ctrl+enter":
		m.promoteOutlineBeat()
		return m, nil
	case "enter":
		if m.editor.AtLineEnd() {
			if prefix, clear, ok := listContinuation(m.editor.CurrentLine()); ok {
				if clear {
					m.editor.ClearLine()
				} else {
					m.editor.InsertString("\n" + prefix)
				}
				m.dirty = true
				m.lastEditAt = time.Now()
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	m.dirty = true
	m.lastEditAt = time.Now()
	return m, cmd
}

// moveOutlineBeat reorders the beat block under the cursor (dir -1 up / +1 down) and saves.
func (m *model) moveOutlineBeat(dir int) {
	out, nc, ok := moveBeat(strings.Split(m.editor.Value(), "\n"), m.editor.Line(), dir)
	if !ok {
		m.status = "move: put the cursor on a beat"
		return
	}
	m.editor.SetValue(strings.Join(out, "\n"))
	m.editor.MoveToLine(nc)
	m.dirty = true
	m.save()
}

// promoteOutlineBeat is implemented in Task 5.
func (m *model) promoteOutlineBeat() { m.status = "promote coming in the next task" }

func (m model) outlineView() string {
	title := projectTitle(filepath.Base(m.files.dir))
	header := sectionHeader("OUTLINE · "+title, m.width)
	foot := lipgloss.NewStyle().Foreground(subtle).Render("alt+↑/↓ move beat · alt+↵ promote · esc done")
	body := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Top, m.editor.View())
	return lipgloss.JoinVertical(lipgloss.Left, header, body,
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
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
