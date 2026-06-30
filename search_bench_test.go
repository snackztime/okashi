package main

import "testing"

// BenchmarkSearchProject guards the per-keystroke project-search cost on a ~400-page corpus
// (40 chapters ≈ 96k words). NoMatch is the worst case (every file read + scanned); AllMatch
// shows the 200-cap short-circuiting. Both must stay well under a typing frame (~16ms).
func BenchmarkSearchProjectNoMatch(b *testing.B) {
	dir := buildBigCorpus(b, 40, 40, 60)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searchProject(dir, allowedDocExts, "qzxnomatchq", searchLimit)
	}
}

func BenchmarkSearchProjectAllMatch(b *testing.B) {
	dir := buildBigCorpus(b, 40, 40, 60)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		searchProject(dir, allowedDocExts, "lorem", searchLimit)
	}
}
