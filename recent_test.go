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

	addRecent(store, paths[0], 0)
	addRecent(store, paths[1], 0)
	addRecent(store, paths[2], 0)
	addRecent(store, paths[0], 0) // re-add moves to front, no dup

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

func TestRecentsLineRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")
	p := filepath.Join(dir, "chapter.md")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	addRecent(store, p, 42)
	if got := recentLine(store, p); got != 42 {
		t.Fatalf("recentLine: got %d, want 42", got)
	}
	// Re-adding with a newer line overwrites the stored position.
	addRecent(store, p, 7)
	if got := recentLine(store, p); got != 7 {
		t.Fatalf("recentLine after re-add: got %d, want 7", got)
	}
	// Unknown file → 0.
	if got := recentLine(store, filepath.Join(dir, "other.md")); got != 0 {
		t.Fatalf("recentLine unknown: got %d, want 0", got)
	}
}

func TestRecentsLegacyStringForm(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "recent.json")
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Legacy on-disk shape: files as bare strings.
	legacy := `{"files":["` + a + `","` + b + `"]}`
	if err := os.WriteFile(store, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadRecents(store)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("legacy load: got %v, want [%s %s]", got, a, b)
	}
	// Legacy entries have no line → 0, no panic.
	if ln := recentLine(store, a); ln != 0 {
		t.Fatalf("legacy line: got %d, want 0", ln)
	}
	// A new add upgrades the store in place without dropping legacy entries.
	addRecent(store, a, 99)
	if ln := recentLine(store, a); ln != 99 {
		t.Fatalf("upgraded line: got %d, want 99", ln)
	}
	if got := loadRecents(store); len(got) != 2 {
		t.Fatalf("after upgrade: got %v, want 2 entries", got)
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
	addRecent(store, gone, 0)
	if len(loadRecents(store)) != 0 {
		t.Fatal("non-existent recent path should be filtered on load")
	}
}
