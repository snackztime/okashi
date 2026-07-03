package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestBackupSnapshotOncePerSession(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("v1"), 0o644)
	m := initialModel()
	m.loadFile(p)
	m.editor.SetValue("v2")
	m.dirty = true
	m.save() // first save: snapshots the pre-edit "v1"
	bakDir := filepath.Join(dir, ".okashi-bak")
	entries, _ := os.ReadDir(bakDir)
	if len(entries) != 1 {
		t.Fatalf("first save should create exactly one snapshot, got %d", len(entries))
	}
	if b, _ := os.ReadFile(filepath.Join(bakDir, entries[0].Name())); string(b) != "v1" {
		t.Fatalf("snapshot should hold the pre-edit content, got %q", b)
	}
	m.editor.SetValue("v3")
	m.dirty = true
	m.save() // second save this session: no new snapshot
	entries2, _ := os.ReadDir(bakDir)
	if len(entries2) != 1 {
		t.Fatalf("second same-session save should not add a snapshot, got %d", len(entries2))
	}
}

func TestExternalChangeDivertsToConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.md")
	os.WriteFile(p, []byte("orig"), 0o644)
	m := initialModel()
	m.loadFile(p) // records loadedMtime
	m.editor.SetValue("my edits")
	m.dirty = true
	// Simulate an external change: rewrite p with a strictly newer mtime.
	os.WriteFile(p, []byte("external version"), 0o644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(p, future, future)
	m.save()
	// The original on disk must NOT be overwritten.
	if got, _ := os.ReadFile(p); string(got) != "external version" {
		t.Fatalf("save must not clobber the externally-changed file, got %q", got)
	}
	// A conflict copy must hold our edits, and currentFile must repoint to it.
	matches, _ := filepath.Glob(filepath.Join(dir, "ch.conflict-*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected one conflict copy, got %d", len(matches))
	}
	if b, _ := os.ReadFile(matches[0]); string(b) != "my edits" {
		t.Fatalf("conflict copy should hold our edits, got %q", b)
	}
	if m.currentFile != matches[0] {
		t.Fatalf("currentFile should repoint to the conflict copy, got %q", m.currentFile)
	}
}

func TestLoadFileAbortsOnFlushFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	os.WriteFile(a, []byte("a-orig"), 0o644)
	os.WriteFile(b, []byte("b-orig"), 0o644)
	m := initialModel()
	m.loadFile(a)
	m.editor.SetValue("a-edited")
	m.dirty = true
	// Make the dir non-writable so the outgoing flush (atomicWrite temp+rename) fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skip("cannot restrict dir perms")
	}
	defer os.Chmod(dir, 0o755)
	m.loadFile(b) // flush fails → load must abort, buffer preserved
	if m.currentFile != a {
		t.Fatalf("load must abort on flush failure and stay on a, currentFile=%q", m.currentFile)
	}
	if m.editor.Value() != "a-edited" {
		t.Fatalf("unsaved buffer must be preserved on flush failure, got %q", m.editor.Value())
	}
}
