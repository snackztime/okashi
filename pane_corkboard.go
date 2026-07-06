package main

import (
	"path/filepath"
	"strings"
)

// Pane corkboard actions: staged reorder (commit behind one confirm) and immediate synopsis editing,
// driven from the left pane when it's showing a manuscript. Reorder mutates the filelist's staged
// view; commit writes the manifest (read-modify-write, preserving titles); discard reloads.

// togglePaneCork flips the left pane between the title list and the corkboard (synopsis cards).
// No-op with a status note when the current folder isn't an ordered manuscript.
func (m *model) togglePaneCork() {
	if !m.files.view.ordered() {
		m.status = "corkboard is for manuscripts — this folder has no chapters"
		return
	}
	m.files.corkMode = !m.files.corkMode
	if m.files.corkMode {
		m.status = "corkboard"
	} else {
		m.status = "chapter list"
	}
}

// paneReorder stages a chapter move from the pane (d = -1 up / +1 down), marking the reorder dirty.
func (m *model) paneReorder(d int) {
	if !m.files.view.ordered() {
		return
	}
	if m.files.moveChapter(d) {
		m.paneReorderDirty = true
		m.status = "-- REORDER -- · esc to apply or discard"
	}
}

// commitPaneReorder writes the staged chapter order to the manifest (read-modify-write: preserve
// manuscript title + per-chapter titles from disk, apply the staged order).
func (m *model) commitPaneReorder() {
	dir := m.files.dir
	mani, present, err := readManifest(dir)
	if err != nil || !present {
		m.status = "reorder failed — manifest unreadable"
		return
	}
	byFile := map[string]manifestItem{}
	for _, it := range mani.Items {
		byFile[it.File] = it
	}
	items := make([]manifestItem, 0, len(m.files.view.chapters))
	for _, ch := range m.files.view.chapters {
		if it, ok := byFile[ch.file]; ok {
			items = append(items, it)
		} else {
			items = append(items, manifestItem{File: ch.file, Title: ch.title})
		}
	}
	mani.Items = items
	if err := writeManifest(dir, mani); err != nil {
		m.status = "reorder save failed: " + err.Error()
		return
	}
	m.status = "order saved"
}

// startPaneSynopsis opens the synopsis popup for the selected chapter (pane edit).
func (m *model) startPaneSynopsis() {
	file, ok := m.files.selectedFile()
	if !ok {
		return
	}
	base := filepath.Base(file)
	if !isChapterOf(m.files.view, base) {
		m.status = "synopsis is for chapters"
		return
	}
	m.synArea = newSynopsisArea(m.files.synopses[base])
	m.synArea.Focus()
	m.paneSynEditing = true
}

// commitPaneSynopsis saves the edited synopsis to the sidecar immediately and refreshes the pane map.
func (m *model) commitPaneSynopsis() {
	m.paneSynEditing = false
	m.synArea.Blur()
	file, ok := m.files.selectedFile()
	if !ok {
		return
	}
	base := filepath.Base(file)
	if m.files.synopses == nil {
		m.files.synopses = map[string]string{}
	}
	text := strings.TrimRight(m.synArea.Value(), "\n")
	if text == "" {
		delete(m.files.synopses, base)
	} else {
		m.files.synopses[base] = text
	}
	chapterSet := map[string]bool{}
	for _, ch := range m.files.view.chapters {
		chapterSet[ch.file] = true
	}
	if err := saveSynopses(m.files.dir, m.files.synopses, chapterSet); err != nil {
		m.status = "synopsis save failed: " + err.Error()
	} else {
		m.status = "synopsis saved"
	}
}
