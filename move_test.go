package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveDocumentChapterBetweenManuscripts(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	first, _ := createManuscript(a, "A", "Alpha") // 01-alpha.md
	b := filepath.Join(root, "b")
	createManuscript(b, "B", "Beta") // 01-beta.md (distinct name → no collision)

	if err := moveDocument(a, first, b, true); err != nil {
		t.Fatal(err)
	}
	// Removed from A's manifest.
	am, _, _ := readManifest(a)
	for _, it := range am.Items {
		if it.File == first {
			t.Fatalf("%s should have been removed from A's manifest, items=%+v", first, am.Items)
		}
	}
	// Appended to B's manifest.
	bm, _, _ := readManifest(b)
	found := false
	for _, it := range bm.Items {
		if it.File == first {
			found = true
		}
	}
	if !found {
		t.Fatalf("%s should have been inserted into B's manifest, items=%+v", first, bm.Items)
	}
	if _, err := os.Stat(filepath.Join(b, first)); err != nil {
		t.Fatalf("file should have moved into B: %v", err)
	}
}

func TestSafeMoveSameVolume(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	dst := filepath.Join(dir, "sub", "a.md")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := safeMove(src, dst); err != nil {
		t.Fatalf("safeMove: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be gone after a move")
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "hello" {
		t.Fatalf("dest content = %q err=%v", b, err)
	}
}

func TestMoveDocumentLooseToCategory(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "note.md"), []byte("x"), 0o644)
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)
	if err := moveDocument(root, "note.md", cat, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cat, "note.md")); err != nil {
		t.Fatalf("file should have moved: %v", err)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsChapter(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "deleted-scene.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled") // one chapter: 01-untitled.md
	if err := moveDocument(root, "deleted-scene.md", proj, true); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	last := m.Items[len(m.Items)-1]
	if last.File != "deleted-scene.md" {
		t.Fatalf("moved file should be appended as a chapter, items=%+v", m.Items)
	}
	if last.Title != "deleted scene" { // sectionTitle de-slugs the filename
		t.Fatalf("chapter title = %q, want de-slugged 'deleted scene'", last.Title)
	}
}

func TestMoveDocumentLooseIntoManuscriptAsResource(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "res.md"), []byte("x"), 0o644)
	proj := filepath.Join(root, "novel")
	createManuscript(proj, "Novel", "Untitled")
	before, _, _ := readManifest(proj)
	if err := moveDocument(root, "res.md", proj, false); err != nil {
		t.Fatal(err)
	}
	after, _, _ := readManifest(proj)
	if len(after.Items) != len(before.Items) {
		t.Fatalf("resource move must NOT change items: before=%d after=%d", len(before.Items), len(after.Items))
	}
	if _, err := os.Stat(filepath.Join(proj, "res.md")); err != nil {
		t.Fatalf("file should be in the folder as a resource: %v", err)
	}
}

func TestMoveDocumentChapterOutRemovesFromManifest(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "novel")
	first, _ := createManuscript(proj, "Novel", "Untitled") // first == 01-untitled.md
	cat := filepath.Join(root, "cat")
	os.MkdirAll(cat, 0o755)
	if err := moveDocument(proj, first, cat, false); err != nil {
		t.Fatal(err)
	}
	m, _, _ := readManifest(proj)
	for _, it := range m.Items {
		if it.File == first {
			t.Fatal("chapter should have been removed from the source manifest")
		}
	}
	if _, err := os.Stat(filepath.Join(cat, first)); err != nil {
		t.Fatalf("file should have moved to the category: %v", err)
	}
}

func TestMoveDocumentRefusesCollisionAndNoop(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	os.MkdirAll(a, 0o755)
	os.MkdirAll(b, 0o755)
	os.WriteFile(filepath.Join(a, "x.md"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(b, "x.md"), []byte("2"), 0o644)
	if err := moveDocument(a, "x.md", b, false); err == nil {
		t.Fatal("a name collision in the destination must be refused")
	}
	if err := moveDocument(a, "x.md", a, false); err == nil {
		t.Fatal("moving into the same folder must be refused")
	}
}
