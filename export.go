package main

import (
	"os"
	"path/filepath"
)

// runExport builds the export doc for the current scope (whole manuscript on the outline
// screen, else the current document) and writes <slug>.rtf + <slug>.pdf under <dir>/export/.
func (m *model) runExport(st ExportStyle) {
	dir := m.files.dir
	var doc ManuscriptDoc
	var title string
	if m.screen == screenOutline {
		sections, _ := orderedSections(m.files.entries)
		doc = manuscriptDoc(dir, sections)
		title = projectTitle(filepath.Base(dir))
	} else {
		if m.currentFile == "" {
			m.status = "nothing to export"
			return
		}
		dir = filepath.Dir(m.currentFile)
		base := filepath.Base(m.currentFile)
		data, err := os.ReadFile(m.currentFile)
		if err != nil {
			m.status = "export failed: " + err.Error()
			return
		}
		title = sectionTitle(base)
		doc = ManuscriptDoc{{Title: title, Blocks: parseSection(data)}}
	}
	if len(doc) == 0 {
		m.status = "nothing to export"
		return
	}

	meta := Meta{Author: os.Getenv("OKASHI_AUTHOR"), Title: title}
	outDir := filepath.Join(dir, "export")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	slug := slugify(title)
	rtfPath := filepath.Join(outDir, slug+".rtf")
	pdfPath := filepath.Join(outDir, slug+".pdf")
	if err := os.WriteFile(rtfPath, writeRTF(doc, st, meta), 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	pdfBytes, err := writePDF(doc, st, meta)
	if err != nil {
		m.status = "export failed (pdf): " + err.Error()
		return
	}
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	m.status = "exported " + slug + ".rtf + .pdf to export/"
}
