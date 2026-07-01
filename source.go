package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// sourceKind distinguishes the always-present primary source from user-added folders.
type sourceKind int

const (
	sourceKindPrimary sourceKind = iota
	sourceKindFolder
)

const primarySourceID = "primary"

// source is one library root okashi browses. It mirrors wicklight's Source (id/name/kind/path)
// so the two apps stay conceptually parallel, though each persists its own file. The primary's
// Path is empty and resolves to writingDir() at runtime; folder sources carry an absolute path.
type source struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Kind sourceKind `json:"kind"`
	Path string     `json:"path"`
}

// primarySource is the always-present, non-removable default root (= writingDir()).
func primarySource() source {
	name := filepath.Base(writingDir())
	if name == "." || name == "" || name == string(filepath.Separator) {
		name = "okashi"
	}
	return source{ID: primarySourceID, Name: name, Kind: sourceKindPrimary}
}

// newFolderSource builds a user folder source from an absolute path. The path is its own stable
// ID (dedup key); the display Name defaults to the folder's base name (user-editable later).
func newFolderSource(path string) source {
	name := filepath.Base(path)
	if name == "." || name == "" {
		name = path
	}
	return source{ID: path, Name: name, Kind: sourceKindFolder, Path: path}
}

// root is the resolved filesystem root: writingDir() for the primary, else the stored Path.
func (s source) root() string {
	if s.Kind == sourceKindPrimary {
		return writingDir()
	}
	return s.Path
}

// reachable reports whether the source root exists and is a directory. An unreachable folder
// source (deleted/offline) is skipped by the UI, never blocking the others (spec §1).
func (s source) reachable() bool {
	info, err := os.Stat(s.root())
	return err == nil && info.IsDir()
}

// sourcesFile is the on-disk shape of sources.json: user-added folder sources only.
type sourcesFile struct {
	Sources []source `json:"sources"`
}

// sourcesPath returns the sources store path, or "" if there is no usable user config dir
// (sources then silently reduce to the primary). Mirrors recentPath().
func sourcesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "sources.json")
}

// loadSources returns the primary source (always first) followed by the folder sources
// persisted at path. A missing/corrupt store, or an empty path, yields just [primary].
// Production callers pass sourcesPath(); tests pass a temp path (mirrors loadRecents).
func loadSources(path string) []source {
	out := []source{primarySource()}
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var f sourcesFile
	if json.Unmarshal(data, &f) != nil {
		return out
	}
	for _, s := range f.Sources {
		if s.Kind == sourceKindPrimary { // never trust a stored primary; it is synthesized
			continue
		}
		out = append(out, s)
	}
	return out
}

// saveSources persists ONLY the folder sources from all to path (the primary is synthesized at
// load, never stored). No-ops on an empty path. Production callers pass sourcesPath().
func saveSources(path string, all []source) error {
	if path == "" {
		return nil
	}
	user := make([]source, 0, len(all))
	for _, s := range all {
		if s.Kind == sourceKindPrimary {
			continue
		}
		user = append(user, s)
	}
	data, err := json.MarshalIndent(sourcesFile{Sources: user}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, data, 0o644)
}

// addSource appends s unless a source with the same ID is already present (dedup).
func addSource(all []source, s source) []source {
	for _, e := range all {
		if e.ID == s.ID {
			return all
		}
	}
	return append(all, s)
}

// removeSource drops the source with the given ID. The primary source is never removable.
func removeSource(all []source, id string) []source {
	if id == primarySourceID {
		return all
	}
	out := make([]source, 0, len(all))
	for _, s := range all {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}
