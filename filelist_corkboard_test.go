package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFirstProseLine(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := firstProseLine(write("a.md", "# The Keeper\n\nFor thirty-one winters Aldous…\nmore")); got != "For thirty-one winters Aldous…" {
		t.Fatalf("got %q", got)
	}
	if got := firstProseLine(write("b.md", "# Only A Heading\n\n   \n")); got != "" {
		t.Fatalf("heading/blank only should be empty, got %q", got)
	}
	if got := firstProseLine(write("c.md", "")); got != "" {
		t.Fatalf("empty file should be empty, got %q", got)
	}
	if got := firstProseLine(filepath.Join(dir, "missing.md")); got != "" {
		t.Fatalf("missing file should be empty, got %q", got)
	}
}

func TestCorkViewRendersChaptersAndResources(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"01-a.md", "02-b.md"} {
		os.WriteFile(filepath.Join(dir, f), []byte("# H\n\nOpening prose of "+f+"."), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose"), 0o644) // a loose resource
	os.MkdirAll(filepath.Join(dir, "Characters"), 0o755)                 // a resource group
	writeManifest(dir, manifest{SchemaVersion: manifestSchemaVersion, Title: "W",
		Items: []manifestItem{{File: "01-a.md", Title: "One"}, {File: "02-b.md", Title: "Two"}}})
	// Author a synopsis for chapter 1 only.
	saveSynopses(dir, map[string]string{"01-a.md": "Aldous keeps the light."}, map[string]bool{"01-a.md": true, "02-b.md": true})

	fl := newFilelist()
	fl.root = dir
	fl.width = 40
	fl.height = 40
	fl.SetDir(dir)
	fl.corkMode = true

	out := ansi.Strip(fl.View(-1, ""))
	for _, want := range []string{"One", "Two", "Aldous keeps the light.", "Opening prose of 02-b.md.", "RESOURCES", "notes.md", "Characters"} {
		if !contains(out, want) {
			t.Fatalf("cork view missing %q, got:\n%s", want, out)
		}
	}
}
