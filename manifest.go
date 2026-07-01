package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const manifestName = "manifest.json"
const manifestSchemaVersion = 1

// manifestItem is one ordered chapter entry: a bare filename + a display title.
type manifestItem struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// manifest is the shared per-manuscript order/membership/title file. okashi reads it AND
// writes it — create + chapter-title retitle today, structural edits later (see design §0).
type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Items         []manifestItem `json:"items"`
}

// hasManifest reports whether dir contains a manifest.json — wicklight's manuscript
// marker (design §4: folder with manifest = manuscript).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, manifestName))
	return err == nil
}

// readManifest loads dir/manifest.json. When the file is absent it returns
// present=false, err=nil. A present-but-unreadable manifest (malformed JSON or an
// unsupported schemaVersion) returns present=true with a non-nil err: okashi
// REFUSES to guess structure and NEVER writes the file back (design §4.1).
func readManifest(dir string) (m manifest, present bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(readErr) {
		return manifest{}, false, nil
	}
	if readErr != nil {
		return manifest{}, true, readErr
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, true, err
	}
	if m.SchemaVersion != manifestSchemaVersion {
		return manifest{}, true, fmt.Errorf(
			"unsupported manifest schemaVersion %d (okashi supports %d)",
			m.SchemaVersion, manifestSchemaVersion)
	}
	return m, true, nil
}

// writeManifest serializes m to dir/manifest.json atomically. okashi owns manifest writes for
// its own AND the wicklight-shared corpus (design §0); the schema is forced to EXACTLY v1 so
// wicklight reads it verbatim. The serialization matches wicklight's
// JSONEncoder(.prettyPrinted, .sortedKeys): alphabetically-sorted keys, 2-space indent, no
// trailing newline — so when the two apps alternate writes the NSFileVersion diff stays small
// and legible instead of a whole-file reformat (storage-spine §67-69). Go sorts map keys
// alphabetically, yielding the same items/schemaVersion/title (and file/title) order Swift emits.
func writeManifest(dir string, m manifest) error {
	m.SchemaVersion = manifestSchemaVersion
	// Force a non-nil slice so an empty manuscript serializes as `[]`, not `null` (which
	// wicklight's [ManifestItem] decode would reject).
	items := m.Items
	if items == nil {
		items = []manifestItem{}
	}
	top := map[string]any{
		"schemaVersion": m.SchemaVersion,
		"title":         m.Title,
		"items":         items,
	}
	// SetEscapeHTML(false): Swift's JSONEncoder emits &, <, > literally; Go's default escapes
	// them to their \uXXXX forms, which would churn the whole file whenever a title contains one
	// ("Tom & Jerry"). Encoder also appends a trailing '\n'; trim it to match Swift (no newline).
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(top); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, manifestName), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

// createManuscript makes a brand-new manuscript at dir: the folder, an empty first
// chapter "01-<slug>.md", and a v1 manifest listing it. firstChapter is that chapter's
// display title. It refuses to clobber an existing manifest and returns the first
// chapter's filename so the caller can open it.
func createManuscript(dir, title, firstChapter string) (string, error) {
	if hasManifest(dir) {
		return "", fmt.Errorf("a manuscript already exists at %s", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	file := "01-" + slugify(firstChapter) + ".md"
	if err := atomicWrite(filepath.Join(dir, file), []byte(""), 0o644); err != nil {
		return "", err
	}
	return file, writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         []manifestItem{{File: file, Title: firstChapter}},
	})
}

// renameChapterTitle edits ONLY the items[].title of the chapter file in dir's manifest,
// preserving order and membership; the filename is birth-stable (design §5.7). It
// read-modify-writes (re-reads immediately before writing, §0) and refuses a file that is
// not a listed chapter or a dir without a readable manifest.
func renameChapterTitle(dir, file, newTitle string) error {
	m, present, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("no manifest in %s", dir)
	}
	found := false
	for i := range m.Items {
		if m.Items[i].File == file {
			m.Items[i].Title = newTitle
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s is not a chapter of %s", file, dir)
	}
	return writeManifest(dir, m)
}
