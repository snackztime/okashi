package main

import (
	"os"
	"path/filepath"
)

// atomicWrite writes data to path atomically: it writes to a temp file in the SAME
// directory, fsyncs it, chmods it, then renames it over path. A crash or a concurrent
// reader never sees a truncated file. The temp is dot-prefixed (hidden from the file pane)
// and removed on any error path.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // harmless no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
