package main

import (
	"os"
	"path/filepath"
	"time"
)

// backupStamp formats t as a filesystem-safe directory name (no colons/slashes).
func backupStamp(t time.Time) string {
	return t.Format("2006-01-02T15-04-05")
}

// backupFiles copies each path into <projectDir>/.backup/<stamp>/ (flat, by base
// name). Used to snapshot files before a destructive op (reorder/delete).
func backupFiles(projectDir, stamp string, paths []string) error {
	dest := filepath.Join(projectDir, ".backup", stamp)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(dest, filepath.Base(p)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
