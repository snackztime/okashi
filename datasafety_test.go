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
