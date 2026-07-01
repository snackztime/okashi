package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrimarySourceResolvesToWritingDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)
	p := primarySource()
	if p.ID != primarySourceID {
		t.Fatalf("primary ID = %q, want %q", p.ID, primarySourceID)
	}
	if p.Kind != sourceKindPrimary {
		t.Fatalf("primary Kind = %v, want primary", p.Kind)
	}
	if p.Path != "" {
		t.Fatalf("primary stored Path must be empty (resolved at runtime), got %q", p.Path)
	}
	if p.root() != dir {
		t.Fatalf("primary root() = %q, want writingDir() %q", p.root(), dir)
	}
}

func TestNewFolderSource(t *testing.T) {
	s := newFolderSource("/tmp/writing/Dropbox Novels")
	if s.Kind != sourceKindFolder {
		t.Fatalf("Kind = %v, want folder", s.Kind)
	}
	if s.Path != "/tmp/writing/Dropbox Novels" || s.root() != "/tmp/writing/Dropbox Novels" {
		t.Fatalf("Path/root = %q/%q", s.Path, s.root())
	}
	if s.Name != "Dropbox Novels" {
		t.Fatalf("Name = %q, want the folder base name", s.Name)
	}
	if s.ID == "" || s.ID == primarySourceID {
		t.Fatalf("folder source needs a stable non-primary ID, got %q", s.ID)
	}
}

func TestSourceReachable(t *testing.T) {
	dir := t.TempDir()
	if !newFolderSource(dir).reachable() {
		t.Fatal("an existing dir must be reachable")
	}
	if newFolderSource(filepath.Join(dir, "gone")).reachable() {
		t.Fatal("a missing dir must be unreachable")
	}
	// A file (not a dir) is not a reachable source root.
	f := filepath.Join(dir, "a.md")
	os.WriteFile(f, []byte("x"), 0o644)
	if newFolderSource(f).reachable() {
		t.Fatal("a plain file is not a reachable source root")
	}
}

func TestLoadSourcesNoFileIsPrimaryOnly(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	got := loadSources(store)
	if len(got) != 1 || got[0].Kind != sourceKindPrimary {
		t.Fatalf("no store should yield exactly [primary], got %+v", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	folder := newFolderSource(t.TempDir())
	all := []source{primarySource(), folder}
	if err := saveSources(store, all); err != nil {
		t.Fatalf("saveSources: %v", err)
	}
	got := loadSources(store)
	if len(got) != 2 {
		t.Fatalf("want [primary, folder], got %+v", got)
	}
	if got[0].Kind != sourceKindPrimary {
		t.Fatalf("primary must be first, got %+v", got[0])
	}
	if got[1].ID != folder.ID || got[1].Kind != sourceKindFolder {
		t.Fatalf("folder source not restored: %+v", got[1])
	}
}

func TestSaveSourcesDoesNotPersistPrimary(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	if err := saveSources(store, []source{primarySource()}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("store should exist: %v", err)
	}
	if strings.Contains(string(data), `"primary"`) {
		t.Fatalf("primary must not be serialized:\n%s", data)
	}
}

func TestAddSourceDedupsByID(t *testing.T) {
	base := []source{primarySource()}
	f := newFolderSource("/tmp/x")
	one := addSource(base, f)
	two := addSource(one, newFolderSource("/tmp/x")) // same path → same ID
	if len(two) != 2 {
		t.Fatalf("adding the same path twice must dedup, got %d sources", len(two))
	}
}

func TestRemoveSourceKeepsPrimary(t *testing.T) {
	all := []source{primarySource(), newFolderSource("/tmp/x")}
	all = removeSource(all, "/tmp/x")
	if len(all) != 1 || all[0].Kind != sourceKindPrimary {
		t.Fatalf("removing a folder should leave [primary], got %+v", all)
	}
	all = removeSource(all, primarySourceID) // must be a no-op
	if len(all) != 1 {
		t.Fatalf("the primary source must not be removable, got %+v", all)
	}
}

func TestLoadSourcesCorruptFileIsPrimaryOnly(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sources.json")
	os.WriteFile(store, []byte("{not json"), 0o644)
	got := loadSources(store)
	if len(got) != 1 || got[0].Kind != sourceKindPrimary {
		t.Fatalf("corrupt store should degrade to [primary], got %+v", got)
	}
}
