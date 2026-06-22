package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type homeKind int

const (
	homeRecentFile homeKind = iota
	homeProject
	homeOpenOther
)

// homeItem is one selectable row on the launch screen.
type homeItem struct {
	kind  homeKind
	label string // basename / project name / action label
	path  string // file path, project dir, or "" for the action
}

// buildHomeItems composes the launch list: recent files (most-recent-first),
// then project folders (immediate non-hidden subdirs of projectsDir, alpha),
// then a final "Open another folder…" action.
func buildHomeItems(recents []string, projectsDir string) []homeItem {
	var items []homeItem
	for _, p := range recents {
		items = append(items, homeItem{kind: homeRecentFile, label: filepath.Base(p), path: p})
	}

	if entries, err := os.ReadDir(projectsDir); err == nil {
		var dirs []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Strings(dirs)
		for _, d := range dirs {
			items = append(items, homeItem{kind: homeProject, label: d, path: filepath.Join(projectsDir, d)})
		}
	}

	items = append(items, homeItem{kind: homeOpenOther, label: "Open another folder…"})
	return items
}
