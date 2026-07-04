package main

import "testing"

func TestReplaceAllInDocAndUndo(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("the cat sat; the cat ran")
	m.undoStack = []string{m.editor.Value()}
	m.searchInput.SetValue("cat")
	m.replaceInput.SetValue("dog")
	m.replaceMode = true
	m.searchReturn = screenWriting
	m.replaceAllInDoc()
	if got := m.editor.Value(); got != "the dog sat; the dog ran" {
		t.Fatalf("replace-all = %q", got)
	}
	if m.replaceMode {
		t.Fatal("replaceMode should clear after replace")
	}
	if m.screen != screenWriting {
		t.Fatalf("should return to the writing screen, got %v", m.screen)
	}
	// Immediate ctrl+z restores the pre-replace text.
	m.undo()
	if got := m.editor.Value(); got != "the cat sat; the cat ran" {
		t.Fatalf("undo after replace = %q, want the original", got)
	}
}

func TestReplaceNoMatchLeavesBufferUnchanged(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("hello world")
	m.searchInput.SetValue("xyz")
	m.replaceInput.SetValue("q")
	m.replaceMode = true
	m.searchReturn = screenWriting
	m.replaceAllInDoc()
	if got := m.editor.Value(); got != "hello world" {
		t.Fatalf("no-match must not change the buffer, got %q", got)
	}
}
