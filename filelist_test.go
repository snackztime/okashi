package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilelistReadsFiltersSorts(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "b.md"), "x")
	mustWrite(t, filepath.Join(dir, "a.txt"), "x")
	mustWrite(t, filepath.Join(dir, "skip.png"), "x") // wrong extension
	if err := os.Mkdir(filepath.Join(dir, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}

	f := newFilelist()
	f.SetDir(dir)

	var names []string
	for _, e := range f.entries {
		names = append(names, e.name)
	}
	// ".." first (temp dir has a parent), then dir, then files alpha; .png skipped.
	want := []string{"..", "chapters", "a.txt", "b.md"}
	if len(names) != len(want) {
		t.Fatalf("entries = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entries = %v, want %v", names, want)
		}
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
