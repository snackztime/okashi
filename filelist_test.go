package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFilelistNavigationAndScroll(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}

	f.moveBy(-1) // clamp at 0
	if f.selected != 0 {
		t.Fatalf("selected = %d, want 0", f.selected)
	}
	f.moveBy(4) // to last; window should scroll
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4", f.selected)
	}
	if f.offset != 2 { // height 3 → window [2,3,4]
		t.Fatalf("offset = %d, want 2", f.offset)
	}
	f.moveBy(10) // clamp at last
	if f.selected != 4 {
		t.Fatalf("selected = %d, want 4 (clamped)", f.selected)
	}
}

func TestFilelistSelectRow(t *testing.T) {
	f := newFilelist()
	f.height = 3
	f.offset = 2
	f.entries = []fileEntry{
		{name: ".."}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"},
	}
	f.selectRow(1) // offset 2 + row 1 = index 3
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3", f.selected)
	}
	f.selectRow(99) // out of range: ignored
	if f.selected != 3 {
		t.Fatalf("selected = %d, want 3 (unchanged)", f.selected)
	}
}

func TestFilelistActivate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "note.md"), "x")
	f := newFilelist()
	f.SetDir(dir)

	// Select the file (entries: "..", "note.md") and activate it.
	f.selected = 1
	path, ok := f.activate()
	if !ok || path != filepath.Join(dir, "note.md") {
		t.Fatalf("activate file = (%q, %v), want (%q, true)", path, ok, filepath.Join(dir, "note.md"))
	}

	// Activating ".." navigates up and opens nothing.
	f.SetDir(dir)
	f.selected = 0
	if _, ok := f.activate(); ok {
		t.Fatal("activating .. should not open a file")
	}
	if f.dir != filepath.Dir(dir) {
		t.Fatalf("after .. dir = %q, want %q", f.dir, filepath.Dir(dir))
	}
}

func TestFilelistHasAndSelectName(t *testing.T) {
	f := newFilelist()
	f.entries = []fileEntry{{name: ".."}, {name: "a.md"}, {name: "b.md"}}

	if !f.has("a.md") || f.has("nope.md") {
		t.Fatal("has() should report membership correctly")
	}
	f.selectName("b.md")
	if f.selected != 2 {
		t.Fatalf("selectName: selected = %d, want 2", f.selected)
	}
	f.selectName("missing") // no-op
	if f.selected != 2 {
		t.Fatalf("selectName(missing) should be a no-op, selected = %d", f.selected)
	}
}

func TestFilelistViewShowsIconsNoSlash(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	f := newFilelist()
	f.width = 20
	f.height = 5
	f.entries = []fileEntry{{name: "proj", isDir: true}, {name: "a.md"}}

	view := f.View()
	if strings.Contains(view, "proj/") {
		t.Fatal("dir should not get a trailing slash (the icon conveys it)")
	}
	if !strings.Contains(view, "▸ proj") {
		t.Fatalf("plain folder icon missing; view=%q", view)
	}
}
