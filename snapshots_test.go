package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func seedSnapshots(t *testing.T) (file, bak string) {
	t.Helper()
	dir := t.TempDir()
	file = filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("CURRENT"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak = filepath.Join(dir, ".okashi-bak")
	if err := os.MkdirAll(bak, 0o755); err != nil {
		t.Fatal(err)
	}
	return file, bak
}

func TestListSnapshotsOrderAndFilter(t *testing.T) {
	file, bak := seedSnapshots(t)
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(bak, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md.20260704-101500", "old")
	write("a.md.20260704-101600", "newer")
	write("b.md.20260704-101600", "other base — must be excluded")
	write("a.md.notastamp", "junk suffix — must be excluded")

	snaps := listSnapshots(file)
	if len(snaps) != 2 {
		t.Fatalf("want 2 snapshots for a.md, got %d", len(snaps))
	}
	if !snaps[0].when.After(snaps[1].when) || snaps[0].name != "a.md.20260704-101600" {
		t.Fatalf("snapshots must be newest-first: %+v", snaps)
	}
}

func TestRestoreSelectedSnapshotBacksUpCurrent(t *testing.T) {
	file, bak := seedSnapshots(t)
	if err := os.WriteFile(filepath.Join(bak, "a.md.20260704-101500"), []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := model{snapshots: newSnapshotsModel(file)}
	if len(m.snapshots.snaps) != 1 {
		t.Fatalf("want 1 snapshot, got %d", len(m.snapshots.snaps))
	}
	m.snapshots.sel = 0
	m.restoreSelectedSnapshot()

	if got, _ := os.ReadFile(file); string(got) != "OLD" {
		t.Fatalf("file after restore = %q, want OLD", got)
	}
	// The pre-restore CURRENT content must be captured as a new snapshot first.
	foundCurrent := false
	for _, sn := range listSnapshots(file) {
		if d, _ := os.ReadFile(filepath.Join(bak, sn.name)); string(d) == "CURRENT" {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Fatal("restore must snapshot the current version before overwriting")
	}
	if m.screen != screenWriting {
		t.Fatal("restore should return to the writing screen")
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "(just now)"},
		{5 * time.Minute, "(5m ago)"},
		{3 * time.Hour, "(3h ago)"},
		{50 * time.Hour, "(2d ago)"},
	}
	for _, c := range cases {
		if got := relTime(now, now.Add(-c.ago)); got != c.want {
			t.Errorf("relTime(-%s) = %q, want %q", c.ago, got, c.want)
		}
	}
	if got := relTime(time.Time{}, now); got != "" {
		t.Errorf("zero now should yield empty, got %q", got)
	}
}

func TestUpdateSnapshotsPreviewAndRestore(t *testing.T) {
	file, bak := seedSnapshots(t)
	if err := os.WriteFile(filepath.Join(bak, "a.md.20260704-101500"), []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := model{width: 80, height: 24, screen: screenSnapshots, snapshots: newSnapshotsModel(file)}

	// space → preview the selected snapshot.
	mm, _ := m.updateSnapshots(tea.KeyMsg{Type: tea.KeySpace})
	m = mm.(model)
	if !m.snapshots.previewing || m.snapshots.preview != "OLD" {
		t.Fatalf("space should preview OLD (previewing=%v preview=%q)", m.snapshots.previewing, m.snapshots.preview)
	}
	// esc → back to the list, still on the screen.
	mm, _ = m.updateSnapshots(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.snapshots.previewing || m.screen != screenSnapshots {
		t.Fatal("esc from preview should return to the list, not leave the screen")
	}
	// enter → confirm, y → restore.
	mm, _ = m.updateSnapshots(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if !m.snapshots.confirmRestore {
		t.Fatal("enter should raise the restore confirmation")
	}
	mm, _ = m.updateSnapshots(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)
	if got, _ := os.ReadFile(file); string(got) != "OLD" {
		t.Fatalf("after confirmed restore file = %q, want OLD", got)
	}
	if m.screen != screenWriting {
		t.Fatal("confirmed restore should return to writing")
	}
}

func TestSnapshotsViewNoPanic(t *testing.T) {
	file, _ := seedSnapshots(t)
	m := model{width: 80, height: 24, snapshots: newSnapshotsModel(file)} // no snapshots
	if out := m.snapshotsView(); out == "" {
		t.Fatal("empty snapshots view should still render (no-snapshots hint)")
	}
}
