package main

import (
	"strings"
	"testing"
)

func TestSectionOrder(t *testing.T) {
	cases := []struct {
		name string
		n    int
		ok   bool
	}{
		{"01-opening.md", 1, true},
		{"1-opening.md", 1, true},
		{"001-opening.md", 1, true},
		{"2-x.md", 2, true},
		{"10-x.md", 10, true},
		{"notes.md", 0, false},
		{"opening.md", 0, false},
	}
	for _, c := range cases {
		n, ok := sectionOrder(c.name)
		if n != c.n || ok != c.ok {
			t.Errorf("sectionOrder(%q) = (%d,%v), want (%d,%v)", c.name, n, ok, c.n, c.ok)
		}
	}
}

func TestSectionTitle(t *testing.T) {
	cases := map[string]string{
		"02-the-letter.md": "the letter",
		"01-opening.md":    "opening",
		"10_two_words.md":  "two words",
		"notes.md":         "notes",
	}
	for in, want := range cases {
		if got := sectionTitle(in); got != want {
			t.Errorf("sectionTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrderedSections(t *testing.T) {
	files := []fileEntry{
		{name: "10-ten.md"}, {name: "2-two.md"}, {name: "notes.md"},
		{name: "01-one.md"}, {name: "apple.md"},
	}
	sections, loose := orderedSections(files)
	var gs, gl []string
	for _, s := range sections {
		gs = append(gs, s.name)
	}
	for _, l := range loose {
		gl = append(gl, l.name)
	}
	if strings.Join(gs, ",") != "01-one.md,2-two.md,10-ten.md" {
		t.Fatalf("sections = %v, want numeric order 1,2,10", gs)
	}
	if strings.Join(gl, ",") != "apple.md,notes.md" {
		t.Fatalf("loose = %v, want alpha", gl)
	}
}

func TestIsManuscript(t *testing.T) {
	if !isManuscript([]fileEntry{{name: "notes.md"}, {name: "01-x.md"}}) {
		t.Fatal("a numbered file makes the folder a manuscript")
	}
	if isManuscript([]fileEntry{{name: "a.md"}, {name: "b.md"}}) {
		t.Fatal("no numbered files = not a manuscript")
	}
	if isManuscript([]fileEntry{{name: "Sub", isDir: true}}) {
		t.Fatal("a subdir alone is not a manuscript")
	}
}
