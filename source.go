package main

import (
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
