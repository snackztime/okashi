package main

import "time"

const undoCap = 30 // per-file checkpoints retained

// checkpointUndo appends the current buffer to the per-file undo ring when it changed since the
// last checkpoint. Called on changed autosave ticks and before bulk (spell/grammar) applies —
// a coarse checkpoint undo, matching the autosave granularity the writer already trusts.
func (m *model) checkpointUndo() {
	cur := m.editor.Value()
	if n := len(m.undoStack); n > 0 && m.undoStack[n-1] == cur {
		return // unchanged since the last checkpoint
	}
	m.undoStack = append(m.undoStack, cur)
	if len(m.undoStack) > undoCap {
		m.undoStack = m.undoStack[len(m.undoStack)-undoCap:]
	}
}

// undo (ctrl+z) restores the buffer to the previous checkpoint. It first ensures the CURRENT
// buffer is on the ring (capturing edits since the last tick, or a mutation like replace/spell-apply
// that changed the buffer without its own post-checkpoint), then pops it and restores the prior
// state. No-op with nothing to undo.
func (m *model) undo() {
	if cur := m.editor.Value(); len(m.undoStack) == 0 || m.undoStack[len(m.undoStack)-1] != cur {
		m.undoStack = append(m.undoStack, cur)
	}
	if len(m.undoStack) < 2 {
		m.status = "nothing to undo"
		return
	}
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.editor.SetValue(m.undoStack[len(m.undoStack)-1])
	m.dirty = true
	m.lastEditAt = time.Now()
	m.status = "undo"
}
