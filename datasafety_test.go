package main

import (
	"os"
	"path/filepath"
	"testing"
)

// editorModelAt builds a model whose editor holds `content` for `path`, marked dirty.
func dirtyModel(t *testing.T, path, content string) model {
	t.Helper()
	m := initialModel()
	m.loadFile(path) // sets currentFile + loadedMtime
	m.editor.SetValue(content)
	m.dirty = true
	return m
}

func TestSaveIfDirtyFlushes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("old"), 0o644)
	m := dirtyModel(t, p, "new content")
	m.saveIfDirty()
	got, _ := os.ReadFile(p)
	if string(got) != "new content" {
		t.Fatalf("saveIfDirty should flush the buffer, disk = %q", got)
	}
}

func TestLoadFileFlushesOutgoingBuffer(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	os.WriteFile(a, []byte("a-orig"), 0o644)
	os.WriteFile(b, []byte("b-orig"), 0o644)
	m := initialModel()
	m.loadFile(a)
	m.editor.SetValue("a-edited")
	m.dirty = true
	m.loadFile(b) // switching away must flush a first
	if got, _ := os.ReadFile(a); string(got) != "a-edited" {
		t.Fatalf("switching chapters must save the outgoing buffer, a = %q", got)
	}
	if m.editor.Value() != "b-orig" {
		t.Fatalf("after switch the editor should show b, got %q", m.editor.Value())
	}
}
