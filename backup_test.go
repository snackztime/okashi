package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupStampSafe(t *testing.T) {
	s := backupStamp(time.Date(2026, 6, 22, 14, 3, 5, 0, time.UTC))
	if s != "2026-06-22T14-03-05" {
		t.Fatalf("stamp = %q, want 2026-06-22T14-03-05", s)
	}
	if strings.ContainsAny(s, ":/ ") {
		t.Fatalf("stamp has filesystem-unsafe chars: %q", s)
	}
}

func TestBackupFilesCopies(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "01-a.md")
	if err := os.WriteFile(a, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := backupStamp(time.Date(2026, 6, 22, 14, 3, 0, 0, time.UTC))
	if err := backupFiles(dir, stamp, []string{a}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".backup", stamp, "01-a.md"))
	if err != nil {
		t.Fatalf("backup not written: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("backup content = %q, want hello", got)
	}
}
