package main

import (
	"os"
	"path/filepath"
	"strings"
)

// iconSet is the glyph set for the file pane and launch lists. Each glyph
// string includes its own trailing space for alignment.
type iconSet struct {
	folder string
	parent string
	file   string
	action string
	byExt  map[string]string
}

// resolveIcons picks the glyph set once at startup. OKASHI_ICONS=plain (or
// ascii) avoids Nerd Font glyphs for terminals without a patched font.
func resolveIcons() iconSet {
	switch strings.ToLower(os.Getenv("OKASHI_ICONS")) {
	case "plain", "ascii":
		return iconSet{folder: "▸ ", parent: "↑ ", file: "  ", action: "+ ", byExt: map[string]string{}}
	}
	return iconSet{
		folder: " ", // nf-fa-folder
		parent: " ", // nf-fa-arrow_up
		file:   " ", // nf-fa-file
		action: " ", // nf-fa-plus (U+F067)
		byExt: map[string]string{
			".md":       " ", // nf-fa-file_text_o
			".markdown": " ",
			".txt":      " ",
			".wg":       " ",
		},
	}
}

// icon returns the glyph for an entry.
func (s iconSet) icon(e fileEntry) string {
	if e.name == ".." {
		return s.parent
	}
	if e.isDir {
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}
