package main

import (
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
