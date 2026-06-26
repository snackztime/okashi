package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestPagerViewHeaderAndCursor(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("four five"), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 10
	view := p.View()
	if !strings.Contains(view, "/ 5w") {
		t.Fatalf("header should show the total '/ 5w':\n%s", view)
	}
	if !strings.Contains(view, "── a ──") {
		t.Fatalf("a chapter header rule should render:\n%s", view)
	}
}

func TestPagerMoveCursorClampsAndScrolls(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line\n")
	}
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte(sb.String()), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 5
	p.moveCursor(1000) // way past the end
	if p.cursor != len(p.lines)-1 {
		t.Fatalf("cursor should clamp to the last line, got %d/%d", p.cursor, len(p.lines))
	}
	if p.cursor < p.offset || p.cursor >= p.offset+p.height {
		t.Fatalf("cursor %d must be visible within [%d,%d)", p.cursor, p.offset, p.offset+p.height)
	}
	p.moveCursor(-1000)
	if p.cursor != 0 || p.offset != 0 {
		t.Fatalf("cursor/offset should return to 0, got cursor=%d offset=%d", p.cursor, p.offset)
	}
}

func TestPagerJumpTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("l0\nl1\nl2"), 0o644)
	var p pagerModel
	p.load(dir, 40)
	// lines: [0]=header(src -1), [1]=l0(src 0), [2]=l1(src 1), [3]=l2(src 2)
	p.cursor = 2
	file, src, ok := p.jumpTarget()
	if !ok || file != "01-a.md" || src != 1 {
		t.Fatalf("jumpTarget at a body line = (%q,%d,%v), want (01-a.md,1,true)", file, src, ok)
	}
	p.cursor = 0 // header line
	file, src, ok = p.jumpTarget()
	if !ok || file != "01-a.md" || src != 0 {
		t.Fatalf("jumpTarget at a header should open the section at line 0, got (%q,%d,%v)", file, src, ok)
	}
}

func TestDimMarkdownKeepsWidth(t *testing.T) {
	in := "# A *bold* idea"
	out := dimMarkdown(in)
	if lipgloss.Width(out) != lipgloss.Width(in) {
		t.Fatalf("dimMarkdown must not change the visible width: %d vs %d", lipgloss.Width(out), lipgloss.Width(in))
	}
}

func TestPagerScrollDoesNotReadFilesAfterBuild(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("some words here\n")
	}
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte(sb.String()), 0o644)
	var p pagerModel
	p.load(dir, 40)
	p.width = 50
	p.height = 5
	// Remove the source file AFTER the build. If the pager re-read on scroll/render
	// it would now see no content; instead it must work entirely from p.lines.
	os.RemoveAll(filepath.Join(dir, "01-a.md"))
	before := len(p.lines)
	p.moveCursor(10)
	p.page(1)
	_ = p.View()
	if len(p.lines) != before {
		t.Fatal("scrolling/rendering must not rebuild lines or re-read files")
	}
}
