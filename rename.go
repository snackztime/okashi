package main

import (
	"fmt"
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

// planConvert numbers a plain folder's files contiguously, keeping each existing
// name as the title portion: "Chapter-00.md" -> "01-Chapter-00.md". Every file
// gains a prefix, so there are no no-ops.
func planConvert(files []fileEntry, width int) []renameOp {
	ops := make([]renameOp, 0, len(files))
	for i, e := range files {
		ops = append(ops, renameOp{from: e.name, to: fmt.Sprintf("%0*d-%s", width, i+1, e.name)})
	}
	return ops
}
