package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitPrefix(t *testing.T) {
	cases := []struct{ name, digits, rest string }{
		{"02-the-letter.md", "02", "-the-letter.md"},
		{"1-x.md", "1", "-x.md"},
		{"notes.md", "", "notes.md"},
		{"01.md", "01", ".md"},
	}
	for _, c := range cases {
		d, r := splitPrefix(c.name)
		if d != c.digits || r != c.rest {
			t.Errorf("splitPrefix(%q) = (%q,%q), want (%q,%q)", c.name, d, r, c.digits, c.rest)
		}
	}
}

func TestPadWidth(t *testing.T) {
	cases := []struct{ count, existing, want int }{
		{3, 2, 2},   // small project, 2 digits
		{99, 2, 2},  // still 2
		{100, 2, 3}, // crossing 100 widens
		{50, 3, 3},  // never shrink below existing width
		{1, 0, 2},   // floor of 2
	}
	for _, c := range cases {
		if got := padWidth(c.count, c.existing); got != c.want {
			t.Errorf("padWidth(%d,%d) = %d, want %d", c.count, c.existing, got, c.want)
		}
	}
}

func TestExistingPrefixWidth(t *testing.T) {
	w := existingPrefixWidth([]fileEntry{{name: "01-a.md"}, {name: "001-b.md"}, {name: "notes.md"}})
	if w != 3 {
		t.Fatalf("existingPrefixWidth = %d, want 3 (widest run)", w)
	}
}

func TestPlanRenamesReorder(t *testing.T) {
	// Move section #3 up one slot: working order [01,03,02].
	working := []fileEntry{{name: "01-a.md"}, {name: "03-c.md"}, {name: "02-b.md"}}
	ops := planRenames(working, 2)
	// 01-a stays; 03-c -> 02-c; 02-b -> 03-b.
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["03-c.md"] != "02-c.md" || got["02-b.md"] != "03-b.md" {
		t.Fatalf("ops = %v, want 03-c->02-c and 02-b->03-b", got)
	}
	if _, ok := got["01-a.md"]; ok {
		t.Fatalf("01-a.md should not be renamed (already correct)")
	}
}

func TestPlanRenamesNoop(t *testing.T) {
	working := []fileEntry{{name: "01-a.md"}, {name: "02-b.md"}}
	if ops := planRenames(working, 2); len(ops) != 0 {
		t.Fatalf("already-correct order should yield no ops, got %v", ops)
	}
}

func TestPlanRenamesWidens(t *testing.T) {
	working := []fileEntry{{name: "1-a.md"}, {name: "2-b.md"}}
	ops := planRenames(working, 2)
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["1-a.md"] != "01-a.md" || got["2-b.md"] != "02-b.md" {
		t.Fatalf("ops = %v, want zero-padded to width 2", got)
	}
}

func TestProjectTitle(t *testing.T) {
	cases := map[string]string{
		"my-novel":          "my novel",
		"2024-trip-journal": "2024 trip journal", // leading digits NOT stripped
		"Essays_draft":      "Essays draft",
	}
	for in, want := range cases {
		if got := projectTitle(in); got != want {
			t.Errorf("projectTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyRenamesSwapNoCollision(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("AAA"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("BBB"), 0o644)
	// Swap their numbers: 01-a -> 02-a, 02-b -> 01-b.
	ops := []renameOp{{"01-a.md", "02-a.md"}, {"02-b.md", "01-b.md"}}
	if err := applyRenames(dir, ops); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "02-a.md")); string(b) != "AAA" {
		t.Fatalf("02-a.md content = %q, want AAA", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "01-b.md")); string(b) != "BBB" {
		t.Fatalf("01-b.md content = %q, want BBB", b)
	}
}

func TestApplyRenamesRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	err := applyRenames(dir, []renameOp{{"01-a.md", "../escaped.md"}})
	if err == nil {
		t.Fatal("expected an error for a target escaping the project dir")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "01-a.md")); statErr != nil {
		t.Fatalf("preflight rejection must leave the original file untouched: %v", statErr)
	}
}

func TestCommitReorderBacksUpAndRenames(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(dir, "03-c.md"), []byte("c"), 0o644)
	// Working order with #3 moved up one: [01-a, 03-c, 02-b].
	working := []fileEntry{{name: "01-a.md"}, {name: "03-c.md"}, {name: "02-b.md"}}
	moved, err := commitReorder(dir, working, "STAMP")
	if err != nil {
		t.Fatal(err)
	}
	// 03-c -> 02-c and 02-b -> 03-b on disk.
	if _, err := os.Stat(filepath.Join(dir, "02-c.md")); err != nil {
		t.Fatalf("expected 02-c.md after reorder: %v", err)
	}
	if moved[filepath.Join(dir, "03-c.md")] != filepath.Join(dir, "02-c.md") {
		t.Fatalf("moved map should record 03-c -> 02-c, got %v", moved)
	}
	// A backup snapshot of the pre-reorder files exists.
	if _, err := os.Stat(filepath.Join(dir, ".backup", "STAMP", "01-a.md")); err != nil {
		t.Fatalf("expected pre-reorder backup: %v", err)
	}
}

func TestCommitReorderNoopNoBackup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("b"), 0o644)
	working := []fileEntry{{name: "01-a.md"}, {name: "02-b.md"}}
	moved, err := commitReorder(dir, working, "STAMP")
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 0 {
		t.Fatalf("no-op reorder should move nothing, got %v", moved)
	}
	if _, err := os.Stat(filepath.Join(dir, ".backup")); !os.IsNotExist(err) {
		t.Fatalf("no-op reorder should not create a backup dir")
	}
}

func TestOutlineLoadAndRows(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("one two"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	rows := o.rows()
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (2 sections + 1 loose)", len(rows))
	}
	if rows[0].entry.name != "01-a.md" || !rows[0].isSection {
		t.Fatalf("row 0 should be section 01-a.md, got %+v", rows[0])
	}
	if rows[2].entry.name != "notes.md" || rows[2].isSection {
		t.Fatalf("row 2 should be loose notes.md, got %+v", rows[2])
	}
}

func TestOutlineMoveSectionMakesDirty(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	if o.dirty() {
		t.Fatal("freshly loaded outline should not be dirty")
	}
	o.selected = 0
	o.moveSection(1) // move section 1 down
	if !o.dirty() {
		t.Fatal("after moving a section the outline should be dirty")
	}
	if o.working[0].name != "02-b.md" || o.working[1].name != "01-a.md" {
		t.Fatalf("working order should be swapped, got %v", o.working)
	}
	if o.selected != 1 {
		t.Fatalf("selection should follow the moved section to index 1, got %d", o.selected)
	}
}

func TestOutlineViewShowsTitlesCountsAndTotal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("a b"), 0o644)
	var o outlineModel
	o.load(dir, newWordCountCache())
	o.width = 60
	o.height = 12
	view := o.View()
	if !strings.Contains(view, "opening") || strings.Contains(view, "01-opening") {
		t.Fatalf("outline should show stripped title 'opening', not the raw filename:\n%s", view)
	}
	if !strings.Contains(view, "3w") {
		t.Fatalf("outline should show the per-section count '3w':\n%s", view)
	}
	if !strings.Contains(view, "5w") {
		t.Fatalf("outline header should show the project total '5w':\n%s", view)
	}
}
