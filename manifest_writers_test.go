package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteManifestMatchesWicklightSortedKeys locks okashi's serialization to wicklight's
// JSONEncoder(.sortedKeys) key order + no trailing newline, so the shared corpus doesn't churn
// when the two apps alternate writes (storage-spine §67-69). Struct-order marshaling would put
// schemaVersion first; sorted keys put items first.
func TestWriteManifestMatchesWicklightSortedKeys(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "Novel A", Items: []manifestItem{{File: "a.md", Title: "One"}}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	iItems := strings.Index(s, `"items"`)
	iSchema := strings.Index(s, `"schemaVersion"`)
	if iItems < 0 || iSchema < 0 || iItems > iSchema {
		t.Fatalf("keys not sorted (want items before schemaVersion):\n%s", s)
	}
	if iFile := strings.Index(s, `"file"`); iFile < 0 || iFile > iSchema {
		t.Fatalf("item key file should precede top-level schemaVersion (items block first):\n%s", s)
	}
	if strings.HasSuffix(s, "\n") {
		t.Fatalf("manifest must have no trailing newline (matches wicklight JSONEncoder)")
	}
	// Still round-trips through the reader.
	if _, present, rerr := readManifest(dir); rerr != nil || !present {
		t.Fatalf("sorted-key manifest must round-trip: present=%v err=%v", present, rerr)
	}
}

// TestWriteManifestNoHTMLEscaping guards that okashi emits &, <, > literally like Swift's
// JSONEncoder (not Go's default \uXXXX escaping) — else a title with "&" churns the whole file
// on every app handoff, defeating the sorted-keys parity fix. Round-trips to the exact strings.
func TestWriteManifestNoHTMLEscaping(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "Tom & Jerry", Items: []manifestItem{{File: "a.md", Title: "A < B > C"}}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, manifestName))
	s := string(b)
	// With Go's default HTML escaping ON, the file would hold "Tom & Jerry" and the literal
	// substrings below would be ABSENT — so these presence checks fail exactly when escaping leaks.
	if !strings.Contains(s, "Tom & Jerry") || !strings.Contains(s, "A < B > C") {
		t.Fatalf("titles must be emitted literally (no HTML escaping):\n%s", s)
	}
	if strings.Contains(s, `u0026`) || strings.Contains(s, `u003c`) || strings.Contains(s, `u003e`) {
		t.Fatalf("found an HTML \\uXXXX escape — must emit &/</> literally:\n%s", s)
	}
	m, _, _ := readManifest(dir)
	if m.Title != "Tom & Jerry" || m.Items[0].Title != "A < B > C" {
		t.Fatalf("round-trip mismatch: %+v", m)
	}
}

// TestWriteManifestEmptyItemsIsArray guards the nil-slice→[] fix: an empty manuscript must
// serialize items as `[]`, never `null` (wicklight decodes a JSON array).
func TestWriteManifestEmptyItemsIsArray(t *testing.T) {
	dir := t.TempDir()
	if err := writeManifest(dir, manifest{Title: "Empty"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, manifestName))
	if strings.Contains(string(b), "null") {
		t.Fatalf("empty items must serialize as [], not null:\n%s", b)
	}
}

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

func TestStartRenameManifestChapterRetitles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "book")
	first, _ := createManuscript(dir, "Book", "Untitled")

	m := initialModel() // same construction as Task 2 (smoke_test.go:369)
	m.files.root = ""
	m.files.SetDir(dir)
	m.files.selectName(first)

	m.startRename()
	if !m.renaming || !m.renameTarget.manifestChapter {
		t.Fatalf("startRename on a manifest chapter should begin a manifestChapter rename; target=%+v renaming=%v", m.renameTarget, m.renaming)
	}
	if got := m.nameInput.Value(); got != "Untitled" {
		t.Fatalf("prefill = %q, want the current chapter title 'Untitled'", got)
	}

	m.nameInput.SetValue("The Opening")
	m.confirmRename()

	mf, _, _ := readManifest(dir)
	if mf.Items[0].Title != "The Opening" {
		t.Fatalf("title = %q, want 'The Opening'", mf.Items[0].Title)
	}
	if mf.Items[0].File != first {
		t.Fatalf("filename changed to %q — must stay birth-stable", mf.Items[0].File)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("chapter file must not be renamed on disk: %v", err)
	}
}

func TestConfirmCreateNewProjectMakesManuscript(t *testing.T) {
	root := t.TempDir()
	m := initialModel() // constructor used by all model tests (e.g. smoke_test.go:369)
	m.files.root = ""   // allow an arbitrary temp dir as root (smoke_test.go pattern)
	m.files.SetDir(root)
	m.creatingFile = true
	m.creatingFolder = true // the New-Project action
	m.creatingInPane = true
	m.nameInput.SetValue("My Novel")

	m.confirmCreate()

	dir := filepath.Join(root, "My Novel")
	if !hasManifest(dir) {
		t.Fatalf("New Project should create a manifest at %s", dir)
	}
	if m.files.dir != dir {
		t.Fatalf("pane dir = %q, want %q (should enter the project)", m.files.dir, dir)
	}
	if filepath.Base(m.currentFile) != "01-untitled.md" {
		t.Fatalf("currentFile = %q, want the opened first chapter", m.currentFile)
	}
	if m.focus != focusEditor {
		t.Fatalf("focus = %v, want focusEditor (land writing)", m.focus)
	}
}
