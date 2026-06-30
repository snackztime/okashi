package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanSnippet(t *testing.T) {
	raw := "---\ntitle: X\n---\n# A Heading\n\nThe *morning* fog rolled in off the [bay](http://x).\nShe had not slept.\n\nSecond para.\n"
	got := cleanSnippet(raw)
	if strings.ContainsAny(got, "#*[]`") {
		t.Fatalf("markdown not stripped: %q", got)
	}
	if !strings.HasPrefix(got, "The morning fog rolled in off the bay.") {
		t.Fatalf("prose not extracted: %q", got)
	}
	if strings.Contains(got, "Second para") {
		t.Fatalf("should stop at the first paragraph: %q", got)
	}
}

func TestCleanSnippetCaps(t *testing.T) {
	got := cleanSnippet(strings.Repeat("word ", 50))
	if utf8.RuneCountInString(got) > snippetMaxRunes+1 { // +1 for the ellipsis
		t.Fatalf("snippet too long: %d runes", utf8.RuneCountInString(got))
	}
}

func TestSnippetCacheReadsHeadAndInvalidates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	// prose in the first bytes, then a megabyte of filler.
	os.WriteFile(p, []byte("Opening line of prose.\n\n"+strings.Repeat("x", 1<<20)), 0o644)
	c := newSnippetCache()
	if got := c.get(p); got != "Opening line of prose." {
		t.Fatalf("first-bytes read: %q", got)
	}
	// rewrite → mtime changes → new snippet
	os.WriteFile(p, []byte("A different opening.\n"), 0o644)
	if got := c.get(p); got != "A different opening." {
		t.Fatalf("mtime invalidation failed: %q", got)
	}
}
