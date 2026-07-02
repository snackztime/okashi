package main

import (
	"testing"
	"time"
)

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
