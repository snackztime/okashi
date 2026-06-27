package main

import "testing"

func TestSectionRetitleKeepsPrefixAndExt(t *testing.T) {
	cases := []struct{ name, title, want string }{
		{"02-the-letter.md", "the telegram", "02-the-telegram.md"},
		{"01-opening.md", "A New Dawn", "01-a-new-dawn.md"},
		{"003-x.md", "scene two", "003-scene-two.md"},
	}
	for _, c := range cases {
		if got := sectionRetitle(c.name, c.title); got != c.want {
			t.Errorf("sectionRetitle(%q,%q) = %q, want %q", c.name, c.title, got, c.want)
		}
	}
}

func TestLooseRenameKeepsExtensionWhenOmitted(t *testing.T) {
	if got := looseRename("draft.md", "notes"); got != "notes.md" {
		t.Errorf("looseRename kept ext: got %q, want notes.md", got)
	}
	if got := looseRename("draft.md", "outline.txt"); got != "outline.txt" {
		t.Errorf("looseRename with explicit ext: got %q, want outline.txt", got)
	}
	if got := looseRename("README", "NOTES"); got != "NOTES" {
		t.Errorf("looseRename no original ext: got %q, want NOTES", got)
	}
}
