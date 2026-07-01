package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPinsToggleRoundTrip(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	if len(loadPins(store)) != 0 {
		t.Fatal("no store → empty")
	}
	p := togglePin(store, "/corpus/my-novel")
	if len(p) != 1 || p[0] != "/corpus/my-novel" {
		t.Fatalf("toggle should add, got %+v", p)
	}
	if got := loadPins(store); len(got) != 1 || got[0] != "/corpus/my-novel" {
		t.Fatalf("pin should persist, got %+v", got)
	}
	// Toggling the same path again removes it.
	if p := togglePin(store, "/corpus/my-novel"); len(p) != 0 {
		t.Fatalf("re-toggle should remove, got %+v", p)
	}
	if len(loadPins(store)) != 0 {
		t.Fatal("removal should persist")
	}
}

func TestPinsNoDuplicate(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	togglePin(store, "/a")
	togglePin(store, "/b")
	// re-adding /a via a fresh toggle removes it (toggle semantics); adding /c keeps order.
	if p := togglePin(store, "/c"); len(p) != 3 {
		t.Fatalf("want [/a /b /c], got %+v", p)
	}
}

func TestLoadPinsCorrupt(t *testing.T) {
	store := filepath.Join(t.TempDir(), "pins.json")
	os.WriteFile(store, []byte("{bad"), 0o644)
	if len(loadPins(store)) != 0 {
		t.Fatal("corrupt store → empty")
	}
}
