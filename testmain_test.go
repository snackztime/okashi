package main

import (
	"os"
	"testing"
)

// TestMain isolates the user config dir for the whole test binary so tests never
// read or write the real ~/Library/.../okashi (goals.json, recent.json).
// os.UserConfigDir() derives from HOME on macOS and XDG_CONFIG_HOME on Linux;
// pointing both at a temp dir redirects goalsPath()/recentPath().
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "okashi-test-config")
	if err == nil {
		os.Setenv("HOME", tmp)
		os.Setenv("XDG_CONFIG_HOME", tmp)
	}
	code := m.Run()
	if tmp != "" {
		os.RemoveAll(tmp)
	}
	os.Exit(code)
}
