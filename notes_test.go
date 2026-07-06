package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNotesRoundTripAndTolerantLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "01-a.md")
	if err := os.WriteFile(file, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	notes := []note{
		{ID: "n1", Scope: "chapter", Text: "Cut the flashback.\nToo slow."},
		{ID: "n2", Scope: "chapter", Text: "Check the date < 1911 & > 1900."},
	}
	if err := saveNotes(file, notes); err != nil {
		t.Fatal(err)
	}
	got := loadNotes(file)
	if len(got) != 2 || got[0].Text != "Cut the flashback.\nToo slow." || got[1].ID != "n2" {
		t.Fatalf("round-trip: %+v", got)
	}
	// Ampersand/angle brackets not HTML-escaped.
	data, _ := os.ReadFile(notesPath(file))
	if !contains(string(data), "< 1911 & > 1900") {
		t.Fatalf("notes should not HTML-escape:\n%s", data)
	}
	// Tolerant: missing → nil; corrupt → nil.
	if loadNotes(filepath.Join(dir, "nope.md")) != nil {
		t.Fatal("missing notes → nil")
	}
	if err := os.WriteFile(notesPath(file), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadNotes(file) != nil {
		t.Fatal("corrupt notes → nil, no error")
	}
}

func TestSaveNotesEmptyRemovesSidecar(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("x"), 0o644)
	saveNotes(file, []note{{ID: "n1", Text: "keep"}})
	if _, err := os.Stat(notesPath(file)); err != nil {
		t.Fatal("sidecar should exist")
	}
	if err := saveNotes(file, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(notesPath(file)); !os.IsNotExist(err) {
		t.Fatal("emptying notes should remove the sidecar")
	}
}

func TestMoveAndDeleteNotesCoupling(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "old.md")
	newFile := filepath.Join(dir, "new.md")
	os.WriteFile(oldFile, []byte("x"), 0o644)
	saveNotes(oldFile, []note{{ID: "n1", Text: "note"}})

	moveNotes(oldFile, newFile)
	if _, err := os.Stat(notesPath(oldFile)); !os.IsNotExist(err) {
		t.Fatal("old sidecar should be gone after moveNotes")
	}
	if loadNotes(newFile)[0].Text != "note" {
		t.Fatal("notes should follow the rename")
	}
	deleteNotes(newFile)
	if _, err := os.Stat(notesPath(newFile)); !os.IsNotExist(err) {
		t.Fatal("deleteNotes should drop the sidecar")
	}
}

func TestNotesScreenAddEditDelete(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("x"), 0o644)

	m := model{width: 80, height: 24, notes: notesModel{file: file}}
	m.screen = screenNotes

	// a → add; type; esc → commit + persist.
	mm, _ := m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = mm.(model)
	if !m.notes.adding {
		t.Fatal("a should start adding")
	}
	m.notes.area.SetValue("This chapter drags.")
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if len(m.notes.notes) != 1 || loadNotes(file)[0].Text != "This chapter drags." {
		t.Fatalf("note not added/persisted: %+v", m.notes.notes)
	}

	// e → edit; change; esc → persist.
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = mm.(model)
	m.notes.area.SetValue("Cut the flashback.")
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if loadNotes(file)[0].Text != "Cut the flashback." {
		t.Fatalf("edit not persisted: %+v", loadNotes(file))
	}

	// d → confirm → y deletes + persists (sidecar removed).
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = mm.(model)
	if !m.notes.confirmDelete {
		t.Fatal("d should raise the delete confirm")
	}
	mm, _ = m.updateNotes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)
	if len(m.notes.notes) != 0 {
		t.Fatal("note should be deleted")
	}
	if _, err := os.Stat(notesPath(file)); !os.IsNotExist(err) {
		t.Fatal("deleting the last note should remove the sidecar")
	}
}
