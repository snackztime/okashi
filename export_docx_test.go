package main

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// docXML unzips a .docx and returns word/document.xml as a string (helper).
func docXML(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			var b bytes.Buffer
			b.ReadFrom(rc)
			rc.Close()
			return b.String()
		}
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func hasZipEntry(data []byte, name string) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

func TestWriteDOCXStructureAndContent(t *testing.T) {
	doc := ManuscriptDoc{{
		Title: "Chapter One",
		Blocks: []Block{
			Paragraph{Runs: []Run{{Text: "Plain "}, {Text: "bold", Bold: true}, {Text: " & tail."}}},
			SceneBreak{},
			Paragraph{Runs: []Run{{Text: "After the break."}}},
		},
	}}
	out, err := writeDOCX(doc, StyleManuscript, Meta{Author: "A. Writer", Title: "Book"})
	if err != nil {
		t.Fatal(err)
	}
	// Minimal package validity — Word needs these parts.
	for _, part := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if !hasZipEntry(out, part) {
			t.Fatalf("missing required part %q", part)
		}
	}
	xml := docXML(t, out)
	if !strings.Contains(xml, "Chapter One") {
		t.Fatalf("chapter title missing:\n%s", xml)
	}
	if !strings.Contains(xml, "<w:b/>") {
		t.Fatalf("bold run should emit <w:b/>")
	}
	if !strings.Contains(xml, "&amp;") {
		t.Fatalf("& must be XML-escaped to &amp;")
	}
	if !strings.Contains(xml, "After the break.") {
		t.Fatalf("post-scene-break text missing")
	}
	if !strings.Contains(xml, ">#<") { // the scene-break dinkus
		t.Fatalf("scene break # missing")
	}
}
