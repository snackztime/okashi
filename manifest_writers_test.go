package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateManuscriptRoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-novel")
	first, err := createManuscript(dir, "My Novel", "Untitled")
	if err != nil {
		t.Fatalf("createManuscript: %v", err)
	}
	if first != "01-untitled.md" {
		t.Fatalf("first chapter file = %q, want 01-untitled.md", first)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("first chapter not on disk: %v", err)
	}
	m, present, err := readManifest(dir)
	if err != nil || !present {
		t.Fatalf("readManifest present=%v err=%v", present, err)
	}
	if m.SchemaVersion != manifestSchemaVersion || m.Title != "My Novel" {
		t.Fatalf("manifest = %+v", m)
	}
	if len(m.Items) != 1 || m.Items[0].File != "01-untitled.md" || m.Items[0].Title != "Untitled" {
		t.Fatalf("items = %+v", m.Items)
	}
	// The resolver must see it as an ordered manifest manuscript.
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceManifest || !v.ordered() || len(v.chapters) != 1 {
		t.Fatalf("resolved view = %+v", v)
	}
}

func TestCreateManuscriptRefusesExisting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dup")
	if _, err := createManuscript(dir, "One", "Untitled"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := createManuscript(dir, "Two", "Untitled"); err == nil {
		t.Fatal("second create should refuse an existing manifest")
	}
}

func TestRenameChapterTitleChangesOnlyTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "book")
	first, _ := createManuscript(dir, "Book", "Untitled")
	if err := renameChapterTitle(dir, first, "Opening"); err != nil {
		t.Fatalf("renameChapterTitle: %v", err)
	}
	m, _, _ := readManifest(dir)
	if m.Items[0].Title != "Opening" {
		t.Fatalf("title = %q, want Opening", m.Items[0].Title)
	}
	if m.Items[0].File != first {
		t.Fatalf("filename changed to %q — must be birth-stable", m.Items[0].File)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("chapter file must NOT be renamed on disk: %v", err)
	}
}

func TestRenameChapterTitlePreservesOrderAndOthers(t *testing.T) {
	dir := t.TempDir()
	m := manifest{SchemaVersion: 1, Title: "T", Items: []manifestItem{
		{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "B"}, {File: "03-c.md", Title: "C"},
	}}
	if err := writeManifest(dir, m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	if err := renameChapterTitle(dir, "02-b.md", "Bee"); err != nil {
		t.Fatalf("renameChapterTitle: %v", err)
	}
	got, _, _ := readManifest(dir)
	want := []manifestItem{{File: "01-a.md", Title: "A"}, {File: "02-b.md", Title: "Bee"}, {File: "03-c.md", Title: "C"}}
	if len(got.Items) != 3 {
		t.Fatalf("items = %+v", got.Items)
	}
	for i := range want {
		if got.Items[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got.Items[i], want[i])
		}
	}
}

func TestRenameChapterTitleRefusesNonMember(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "book")
	createManuscript(dir, "Book", "Untitled")
	if err := renameChapterTitle(dir, "99-ghost.md", "Nope"); err == nil {
		t.Fatal("retitling a non-member should error")
	}
}

func TestWriteManifestForcesSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "X", Items: []manifestItem{{File: "01-a.md", Title: "A"}}}); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}
	m, _, err := readManifest(dir)
	if err != nil {
		t.Fatalf("readback rejected (schemaVersion not forced?): %v", err)
	}
	if m.SchemaVersion != manifestSchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", m.SchemaVersion, manifestSchemaVersion)
	}
}
