package main

import "path/filepath"

type manuscriptSource int

const (
	sourceNone     manuscriptSource = iota // category: no manifest, no numbered files
	sourceManifest                         // manifest.json present and readable
	sourceLegacy                           // no manifest, numbered files (read-only fallback)
)

// chapterRef is one ordered chapter in a manuscript view: a base filename plus its
// resolved display title (from the manifest, or de-slugged in the legacy path).
type chapterRef struct {
	file  string
	title string
}

// manuscriptView is a folder's resolved structure. One resolver feeds the sidebar,
// outline, pager, and export so they never disagree (design §4).
type manuscriptView struct {
	source   manuscriptSource
	title    string
	chapters []chapterRef
	loose    []fileEntry // resources / loose files
	warning  string      // non-empty when a manifest was present but unreadable (§4.1)
}

// ordered reports whether the folder renders as an ordered manuscript (manifest or
// legacy). Categories and the resolver's never-ordered states return false.
func (v manuscriptView) ordered() bool {
	return v.source == sourceManifest || v.source == sourceLegacy
}

// filterFiles drops directories, keeping document fileEntry values.
func filterFiles(entries []fileEntry) []fileEntry {
	var out []fileEntry
	for _, e := range entries {
		if !e.isDir {
			out = append(out, e)
		}
	}
	return out
}

// docEntries returns the non-dir document entries (loose display order: alpha,
// matching orderedSections' loose sort) for category folders.
func docEntries(entries []fileEntry) []fileEntry {
	_, loose := orderedSections(filterFiles(entries))
	return loose
}

// isManuscript reports whether dir has a manifest.json — wicklight's manuscript
// marker (design §4). This is the contract-precise definition; use
// hasNumberedSections for the legacy-prefix heuristic.
func isManuscript(dir string) bool {
	return hasManifest(dir)
}

// resolveManuscript decides a folder's structure into one of three mutually
// exclusive states (design §4). A readable manifest is the SOLE source of order,
// titles, and membership (filenames opaque). Absent a manifest, numbered files fall
// back to legacy prefix ordering (read-only). Otherwise the folder is a category.
func resolveManuscript(dir string, entries []fileEntry) manuscriptView {
	m, present, err := readManifest(dir)
	if present {
		if err != nil {
			// Refuse to guess (§4.1): show ALL .md files flat as loose, prose still editable.
			// Use filterFiles (not docEntries) so numbered files are not stripped out.
			return manuscriptView{
				source:  sourceManifest,
				title:   projectTitle(filepath.Base(dir)),
				loose:   filterFiles(entries),
				warning: err.Error(),
			}
		}
		return manifestView(dir, m, entries)
	}
	if hasNumberedSections(entries) {
		sections, loose := orderedSections(filterFiles(entries))
		chapters := make([]chapterRef, 0, len(sections))
		for _, s := range sections {
			chapters = append(chapters, chapterRef{file: s.name, title: sectionTitle(s.name)})
		}
		return manuscriptView{
			source:   sourceLegacy,
			title:    projectTitle(filepath.Base(dir)),
			chapters: chapters,
			loose:    loose,
		}
	}
	return manuscriptView{
		source: sourceNone,
		title:  projectTitle(filepath.Base(dir)),
		loose:  docEntries(entries),
	}
}

// isChapterOf reports whether name is a chapter of the given manuscript view (a
// manifest item, or — in a legacy folder — a numbered section). Manifest chapters
// are not renamable; legacy chapters retitle via sectionRetitle (resolved O1).
func isChapterOf(v manuscriptView, name string) bool {
	for _, c := range v.chapters {
		if c.file == name {
			return true
		}
	}
	return false
}

// manifestView projects a readable manifest onto on-disk entries: items in manifest
// order whose file exists become chapters; every other .md is loose (a Resource).
func manifestView(dir string, m manifest, entries []fileEntry) manuscriptView {
	onDisk := map[string]bool{}
	for _, e := range entries {
		if !e.isDir {
			onDisk[e.name] = true
		}
	}
	listed := map[string]bool{}
	chapters := make([]chapterRef, 0, len(m.Items))
	for _, it := range m.Items {
		listed[it.File] = true
		if onDisk[it.File] { // omit a truly-absent file from display (§4.2)
			chapters = append(chapters, chapterRef{file: it.File, title: it.Title})
		}
	}
	var loose []fileEntry
	for _, e := range entries {
		if !e.isDir && !listed[e.name] {
			loose = append(loose, e)
		}
	}
	title := m.Title
	if title == "" {
		title = projectTitle(filepath.Base(dir))
	}
	return manuscriptView{source: sourceManifest, title: title, chapters: chapters, loose: loose}
}
