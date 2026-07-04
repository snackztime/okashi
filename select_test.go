package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"okashi/internal/textarea"
)

func TestSelectModeToggle(t *testing.T) {
	m := model{screen: screenWriting, editor: textarea.New()}

	mm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = mm.(model)
	if !m.selectMode {
		t.Fatal("ctrl+x should enable select mode")
	}
	if cmd == nil {
		t.Fatal("enabling select mode should return a mouse-disable command")
	}

	mm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = mm.(model)
	if m.selectMode {
		t.Fatal("a second ctrl+x should disable select mode")
	}
	if cmd == nil {
		t.Fatal("disabling select mode should return a mouse-enable command")
	}
}

func TestSelectModeStatusIndicator(t *testing.T) {
	m := model{width: 100, colWidth: 72, selectMode: true, editor: textarea.New()}
	if !containsSubstr(m.statusBar(), "SELECT") {
		t.Fatal("the status bar should show the -- SELECT -- indicator when select mode is on")
	}
}

// containsSubstr is a tiny helper so the test file needs no extra import.
func containsSubstr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
