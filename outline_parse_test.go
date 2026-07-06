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
