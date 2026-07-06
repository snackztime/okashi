package main

import (
	"os"
	"path/filepath"
	"testing"
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

func TestSynopsisRoundTripAndPrune(t *testing.T) {
	dir := t.TempDir()
	chapters := map[string]bool{"01-a.md": true, "02-b.md": true}
	syn := map[string]string{
		"01-a.md": "A line.\nA second line.",
		"02-b.md": "B synopsis.",
		"gone.md": "orphan — chapter no longer exists",
		"03-c.md": "", // empty → dropped
	}
	if err := saveSynopses(dir, syn, chapters); err != nil {
		t.Fatal(err)
	}
	got := loadSynopses(dir)
	if got["01-a.md"] != "A line.\nA second line." || got["02-b.md"] != "B synopsis." {
		t.Fatalf("round-trip: %+v", got)
	}
	if _, ok := got["gone.md"]; ok {
		t.Fatal("orphan key (not in chapter set) must be pruned on write")
	}
	if _, ok := got["03-c.md"]; ok {
		t.Fatal("empty synopsis must be dropped")
	}
	// No leftover atomic-write temp file.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != synopsisName {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only %s, got %v", synopsisName, names)
	}
}

func TestSynopsisTolerantLoad(t *testing.T) {
	dir := t.TempDir()
	if len(loadSynopses(dir)) != 0 {
		t.Fatal("missing sidecar → empty map")
	}
	if err := os.WriteFile(synopsisPath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadSynopses(dir)) != 0 {
		t.Fatal("corrupt sidecar → empty map, no error")
	}
	// Unsupported schema version → empty.
	if err := os.WriteFile(synopsisPath(dir), []byte(`{"schemaVersion":99,"synopses":{"a.md":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(loadSynopses(dir)) != 0 {
		t.Fatal("unsupported schemaVersion → empty map")
	}
}

func TestSynopsisNoHTMLEscape(t *testing.T) {
	dir := t.TempDir()
	if err := saveSynopses(dir, map[string]string{"a.md": "Tom & Jerry < > \""}, map[string]bool{"a.md": true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(synopsisPath(dir))
	if !contains(string(data), "Tom & Jerry") {
		t.Fatalf("ampersand/angle-brackets should not be HTML-escaped:\n%s", data)
	}
}
