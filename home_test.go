package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildHomeItems(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"novel", "journal", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	recents := []string{"/abs/chapter-03.md", "/abs/note.md"}

	items := buildHomeItems(recents, dir)

	// 2 recents + 2 projects (hidden excluded) + 1 open-other = 5
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d: %+v", len(items), items)
	}
	if items[0].kind != homeRecentFile || items[0].path != "/abs/chapter-03.md" {
		t.Fatalf("first item should be the most-recent file, got %+v", items[0])
	}
	if items[0].label != "chapter-03.md" {
		t.Fatalf("recent label should be the basename, got %q", items[0].label)
	}
	if items[2].kind != homeProject || items[2].label != "journal" {
		t.Fatalf("projects should be alpha-sorted after recents, got %+v", items[2])
	}
	if items[4].kind != homeOpenOther {
		t.Fatalf("last item should be open-other, got %+v", items[4])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	if len(items) != 1 || items[0].kind != homeOpenOther {
		t.Fatalf("empty state should be just open-other, got %+v", items)
	}
}
