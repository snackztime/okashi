package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildBigCorpus writes `chapters` numbered chapter files, each `paras`
// paragraphs of `wordsPerPara` words, into a temp dir. ~400 pages at the
// defaults below (40*40*60 = 96,000 words ≈ 384 pages of 250 words).
func buildBigCorpus(tb testing.TB, chapters, paras, wordsPerPara int) string {
	tb.Helper()
	dir := tb.TempDir()
	oneWord := "lorem"
	line := strings.TrimSpace(strings.Repeat(oneWord+" ", wordsPerPara)) // one source line = one paragraph
	var body strings.Builder
	body.Grow(paras * (len(line) + 2))
	for p := 0; p < paras; p++ {
		if p > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString(line)
	}
	chapterText := body.String()
	for c := 1; c <= chapters; c++ {
		name := fmt.Sprintf("%02d-chapter.md", c)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(chapterText), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return dir
}

const (
	benchChapters = 40
	benchParas    = 40
	benchWords    = 60
	benchWidth    = 72
	benchHeight   = 40
)

// TestPagerCorpusStats reports the size of the synthetic corpus so the
// benchmark numbers have context. Run with -v.
func TestPagerCorpusStats(t *testing.T) {
	dir := buildBigCorpus(t, benchChapters, benchParas, benchWords)
	var p pagerModel
	p.load(dir, benchWidth)
	approxPages := p.total / 250
	t.Logf("corpus: %d chapters · %d words · %d wrapped display lines · ~%d pages (width %d)",
		benchChapters, p.total, len(p.lines), approxPages, benchWidth)
}

// BenchmarkPagerLoad measures the ONE-TIME cost of opening the pager:
// read every chapter from disk + wrap every line + build the source map.
func BenchmarkPagerLoad(b *testing.B) {
	dir := buildBigCorpus(b, benchChapters, benchParas, benchWords)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var p pagerModel
		p.load(dir, benchWidth)
		if len(p.lines) == 0 {
			b.Fatal("empty load")
		}
	}
}

// BenchmarkPagerViewMiddle measures a single frame render with the viewport
// parked in the MIDDLE of the document. This is the per-frame cost Bubble Tea
// pays; it must be O(height), independent of document size.
func BenchmarkPagerViewMiddle(b *testing.B) {
	dir := buildBigCorpus(b, benchChapters, benchParas, benchWords)
	var p pagerModel
	p.load(dir, benchWidth)
	p.height = benchHeight
	p.cursor = len(p.lines) / 2
	p.ensureVisible()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.View()
	}
}

// BenchmarkPagerViewTop renders at offset 0 — compare against ViewMiddle to
// confirm render cost does NOT depend on scroll position (true O(height)).
func BenchmarkPagerViewTop(b *testing.B) {
	dir := buildBigCorpus(b, benchChapters, benchParas, benchWords)
	var p pagerModel
	p.load(dir, benchWidth)
	p.height = benchHeight
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.View()
	}
}

// BenchmarkPagerScrollStep measures one keystroke of scrolling: advance the
// cursor a line (re-clamping offset) and render the frame — the full per-input
// cost while reading through the draft.
func BenchmarkPagerScrollStep(b *testing.B) {
	dir := buildBigCorpus(b, benchChapters, benchParas, benchWords)
	var p pagerModel
	p.load(dir, benchWidth)
	p.height = benchHeight
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.moveCursor(1)
		if p.cursor >= len(p.lines)-1 {
			p.cursor, p.offset = 0, 0 // wrap back to the top, keep scrolling
		}
		_ = p.View()
	}
}
