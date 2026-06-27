package main

import (
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

// manifest is inkmere's per-manuscript order/membership/title file. okashi reads
// it and NEVER writes it (see the reconciliation design, §3.1).
type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Items         []manifestItem `json:"items"`
}

// hasManifest reports whether dir contains a manifest.json — inkmere's manuscript
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
