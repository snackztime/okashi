package main

import (
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
