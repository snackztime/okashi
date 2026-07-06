package main

import (
	"reflect"
	"testing"
)

func TestBeatBlocksAndTitles(t *testing.T) {
	lines := []string{
		"preamble note",    // 0 — preamble, no block
		"- Act I",          // 1
		"  - storm coming", // 2
		"  - the letter",   // 3
		"- [x] Act II",     // 4
		"* Act III",        // 5 (star marker)
	}
	got := beatBlocks(lines)
	want := []outlineBlock{{1, 4}, {4, 5}, {5, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("beatBlocks = %v, want %v", got, want)
	}
	if b, ok := blockAt(lines, 3); !ok || b != (outlineBlock{1, 4}) {
		t.Fatalf("blockAt(3) = %v,%v", b, ok)
	}
	if _, ok := blockAt(lines, 0); ok {
		t.Fatal("blockAt(0) should be preamble (ok=false)")
	}
	if beatTitle(lines[1]) != "Act I" {
		t.Fatalf("title I = %q", beatTitle(lines[1]))
	}
	if beatTitle(lines[4]) != "Act II" || !beatIsPromoted(lines[4]) {
		t.Fatalf("title II = %q promoted=%v", beatTitle(lines[4]), beatIsPromoted(lines[4]))
	}
	if beatIsPromoted(lines[1]) {
		t.Fatal("Act I is not promoted")
	}
	notes := beatNotes(lines, outlineBlock{1, 4})
	if !reflect.DeepEqual(notes, []string{"storm coming", "the letter"}) {
		t.Fatalf("notes = %v", notes)
	}
}

func TestMoveBeat(t *testing.T) {
	lines := []string{"- A", "  - a1", "- B", "  - b1", "  - b2"}
	// Move block B (cursor on its note, line 3) UP past A.
	out, nc, ok := moveBeat(lines, 3, -1)
	if !ok {
		t.Fatal("move up should apply")
	}
	want := []string{"- B", "  - b1", "  - b2", "- A", "  - a1"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("out = %v", out)
	}
	if nc != 1 { // cursor kept its offset (was b.start+1) on the moved block, now at 0+1
		t.Fatalf("newCursor = %d, want 1", nc)
	}
	// No neighbor above A → no-op.
	if _, _, ok := moveBeat(lines, 0, -1); ok {
		t.Fatal("A has no block above → no-op")
	}
	// Preamble cursor → no-op.
	if _, _, ok := moveBeat([]string{"note", "- A"}, 0, 1); ok {
		t.Fatal("preamble → no-op")
	}
}
