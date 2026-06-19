package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPreviewToggle(t *testing.T) {
	m := initialModel()
	// Give it a size so layout() sizes the viewport.
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)

	// A document tall enough that the preview must scroll.
	var sb strings.Builder
	sb.WriteString("# Hello\n\nSome **bold** prose, then a long list:\n\n")
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "- item %d\n", i)
	}
	m.editor.SetValue(sb.String())

	// ctrl+p should enter preview and render via glamour. Note: focus is still
	// the launch default (focusSidebar) here — exactly the case that regressed.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if !m.previewing {
		t.Fatal("expected previewing=true after ctrl+p")
	}
	if m.focus != focusSidebar {
		t.Fatal("test precondition: expected focus to still be on the sidebar")
	}
	view := m.View()
	if strings.TrimSpace(view) == "" {
		t.Fatal("preview view is empty")
	}
	// glamour styles "Hello" — the literal "# " markdown marker should be gone.
	if strings.Contains(view, "# Hello") {
		t.Fatal("expected rendered markdown, found raw source")
	}

	// ↓ must scroll the preview, NOT the filepicker underneath it. With sidebar
	// focus this only works because previewing is routed first.
	if m.preview.YOffset != 0 {
		t.Fatalf("expected preview to start at top, got YOffset=%d", m.preview.YOffset)
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = nm.(model)
	if m.preview.YOffset == 0 {
		t.Fatal("expected ↓ to scroll the preview, but YOffset stayed 0 (routed to sidebar?)")
	}

	// ctrl+p again returns to editing.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = nm.(model)
	if m.previewing {
		t.Fatal("expected previewing=false after second ctrl+p")
	}
}
