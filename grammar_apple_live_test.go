//go:build darwin && cgo && applegrammar

package main

import "testing"

func TestAppleBackendLive(t *testing.T) {
	c := appleGrammarChecker()
	if c == nil {
		t.Fatal("expected an on-device backend on macOS")
	}
	t.Logf("backend = %q available=%v", c.Name(), c.Available())
	f, err := c.Check("The cat are sleeping on the bed. Their is many problem here.")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("findings: %d", len(f))
	for _, x := range f {
		t.Logf("  line=%d rune[%d:%d] msg=%q fix=%v", x.Line, x.Start, x.End, x.Message, x.Replacements)
	}
	// FM is non-deterministic; just confirm the path runs without error.
	_ = f
}
