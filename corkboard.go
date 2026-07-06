package main

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"okashi/internal/textarea"
)

// enterCorkboard opens the corkboard for the binder's manuscript: it loads the same staged buffer
// structure mode uses (so reorder + commit are shared) plus the synopsis sidecar.
func (m *model) enterCorkboard() {
	dir := m.files.dir
	sm, present, err := readManifest(dir)
	if !present || err != nil {
		m.status = "not reorderable — no manifest"
		return
	}
	m.structureDir = dir
	m.structureItems = append([]manifestItem{}, sm.Items...)
	m.structureSel = 0
	m.structurePendingNew = map[string]bool{}
	m.structureDirty = false
	m.structureConfirm = false
	m.structureAdding = false   // defensive: never inherit a structure-mode sub-mode
	m.structureRenaming = false // (the two modes share the structure* fields)
	m.synopses = loadSynopses(dir)
	m.synEditing = false
	m.screen = screenCorkboard
}

// corkChapterSet is the current chapter file set. The corkboard is reorder-only, so the staged
// buffer never changes the SET — it equals the on-disk manifest set, making it a safe prune target.
func (m model) corkChapterSet() map[string]bool {
	s := map[string]bool{}
	for _, it := range m.structureItems {
		s[it.File] = true
	}
	return s
}

func newSynopsisArea(val string) textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.SetValue(val)
	return ta
}

func (m *model) startSynopsisEdit() {
	if m.structureSel < 0 || m.structureSel >= len(m.structureItems) {
		return
	}
	file := m.structureItems[m.structureSel].File
	m.synArea = newSynopsisArea(m.synopses[file])
	m.synArea.Focus()
	m.synEditing = true
}

// commitSynopsis stores the edited synopsis and writes the sidecar immediately (pruned), keeping
// synopsis persistence independent of the manifest reorder commit.
func (m *model) commitSynopsis() {
	m.synEditing = false
	m.synArea.Blur()
	if m.structureSel < 0 || m.structureSel >= len(m.structureItems) {
		return
	}
	file := m.structureItems[m.structureSel].File
	text := strings.TrimRight(m.synArea.Value(), "\n")
	if m.synopses == nil {
		m.synopses = map[string]string{}
	}
	if text == "" {
		delete(m.synopses, file)
	} else {
		m.synopses[file] = text
	}
	if err := saveSynopses(m.structureDir, m.synopses, m.corkChapterSet()); err != nil {
		m.status = "synopsis save failed: " + err.Error()
	} else {
		m.status = "synopsis saved"
	}
}

func (m model) updateCorkboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.synEditing {
		if key.String() == "esc" { // esc commits (⏎ inserts the 1–3 line breaks)
			m.commitSynopsis()
			return m, nil
		}
		var cmd tea.Cmd
		m.synArea, cmd = m.synArea.Update(key)
		return m, cmd
	}

	// Reorder-commit confirm — reuses structure mode's commit path, returns to the binder.
	if m.structureConfirm {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "y":
			if err := m.commitStructure(); err != nil {
				m.status = "commit failed: " + err.Error()
				m.structureConfirm = false
				return m, nil
			}
			m.structureConfirm = false
			m.exitStructure()
			m.status = "order saved"
		case "esc", "n":
			m.structureConfirm = false
			m.exitStructure()
			m.status = "order changes discarded"
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.structureSel > 0 {
			m.structureSel--
		}
	case "down", "j":
		if m.structureSel < len(m.structureItems)-1 {
			m.structureSel++
		}
	case "J", "shift+down":
		if m.structureSel < len(m.structureItems)-1 {
			i := m.structureSel
			m.structureItems[i], m.structureItems[i+1] = m.structureItems[i+1], m.structureItems[i]
			m.structureSel++
			m.structureDirty = true
		}
	case "K", "shift+up":
		if m.structureSel > 0 {
			i := m.structureSel
			m.structureItems[i], m.structureItems[i-1] = m.structureItems[i-1], m.structureItems[i]
			m.structureSel--
			m.structureDirty = true
		}
	case "e", "enter":
		m.startSynopsisEdit()
	case "esc":
		if m.structureDirty {
			m.structureConfirm = true
		} else {
			m.exitStructure()
		}
	}
	return m, nil
}

// wrapClamp word-wraps s to width columns, clamped to maxLines (overflow → an ellipsis).
func wrapClamp(s string, width, maxLines int) string {
	if width < 1 {
		width = 1
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		cur := ""
		for _, w := range words {
			switch {
			case cur == "":
				cur = w
			case lipgloss.Width(cur)+1+lipgloss.Width(w) <= width:
				cur += " " + w
			default:
				lines = append(lines, cur)
				cur = w
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = ansi.Truncate(lines[maxLines-1], width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m model) corkboardView() string {
	const bodyRows = 3
	cardRows := bodyRows + 2 // + top/bottom border
	perCard := cardRows + 1  // + one blank line between cards
	vis := (m.height - 4) / perCard
	if vis < 1 {
		vis = 1
	}
	off := homeWindowOffset(len(m.structureItems), m.structureSel, vis)
	cardW := max(40, min(m.width-8, 76))

	var cards []string
	for i := off; i < len(m.structureItems) && len(cards) < vis; i++ {
		it := m.structureItems[i]
		wc := ""
		if m.files.wc != nil {
			wc = commafy(m.files.wc.count(filepath.Join(m.structureDir, it.File))) + "w"
		}
		syn := m.synopses[it.File]
		var body string
		if syn == "" {
			body = lipgloss.NewStyle().Foreground(subtle).Render("(no synopsis — e to add)")
		} else {
			body = wrapClamp(syn, cardW-4, bodyRows)
		}
		marker := "  "
		if i == m.structureSel {
			marker = selectedStyle.Render("▸ ")
		}
		hdr := marker + fmtNum(i+1) + " · " + it.Title
		cards = append(cards, framedPanel(hdr, body, cardW, cardRows, wc))
	}
	if len(cards) == 0 {
		cards = append(cards, lipgloss.NewStyle().Foreground(subtle).Render("(no chapters)"))
	}

	var b strings.Builder
	board := strings.Join(cards, "\n")
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, board))

	if m.synEditing {
		edit := framedPanel("synopsis · "+m.structureItems[m.structureSel].Title, m.synArea.View(), cardW, 5, "esc save")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, edit))
		return b.String()
	}
	if m.structureConfirm {
		bar := lipgloss.NewStyle().Foreground(accent).Render("apply the new order? y apply · esc discard")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ select · J/K reorder · e synopsis · esc back")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
