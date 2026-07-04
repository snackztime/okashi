package main

import (
	"strings"
	"testing"
)

func TestHeatBucket(t *testing.T) {
	cases := []struct{ v, max, want int }{
		{0, 100, 0},
		{1, 100, 1},
		{50, 100, 2},
		{100, 100, 4},
		{5, 0, 0}, // no max → 0
	}
	for _, c := range cases {
		if got := heatBucket(c.v, c.max); got != c.want {
			t.Errorf("heatBucket(%d,%d) = %d, want %d", c.v, c.max, got, c.want)
		}
	}
}

func TestSparkline(t *testing.T) {
	out := sparkline([]int{0, 10, 5})
	if out == "" {
		t.Fatal("sparkline should render")
	}
	if !strings.Contains(out, "·") {
		t.Fatal("a zero day should render as a dot")
	}
	if sparkline(nil) != "" {
		t.Fatal("empty input → empty sparkline")
	}
}

func TestHeatmapViewRenders(t *testing.T) {
	h := newHeatmapModel("Proj", map[string]int{"2026-07-01": 300, "2026-07-04": 0}, "2026-07-04")
	m := model{width: 80, height: 24, heatmap: h}
	out := m.heatmapView()
	if !strings.Contains(out, "writing history · Proj") {
		t.Fatal("heatmap header missing")
	}
	// 7 weekday rows plus chrome — must not blow up on a narrow terminal either.
	m.width, m.height = 20, 12
	if m.heatmapView() == "" {
		t.Fatal("narrow heatmap should still render")
	}
}

func TestEnterHeatmapOpensScreen(t *testing.T) {
	m := model{goalsAll: map[string]projectGoals{"": {History: map[string]int{"2026-07-04": 100}}}}
	m.enterHeatmap()
	if m.screen != screenHeatmap {
		t.Fatalf("enterHeatmap should switch to the heatmap screen, got %v", m.screen)
	}
}
