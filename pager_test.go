package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagerLoadBuildsHeadersThenBody(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-opening.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-the-letter.md"), []byte("four five"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("loose excluded"), 0o644) // loose
	var p pagerModel
	p.load(dir, 40)

	if len(p.lines) < 4 {
		t.Fatalf("expected at least 4 lines (2 headers + 2 body), got %d", len(p.lines))
	}
	if !p.lines[0].header || !strings.Contains(p.lines[0].text, "opening") {
		t.Fatalf("line 0 should be the 'opening' chapter header, got %+v", p.lines[0])
	}
	if p.lines[1].header || p.lines[1].file != "01-opening.md" || p.lines[1].src != 0 {
		t.Fatalf("line 1 should be opening's body line mapped to (01-opening.md,0), got %+v", p.lines[1])
	}
	// Loose file never appears.
	for _, l := range p.lines {
		if l.file == "notes.md" {
			t.Fatal("loose file must not appear in the pager")
		}
	}
	// Header lines carry their section file (so a header can jump to the section).
	if p.lines[0].file != "01-opening.md" || p.lines[0].src != -1 {
		t.Fatalf("header line should carry its file with src=-1, got %+v", p.lines[0])
	}
}

func TestPagerLoadWrapsLongLineKeepingMap(t *testing.T) {
	dir := t.TempDir()
	// One source line of 10 words; wrap to a small width so it spans several rows.
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("aa bb cc dd ee ff gg hh ii jj"), 0o644)
	var p pagerModel
	p.load(dir, 8)
	var bodyRows int
	for _, l := range p.lines {
		if l.header {
			continue
		}
		bodyRows++
		if l.file != "01-a.md" || l.src != 0 {
			t.Fatalf("every wrapped row of source line 0 must map to (01-a.md,0), got %+v", l)
		}
	}
	if bodyRows < 2 {
		t.Fatalf("a 10-word line at width 8 should wrap to >=2 rows, got %d", bodyRows)
	}
}

func TestPagerCumWordsAndTotal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644) // 3
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)     // 2
	var p pagerModel
	p.load(dir, 40)
	if p.total != 5 {
		t.Fatalf("total = %d, want 5", p.total)
	}
	last := 0
	for _, l := range p.lines {
		if l.cumWords < last {
			t.Fatalf("cumWords must be monotonic non-decreasing, saw %d after %d", l.cumWords, last)
		}
		last = l.cumWords
	}
	if last != 5 {
		t.Fatalf("final cumWords = %d, want 5 (== total)", last)
	}
}
