package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// firstRune returns the first rune of a glyph string (the icon itself, before
// its trailing space). Tests assert codepoints, not glyph literals, so a
// glyph-collapse regression can't make them pass vacuously.
func firstRune(s string) rune { return []rune(s)[0] }

func TestResolveIconsPlainViaEnv(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	s := resolveIcons()
	if firstRune(s.folder.ch) != 0x25B8 { // ▸
		t.Fatalf("plain folder rune = U+%04X, want U+25B8 (▸)", firstRune(s.folder.ch))
	}
	// Plain set is monochrome: no per-type color.
	if s.folder.color != "" || s.iconFor(fileEntry{name: "x.md"}).color != "" {
		t.Fatal("plain set must carry no color")
	}
}

func TestIconForGlyphAndColor(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set
	s := resolveIcons()
	cases := []struct {
		e     fileEntry
		cp    rune
		color lipgloss.Color
	}{
		{fileEntry{name: "..", isDir: true}, 0xF062, iconParentColor}, // nf-fa-arrow_up
		{fileEntry{name: "ch", isDir: true}, 0xF07B, iconFolderColor}, // nf-fa-folder
		{fileEntry{name: "a.md"}, 0xF0F6, iconTextColor},              // nf-fa-file_text_o
		{fileEntry{name: "a.pdf"}, 0xF1C1, iconPdfColor},              // nf-fa-file_pdf_o
		{fileEntry{name: "a.png"}, 0xF1C5, iconImageColor},            // nf-fa-file_image_o
		{fileEntry{name: "a.toml"}, 0xF1C9, iconCodeColor},            // nf-fa-file_code_o
		{fileEntry{name: "a.bin"}, 0xF15B, iconGenericColor},          // nf-fa-file
	}
	for _, c := range cases {
		g := s.iconFor(c.e)
		if firstRune(g.ch) != c.cp || g.color != c.color {
			t.Fatalf("iconFor(%q) = {U+%04X,%v}, want {U+%04X,%v}", c.e.name, firstRune(g.ch), g.color, c.cp, c.color)
		}
	}
	// icon() back-compat returns the glyph string.
	if firstRune(s.icon(fileEntry{name: "a.pdf"})) != 0xF1C1 {
		t.Fatalf("icon() back-compat rune = U+%04X, want U+F1C1", firstRune(s.icon(fileEntry{name: "a.pdf"})))
	}
}

// TestNerdIconsAreRealGlyphs guards against a glyph-collapse regression: every
// glyph in the nerd set must be a real Nerd Font glyph (Private Use Area,
// >= U+E000), never a plain space.
func TestNerdIconsAreRealGlyphs(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set
	s := resolveIcons()
	all := []glyph{s.folder, s.parent, s.file, s.action}
	for _, g := range s.byExt {
		all = append(all, g)
	}
	for _, g := range all {
		if r := firstRune(g.ch); r < 0xE000 {
			t.Fatalf("nerd glyph %q starts with U+%04X, want a PUA glyph (>= U+E000) — collapsed to a plain char?", g.ch, r)
		}
	}
}
