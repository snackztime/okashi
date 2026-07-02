package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoalsRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "goals.json")
	in := map[string]projectGoals{"/proj": {DailyGoal: 500, ProjectGoal: 80000, DayBaseline: 1200, Day: "2026-06-29"}}
	saveGoals(p, in)
	out := loadGoals(p)
	if out["/proj"] != in["/proj"] {
		t.Fatalf("round-trip = %+v, want %+v", out["/proj"], in["/proj"])
	}
	if len(loadGoals(filepath.Join(t.TempDir(), "missing.json"))) != 0 {
		t.Fatal("missing file should load an empty map")
	}
}

func TestApplyEnvDefaults(t *testing.T) {
	t.Setenv("OKASHI_DAILY_GOAL", "750")
	t.Setenv("OKASHI_PROJECT_GOAL", "")
	pg := projectGoals{}.applyEnvDefaults()
	if pg.DailyGoal != 750 {
		t.Fatalf("daily default = %d, want 750", pg.DailyGoal)
	}
	if pg.ProjectGoal != 0 {
		t.Fatalf("project default = %d, want 0", pg.ProjectGoal)
	}
	// A non-zero stored value is kept.
	if got := (projectGoals{DailyGoal: 300}).applyEnvDefaults().DailyGoal; got != 300 {
		t.Fatalf("stored daily kept = %d, want 300", got)
	}
}

func TestRolloverAndTodayWords(t *testing.T) {
	pg := projectGoals{Day: "2026-06-28", DayBaseline: 100}
	pg2, changed := rolloverIfNeeded(pg, 450, "2026-06-29")
	if !changed || pg2.Day != "2026-06-29" || pg2.DayBaseline != 450 {
		t.Fatalf("rollover = %+v changed=%v, want day reset + baseline 450", pg2, changed)
	}
	if _, changed := rolloverIfNeeded(pg2, 500, "2026-06-29"); changed {
		t.Fatal("same day should not change")
	}
	if w := todayWords(pg2, 470); w != 20 {
		t.Fatalf("todayWords = %d, want 20", w)
	}
	if w := todayWords(pg2, 400); w != 0 {
		t.Fatalf("todayWords below baseline = %d, want 0 (clamped)", w)
	}
}

func TestGoalsPathUnderConfig(t *testing.T) {
	if p := goalsPath(); p != "" && filepath.Base(p) != "goals.json" {
		t.Fatalf("goalsPath base = %q, want goals.json", filepath.Base(p))
	}
	_ = os.Getenv // keep os imported
}

func TestSessionGoalRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	saveGoals(path, map[string]projectGoals{"/p": {DailyGoal: 500, SessionGoalMin: 30}})
	got := loadGoals(path)
	if got["/p"].SessionGoalMin != 30 {
		t.Fatalf("SessionGoalMin round-trip = %d, want 30", got["/p"].SessionGoalMin)
	}
}
