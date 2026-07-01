package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type pinsFile struct {
	Pins []string `json:"pins"`
}

// pinsPath is the pinned-containers store, or "" if there is no usable config dir. Mirrors recentPath().
func pinsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "pins.json")
}

// loadPins reads the store; missing/corrupt/empty-path → nil.
func loadPins(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f pinsFile
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	return f.Pins
}

// togglePin adds dir if absent (appended) or removes it if present, persists atomically, and returns
// the new list. No-ops the write on an empty path (returns the computed list either way).
func togglePin(path, dir string) []string {
	pins := loadPins(path)
	out := make([]string, 0, len(pins)+1)
	found := false
	for _, p := range pins {
		if p == dir {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found {
		out = append(out, dir)
	}
	if path != "" {
		if data, err := json.Marshal(pinsFile{Pins: out}); err == nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				_ = atomicWrite(path, data, 0o644)
			}
		}
	}
	return out
}
