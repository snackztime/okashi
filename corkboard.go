package main

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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

	// Export prompt (ctrl+e from the corkboard → whole-manuscript export).
	if m.exportPrompt {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "m":
			m.exportPrompt = false
			m.runExport(StyleManuscript)
		case "t":
			m.exportPrompt = false
			m.runExport(StyleTufte)
		case "esc":
			m.exportPrompt = false
			m.status = "export cancelled"
		}
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

	// Retitle sub-mode.
	if m.structureRenaming {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.structureRenaming = false
			m.nameInput.Blur()
		case "enter":
			m.structureRenaming = false
			m.nameInput.Blur()
			if t := strings.TrimSpace(m.nameInput.Value()); t != "" && m.structureSel < len(m.structureItems) {
				m.structureItems[m.structureSel].Title = t
				m.structureDirty = true
			}
		default:
			var cmd tea.Cmd
			m.nameInput, cmd = m.nameInput.Update(key)
			return m, cmd
		}
		return m, nil
	}

	// Add sub-mode (new blank chapter / promote a Resource).
	if m.structureAdding {
		choices := m.structureAddChoices()
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.structureAdding = false
		case "up", "k":
			if m.structureAddSel > 0 {
				m.structureAddSel--
			}
		case "down", "j":
			if m.structureAddSel < len(choices)-1 {
				m.structureAddSel++
			}
		case "enter":
			if m.structureAddSel < len(choices) {
				m.applyAdd(choices[m.structureAddSel])
			}
			m.structureAdding = false
		}
		return m, nil
	}

	// Reorder/structure commit confirm.
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
			m.exitCorkboard()
			m.status = "changes saved"
		case "esc", "n":
			m.structureConfirm = false
			m.exitCorkboard()
			m.status = "changes discarded"
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
	case "e":
		m.startSynopsisEdit()
	case "ctrl+e":
		m.exportPrompt = true
		m.status = "export: m manuscript · t tufte · esc cancel"
	case "a":
		m.structureAdding = true
		m.structureAddSel = 0
	case "x":
		if m.structureSel < len(m.structureItems) {
			f := m.structureItems[m.structureSel].File
			delete(m.structurePendingNew, f)
			m.structureItems = append(m.structureItems[:m.structureSel], m.structureItems[m.structureSel+1:]...)
			if m.structureSel >= len(m.structureItems) && m.structureSel > 0 {
				m.structureSel--
			}
			m.structureDirty = true
		}
	case "r":
		if m.structureSel < len(m.structureItems) {
			m.structureRenaming = true
			m.nameInput.SetValue(m.structureItems[m.structureSel].Title)
			m.nameInput.CursorEnd()
			m.nameInput.Focus()
			return m, textinput.Blink
		}
	case "enter":
		// Open the selected chapter; staged changes must be resolved first.
		if m.structureDirty {
			m.structureConfirm = true
			m.status = "apply changes first — y apply · esc discard"
		} else if m.structureSel < len(m.structureItems) {
			file := filepath.Join(m.structureDir, m.structureItems[m.structureSel].File)
			m.exitCorkboard()
			m.loadFile(file)
			m.focus = focusEditor
			m.editor.Focus()
		}
	case "esc":
		if m.structureDirty {
			m.structureConfirm = true
		} else {
			m.exitCorkboard()
		}
	}
	return m, nil
}

// exitCorkboard leaves the corkboard for the writing screen, clearing the staged buffer and
// reloading the pane so it reflects any committed structural changes.
func (m *model) exitCorkboard() {
	m.structureDirty = false
	m.structureConfirm = false
	m.structureAdding = false
	m.structureRenaming = false
	m.structureItems = nil
	m.files.SetDir(m.files.dir)
	m.screen = screenWriting
	m.focus = focusSidebar
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
	if m.structureRenaming {
		field := "retitle ▸ " + m.nameInput.View()
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, field))
		return b.String()
	}
	if m.structureAdding {
		var picks []string
		for i, c := range m.structureAddChoices() {
			label := c.label
			if i == m.structureAddSel {
				label = selectedStyle.Render(label)
			}
			picks = append(picks, label)
		}
		if len(picks) == 0 {
			picks = append(picks, lipgloss.NewStyle().Foreground(subtle).Render("(no resources to promote)"))
		}
		pick := framedPanel("add", strings.Join(picks, "\n"), max(30, min(m.width-8, 44)), len(picks)+2, "")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, pick))
		return b.String()
	}
	if m.structureConfirm {
		bar := lipgloss.NewStyle().Foreground(accent).Render("apply changes? y apply · esc discard")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ · J/K reorder · e synopsis · a add · x remove · r retitle · ⏎ open · esc")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
