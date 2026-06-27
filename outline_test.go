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
