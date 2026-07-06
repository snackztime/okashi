package main

import (
	"os"
	"testing"
)

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
