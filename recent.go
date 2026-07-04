package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const recentCap = 15

// recentEntry is one recent-files record: the path plus the editor line to resume at.
type recentEntry struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// UnmarshalJSON accepts both the new {"path":…,"line":…} object and the legacy bare-string form
// (older recent.json files stored plain path strings), so upgrades don't lose the list.
func (e *recentEntry) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		e.Path, e.Line = s, 0
		return nil
	}
	type alias recentEntry
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*e = recentEntry(a)
	return nil
}

type recentFile struct {
	Files []recentEntry `json:"files"`
}

// recentPath returns the recent-files store path, or "" if there is no usable
// user config dir (recents then silently disabled).
func recentPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "recent.json")
}

// loadRecents reads the store as paths, dropping entries whose path no longer exists.
// Missing/corrupt/empty-path all yield an empty slice.
func loadRecents(path string) []string {
	out := make([]string, 0, recentCap)
	for _, e := range readRecentsRaw(path) {
		if _, err := os.Stat(e.Path); err == nil {
			out = append(out, e.Path)
		}
	}
	return out
}

// recentLine returns the stored resume line for file, or 0 if unknown.
func recentLine(path, file string) int {
	for _, e := range readRecentsRaw(path) {
		if e.Path == file {
			return e.Line
		}
	}
	return 0
}

// addRecent prepends file to the store with its resume line (dedup by path, cap recentCap).
// No-ops on an empty path or any write error.
func addRecent(path, file string, line int) {
	if path == "" || file == "" {
		return
	}
	out := []recentEntry{{Path: file, Line: line}}
	for _, e := range readRecentsRaw(path) {
		if e.Path != file {
			out = append(out, e)
		}
	}
	if len(out) > recentCap {
		out = out[:recentCap]
	}
	data, err := json.Marshal(recentFile{Files: out})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = atomicWrite(path, data, 0o644)
}

// readRecentsRaw reads the stored entries without existence-filtering (so adding a
// new file doesn't silently drop still-pending entries).
func readRecentsRaw(path string) []recentEntry {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r recentFile
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	return r.Files
}
