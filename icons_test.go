package main

import (
	"strings"
	"testing"
)

func TestResolveIconsPlainViaEnv(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	s := resolveIcons()
	if len(s.byExt) != 0 {
		t.Fatal("plain set should have no per-extension icons")
	}
	if s.icon(fileEntry{name: "a.md"}) != s.file {
		t.Fatal("plain: a file should use the generic file icon")
	}
}

func TestNerdIconsAreRealGlyphs(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "")
	s := resolveIcons()
	for name, g := range map[string]string{"folder": s.folder, "file": s.file, "parent": s.parent, ".md": s.byExt[".md"]} {
		r := []rune(strings.TrimSpace(g))
		if len(r) == 0 {
			t.Fatalf("%s icon is blank — Nerd glyph missing", name)
		}
		if r[0] < 0xE000 {
			t.Fatalf("%s icon first rune U+%04X is not a Nerd Font private-use glyph", name, r[0])
		}
	}
}

func TestIconMapping(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set
	s := resolveIcons()
	if s.icon(fileEntry{name: ".."}) != s.parent {
		t.Fatal(".. should use the parent icon")
	}
	if s.icon(fileEntry{name: "proj", isDir: true}) != s.folder {
		t.Fatal("a dir should use the folder icon")
	}
	if s.icon(fileEntry{name: "ch.md"}) != s.byExt[".md"] {
		t.Fatal(".md should use its mapped icon")
	}
	if s.icon(fileEntry{name: "x.unknown"}) != s.file {
		t.Fatal("unknown ext should use the generic file icon")
	}
}
