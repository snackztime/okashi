package main

import (
	"math"
	"testing"
)

func TestSentenceStats(t *testing.T) {
	// Three sentences of 3, 3, 3 words → mean 3, stddev 0.
	mean, std := sentenceStats("One two three. Four five six! Seven eight nine?")
	if mean != 3 || std != 0 {
		t.Fatalf("uniform: mean=%v std=%v, want 3, 0", mean, std)
	}
	// Lengths 2 and 4 → mean 3, population stddev 1.
	mean, std = sentenceStats("One two. Three four five six.")
	if mean != 3 || math.Abs(std-1) > 1e-9 {
		t.Fatalf("varied: mean=%v std=%v, want 3, 1", mean, std)
	}
	// Empty → zeros, no divide-by-zero.
	if m, s := sentenceStats("   "); m != 0 || s != 0 {
		t.Fatalf("empty: mean=%v std=%v, want 0, 0", m, s)
	}
}

func TestOverusedWords(t *testing.T) {
	text := "The shadow moved. Shadow upon shadow, a shadow deep. Very very very still. " +
		"The the the the." // "the" is a stop word and must not appear
	got := overusedWords(text, 5)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 overused words, got %v", got)
	}
	// "shadow" appears 4×, "very" 3× — both should be present; "the" excluded.
	found := map[string]int{}
	for _, wf := range got {
		found[wf.word] = wf.n
	}
	if found["shadow"] != 4 {
		t.Errorf("shadow count = %d, want 4 (%v)", found["shadow"], got)
	}
	if found["very"] != 3 {
		t.Errorf("very count = %d, want 3 (crutch words must be flaggable)", found["very"])
	}
	if _, ok := found["the"]; ok {
		t.Error("stop word \"the\" must be excluded from overused")
	}
	// Ordered by count descending.
	for i := 1; i < len(got); i++ {
		if got[i-1].n < got[i].n {
			t.Errorf("not sorted by count desc: %v", got)
		}
	}
}

func TestOverusedWordsThresholdAndShortWords(t *testing.T) {
	// "shade" (5 letters, 3×) qualifies; "fog" is 3 letters (dropped); "mist" 2× is below
	// the ≥3 threshold.
	got := overusedWords("shade shade shade fog fog fog mist mist", 5)
	if len(got) != 1 || got[0].word != "shade" || got[0].n != 3 {
		t.Fatalf("got %v, want [shade:3] only", got)
	}
}

func TestComputeDocStatsReadability(t *testing.T) {
	ds := computeDocStats("One two three four. Five six seven eight.") // 8 words
	if ds.words != 8 {
		t.Fatalf("words = %d, want 8", ds.words)
	}
	if ds.readSecs != 8*60/238 {
		t.Fatalf("readSecs = %d, want %d", ds.readSecs, 8*60/238)
	}
	if ds.sentMean != 4 {
		t.Fatalf("sentMean = %v, want 4", ds.sentMean)
	}
}

func TestFmtReadTime(t *testing.T) {
	cases := map[int]string{0: "0:00", 5: "0:05", 65: "1:05", 612: "10:12"}
	for secs, want := range cases {
		if got := fmtReadTime(secs); got != want {
			t.Errorf("fmtReadTime(%d) = %q, want %q", secs, got, want)
		}
	}
}
