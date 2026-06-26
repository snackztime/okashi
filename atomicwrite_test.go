package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteCreatesFileWithMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	if err := atomicWrite(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "hello" {
		t.Fatalf("content = %q, err %v", b, err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %v, want 0644", fi.Mode().Perm())
	}
}

func TestAtomicWriteOverwrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	os.WriteFile(p, []byte("old longer contents"), 0o644)
	if err := atomicWrite(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "new" {
		t.Fatalf("overwrite content = %q, want new", b)
	}
}

func TestAtomicWriteLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	if err := atomicWrite(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "f.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only f.md, got %v (temp left behind?)", names)
	}
}

func TestAtomicWriteErrorsOnMissingDir(t *testing.T) {
	if err := atomicWrite(filepath.Join(t.TempDir(), "nope", "f.md"), []byte("x"), 0o644); err == nil {
		t.Fatal("expected an error writing into a non-existent directory")
	}
}
