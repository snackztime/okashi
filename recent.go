package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const recentCap = 15

type recentFile struct {
	Files []string `json:"files"`
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

// loadRecents reads the store, dropping entries whose path no longer exists.
// Missing/corrupt/empty-path all yield an empty slice.
func loadRecents(path string) []string {
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
	out := make([]string, 0, len(r.Files))
	for _, f := range r.Files {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// addRecent prepends file to the store (dedup, cap recentCap). No-ops on an
// empty path or any write error.
func addRecent(path, file string) {
	if path == "" || file == "" {
		return
	}
	existing := readRecentsRaw(path)
	out := []string{file}
	for _, f := range existing {
		if f != file {
			out = append(out, f)
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
	_ = os.WriteFile(path, data, 0o644)
}

// readRecentsRaw reads the stored list without existence-filtering (so adding a
// new file doesn't silently drop still-pending entries).
func readRecentsRaw(path string) []string {
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
