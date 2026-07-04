package main

import (
	"fmt"
	"testing"
)

func TestUndoWalksBackThroughCheckpoints(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("v0")
	m.undoStack = []string{"v0"}
	m.editor.SetValue("v1")
	m.checkpointUndo() // [v0, v1]
	m.editor.SetValue("v2")
	m.checkpointUndo() // [v0, v1, v2]
	m.undo()
	if m.editor.Value() != "v1" {
		t.Fatalf("first undo = %q, want v1", m.editor.Value())
	}
	m.undo()
	if m.editor.Value() != "v0" {
		t.Fatalf("second undo = %q, want v0", m.editor.Value())
	}
	m.undo() // nothing left (len < 2) → stays at v0
	if m.editor.Value() != "v0" {
		t.Fatalf("undo past the start = %q, want v0", m.editor.Value())
	}
}

func TestUndoNoDupCheckpointAndCap(t *testing.T) {
	m := initialModel()
	m.editor.SetValue("same")
	m.undoStack = []string{"same"}
	m.checkpointUndo() // unchanged → no append
	if len(m.undoStack) != 1 {
		t.Fatalf("unchanged checkpoint should not append, len=%d", len(m.undoStack))
	}
	m.undoStack = nil
	for i := 0; i < undoCap+20; i++ {
		m.editor.SetValue(fmt.Sprintf("x%d", i))
		m.checkpointUndo()
	}
	if len(m.undoStack) > undoCap {
		t.Fatalf("ring exceeded cap %d: got %d", undoCap, len(m.undoStack))
	}
}
