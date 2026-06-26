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

func TestPlanConvertNumbersAndKeepsNames(t *testing.T) {
	files := []fileEntry{{name: "Chapter-00.md"}, {name: "Chapter-01.md"}}
	ops := planConvert(files, 2)
	got := map[string]string{}
	for _, o := range ops {
		got[o.from] = o.to
	}
	if got["Chapter-00.md"] != "01-Chapter-00.md" || got["Chapter-01.md"] != "02-Chapter-01.md" {
		t.Fatalf("planConvert = %v, want contiguous NN- prefixes keeping the name", got)
	}
	// The result must read as a manuscript and de-slug back to the original name.
	if _, ok := sectionOrder("01-Chapter-00.md"); !ok {
		t.Fatal("converted name should parse as a numbered section")
	}
	if title := sectionTitle("01-Chapter-00.md"); title != "Chapter 00" {
		t.Fatalf("sectionTitle of converted name = %q, want 'Chapter 00'", title)
	}
}
