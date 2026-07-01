package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrimarySourceResolvesToWritingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)
	p := primarySource()
	if p.ID != primarySourceID {
		t.Fatalf("primary ID = %q, want %q", p.ID, primarySourceID)
	}
	if p.Kind != sourceKindPrimary {
		t.Fatalf("primary Kind = %v, want primary", p.Kind)
	}
	if p.Path != "" {
		t.Fatalf("primary stored Path must be empty (resolved at runtime), got %q", p.Path)
	}
	if p.root() != dir {
		t.Fatalf("primary root() = %q, want writingDir() %q", p.root(), dir)
	}
}

func TestNewFolderSource(t *testing.T) {
	s := newFolderSource("/tmp/writing/Dropbox Novels")
	if s.Kind != sourceKindFolder {
		t.Fatalf("Kind = %v, want folder", s.Kind)
	}
	if s.Path != "/tmp/writing/Dropbox Novels" || s.root() != "/tmp/writing/Dropbox Novels" {
		t.Fatalf("Path/root = %q/%q", s.Path, s.root())
	}
	if s.Name != "Dropbox Novels" {
		t.Fatalf("Name = %q, want the folder base name", s.Name)
	}
	if s.ID == "" || s.ID == primarySourceID {
		t.Fatalf("folder source needs a stable non-primary ID, got %q", s.ID)
	}
}

func TestSourceReachable(t *testing.T) {
	dir := t.TempDir()
	if !newFolderSource(dir).reachable() {
		t.Fatal("an existing dir must be reachable")
	}
	if newFolderSource(filepath.Join(dir, "gone")).reachable() {
		t.Fatal("a missing dir must be unreachable")
	}
	// A file (not a dir) is not a reachable source root.
	f := filepath.Join(dir, "a.md")
	os.WriteFile(f, []byte("x"), 0o644)
	if newFolderSource(f).reachable() {
		t.Fatal("a plain file is not a reachable source root")
	}
}
