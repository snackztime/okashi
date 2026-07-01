package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifestRaw(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveManifestOrderAndTitles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "opening.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "the-letter.md"), []byte("y"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":1,"title":"Windermere","items":[
		{"file":"the-letter.md","title":"The Letter"},
		{"file":"opening.md","title":"Chapter One"}]}`)
	entries := readEntries(dir)
	v := resolveManuscript(dir, entries)
	if v.source != sourceManifest || !v.ordered() {
		t.Fatalf("want manifest source, got %v", v.source)
	}
	if v.title != "Windermere" {
		t.Fatalf("title = %q, want Windermere", v.title)
	}
	// Manifest order wins over filename alpha: the-letter before opening.
	if len(v.chapters) != 2 ||
		v.chapters[0].file != "the-letter.md" || v.chapters[0].title != "The Letter" ||
		v.chapters[1].file != "opening.md" || v.chapters[1].title != "Chapter One" {
		t.Fatalf("chapters = %+v", v.chapters)
	}
}

func TestResolveManifestUnlistedIsLoose(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644)
	writeManifestRaw(t, dir, `{"schemaVersion":1,"title":"N","items":[{"file":"a.md","title":"One"}]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.chapters) != 1 || v.chapters[0].file != "a.md" {
		t.Fatalf("chapters = %+v, want only a.md", v.chapters)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("loose = %+v, want notes.md", v.loose)
	}
}

func TestResolveManifestAbsentFileOmitted(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644) // gone.md never written
	writeManifestRaw(t, dir, `{"schemaVersion":1,"title":"N","items":[
		{"file":"a.md","title":"One"},{"file":"gone.md","title":"Lost"}]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.chapters) != 1 || v.chapters[0].file != "a.md" {
		t.Fatalf("a truly-absent item must be omitted from display, got %+v", v.chapters)
	}
}

func TestResolveLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("z"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceLegacy || !v.ordered() {
		t.Fatalf("numbered files + no manifest -> legacy, got %v", v.source)
	}
	if v.chapters[0].file != "01-a.md" || v.chapters[1].file != "02-b.md" {
		t.Fatalf("legacy order = %+v, want numeric", v.chapters)
	}
	if v.chapters[0].title != "a" { // de-slugged
		t.Fatalf("legacy title = %q, want de-slugged 'a'", v.chapters[0].title)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("legacy loose = %+v", v.loose)
	}
}

func TestResolveCategory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "on-silence.md"), []byte("x"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceNone || v.ordered() {
		t.Fatalf("plain folder -> category, got %v", v.source)
	}
}

func TestResolveUnreadableManifestRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644) // numbered, but...
	writeManifestRaw(t, dir, `{"schemaVersion":2,"title":"N","items":[]}`)
	v := resolveManuscript(dir, readEntries(dir))
	// Refuse to guess: NOT legacy, files shown flat as loose, warning set.
	if v.source != sourceManifest {
		t.Fatalf("unreadable manifest still marks the folder a manuscript, got %v", v.source)
	}
	if len(v.chapters) != 0 {
		t.Fatalf("must not invent chapters from a bad manifest, got %+v", v.chapters)
	}
	if v.warning == "" {
		t.Fatal("an unreadable manifest must surface a warning")
	}
	if len(v.loose) != 1 || v.loose[0].name != "01-a.md" {
		t.Fatalf("files shown flat as loose, got %+v", v.loose)
	}
}
