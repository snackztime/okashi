package main

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed demo/the-lighthouse
var sampleFS embed.FS

// seedMarkerPath is the once-only first-run marker (UserConfigDir/okashi/.seeded).
func seedMarkerPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", ".seeded")
}

// maybeSeedSample seeds the sample manuscript into writingDir on the very first run — only when the
// dir has no existing projects/documents — then writes the marker so it never runs again. Best-effort.
func maybeSeedSample(writingDir, marker string) {
	if marker == "" {
		return
	}
	if _, err := os.Stat(marker); err == nil {
		return // already ran once
	}
	if !dirIsEmptyish(writingDir) {
		_ = writeMarker(marker) // existing corpus — never seed, but don't re-check every launch
		return
	}
	dst := filepath.Join(writingDir, "the-lighthouse")
	_ = fs.WalkDir(sampleFS, "demo/the-lighthouse", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := sampleFS.ReadFile(p)
		if rerr != nil {
			return nil
		}
		_ = os.MkdirAll(dst, 0o755)
		_ = atomicWrite(filepath.Join(dst, filepath.Base(p)), data, 0o644)
		return nil
	})
	_ = writeMarker(marker)
}

// dirIsEmptyish reports whether dir has no non-dotfile .md files and no subdirectories.
func dirIsEmptyish(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() || strings.HasSuffix(name, ".md") {
			return false
		}
	}
	return true
}

// writeMarker creates the marker file (and its dir). Best-effort.
func writeMarker(marker string) error {
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	return atomicWrite(marker, []byte("1"), 0o644)
}
