package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
