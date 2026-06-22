package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecentsAddDedupCapAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")

	// Create real files so load doesn't filter them out.
	var paths []string
	for i := 0; i < 3; i++ {
		p := filepath.Join(dir, string(rune('a'+i))+".md")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}

	addRecent(store, paths[0])
	addRecent(store, paths[1])
	addRecent(store, paths[2])
	addRecent(store, paths[0]) // re-add moves to front, no dup

	got := loadRecents(store)
	want := []string{paths[0], paths[2], paths[1]}
	if len(got) != 3 {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v, want %v", got, want)
		}
	}
}

func TestRecentsLoadFiltersMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")

	// Missing file → empty.
	if len(loadRecents(store)) != 0 {
		t.Fatal("missing store should load empty")
	}
	// Corrupt → empty, no panic.
	if err := os.WriteFile(store, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadRecents(store)) != 0 {
		t.Fatal("corrupt store should load empty")
	}
	// A path that no longer exists is dropped.
	gone := filepath.Join(dir, "gone.md")
	addRecent(store, gone)
	if len(loadRecents(store)) != 0 {
		t.Fatal("non-existent recent path should be filtered on load")
	}
}
