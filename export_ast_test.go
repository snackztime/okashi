package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSectionBlocksAndRuns(t *testing.T) {
	src := []byte("# Dropped Title\n\nHello **bold** and *italic* world.\n\n---\n\nNext para.\n")
	blocks := parseSection(src)
	// Leading H1 dropped -> [Paragraph, SceneBreak, Paragraph]
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3 (H1 dropped): %#v", len(blocks), blocks)
	}
	p, ok := blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("block 0 should be a Paragraph, got %T", blocks[0])
	}
	var bold, italic bool
	for _, r := range p.Runs {
		if r.Text == "bold" && r.Bold {
			bold = true
		}
		if r.Text == "italic" && r.Italic {
			italic = true
		}
	}
	if !bold || !italic {
		t.Fatalf("paragraph runs should mark bold+italic: %#v", p.Runs)
	}
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("--- should be a SceneBreak, got %T", blocks[1])
	}
}

func TestParseSectionSoftBreakKeepsSpace(t *testing.T) {
	// A soft-wrapped paragraph: goldmark strips the newline; we must re-insert a space.
	blocks := parseSection([]byte("alpha\nbeta\n"))
	p := blocks[0].(Paragraph)
	var joined string
	for _, r := range p.Runs {
		joined += r.Text
	}
	if joined != "alpha beta" {
		t.Fatalf("soft break should keep a space: %q", joined)
	}
}

func TestParseSectionLoneHashIsSceneBreak(t *testing.T) {
	blocks := parseSection([]byte("para one\n\n#\n\npara two\n"))
	if _, ok := blocks[1].(SceneBreak); !ok {
		t.Fatalf("a lone # should be a SceneBreak, got %T", blocks[1])
	}
}

func TestManuscriptDocExcludesLooseAndTitlesFromFilename(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("first"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("second"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644)
	sections, _ := orderedSections([]fileEntry{
		{name: "01-opening.md"}, {name: "02-the-letter.md"}, {name: "notes.md"},
	})
	doc := manuscriptDoc(dir, sections)
	if len(doc) != 2 {
		t.Fatalf("manuscriptDoc should have 2 sections (loose excluded), got %d", len(doc))
	}
	if doc[0].Title != "opening" || doc[1].Title != "the letter" {
		t.Fatalf("titles should come from the filename: %q, %q", doc[0].Title, doc[1].Title)
	}
}
