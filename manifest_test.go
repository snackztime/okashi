package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadManifestAbsent(t *testing.T) {
	dir := t.TempDir()
	_, present, err := readManifest(dir)
	if present || err != nil {
		t.Fatalf("absent manifest: present=%v err=%v, want false,nil", present, err)
	}
	if hasManifest(dir) {
		t.Fatal("hasManifest should be false with no manifest.json")
	}
}

func TestReadManifestValid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{
		"schemaVersion": 1, "title": "Windermere",
		"items": [ {"file":"opening.md","title":"Chapter One"},
		           {"file":"the-letter.md","title":"The Letter"} ] }`), 0o644)
	m, present, err := readManifest(dir)
	if !present || err != nil {
		t.Fatalf("valid manifest: present=%v err=%v, want true,nil", present, err)
	}
	if m.Title != "Windermere" || len(m.Items) != 2 {
		t.Fatalf("decoded = %+v, want title Windermere with 2 items", m)
	}
	if m.Items[0].File != "opening.md" || m.Items[0].Title != "Chapter One" {
		t.Fatalf("item 0 = %+v", m.Items[0])
	}
	if !hasManifest(dir) {
		t.Fatal("hasManifest should be true")
	}
}

func TestReadManifestRejectsBadVersion(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":2,"title":"X","items":[]}`), 0o644)
	_, present, err := readManifest(dir)
	if !present {
		t.Fatal("a present-but-unsupported manifest must report present=true")
	}
	if err == nil {
		t.Fatal("schemaVersion 2 must be refused with an error, not silently read")
	}
}

func TestReadManifestRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, manifestName), []byte(`{ not json`), 0o644)
	_, present, err := readManifest(dir)
	if !present || err == nil {
		t.Fatalf("malformed manifest: present=%v err=%v, want true,non-nil", present, err)
	}
}
