package main

import (
	"testing"
	"time"
)

func TestSprintDisplay(t *testing.T) {
	if got := sprintDisplay(90*time.Second, false); got != "🍅 1:30" {
		t.Fatalf("work display = %q", got)
	}
	if got := sprintDisplay(30*time.Second, true); got != "☕ 0:30" {
		t.Fatalf("break display = %q", got)
	}
}

func TestAdvanceSprint(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	// work ends → break begins (5 min), still active.
	end, onBreak, active, _ := advanceSprint(base, base, false, 5*time.Minute)
	if !active || !onBreak || !end.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("work→break wrong: active=%v onBreak=%v end=%v", active, onBreak, end)
	}
	// break ends → sprint complete, inactive.
	_, _, active2, _ := advanceSprint(base, base, true, 5*time.Minute)
	if active2 {
		t.Fatalf("break end should deactivate the sprint")
	}
}

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{65 * time.Second, "1:05"},
		{9 * time.Second, "0:09"},
		{3725 * time.Second, "1:02:05"},
		{-5 * time.Second, "0:00"},
	}
	for _, c := range cases {
		if got := fmtDuration(c.d); got != c.want {
			t.Fatalf("fmtDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
