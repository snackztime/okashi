package main

import (
	"path/filepath"
)

// sectionRetitle renames a numbered section to a new title, keeping its numeric
// prefix and extension and slugifying the title. "02-the-letter.md" + "the
// telegram" -> "02-the-telegram.md".
func sectionRetitle(name, newTitle string) string {
	digits, _ := splitPrefix(name)
	ext := filepath.Ext(name)
	return digits + "-" + slugify(newTitle) + ext
}

// looseRename is the new base name for a loose file: the typed name, with the
// original extension restored when the typed name omits one. "draft.md" +
// "notes" -> "notes.md"; "draft.md" + "outline.txt" -> "outline.txt".
func looseRename(oldName, typed string) string {
	if filepath.Ext(typed) == "" {
		typed += filepath.Ext(oldName)
	}
	return typed
}
