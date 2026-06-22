package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildHomeItems(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"novel", "journal", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	recents := []string{"/abs/chapter-03.md", "/abs/note.md"}

	items := buildHomeItems(recents, dir)

	// 2 recents + 2 projects (hidden excluded) + 3 actions = 7
	if len(items) != 7 {
		t.Fatalf("want 7 items, got %d: %+v", len(items), items)
	}
	if items[0].kind != homeRecentFile || items[0].path != "/abs/chapter-03.md" {
		t.Fatalf("first item should be the most-recent file, got %+v", items[0])
	}
	if items[0].label != "chapter-03.md" {
		t.Fatalf("recent label should be the basename, got %q", items[0].label)
	}
	if items[2].kind != homeProject || items[2].label != "journal" {
		t.Fatalf("projects should be alpha-sorted after recents, got %+v", items[2])
	}
	if items[4].kind != homeNewDocument {
		t.Fatalf("5th item should be new document action, got %+v", items[4])
	}
	if items[6].kind != homeOpenOther {
		t.Fatalf("last item should be open-other, got %+v", items[6])
	}
}

func TestBuildHomeItemsEmpty(t *testing.T) {
	dir := t.TempDir() // no subdirs
	items := buildHomeItems(nil, dir)
	// Should have the 3 actions
	if len(items) != 3 || items[2].kind != homeOpenOther {
		t.Fatalf("empty state should be the 3 actions, got %+v", items)
	}
}

func TestHomeViewUsesPerExtensionIconForRecents(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set so .md has a distinct glyph
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.homeItems = []homeItem{{kind: homeRecentFile, label: "chapter.md", path: "/x/chapter.md"}}
	m.homeSelected = 0
	want := resolveIcons().icon(fileEntry{name: "chapter.md"})
	if !strings.Contains(m.homeView(), want) {
		t.Fatalf("recent row should use the .md glyph %q", want)
	}
}

func TestBuildHomeItemsHasActions(t *testing.T) {
	dir := t.TempDir()
	items := buildHomeItems(nil, dir) // no recents, no projects
	// Just the three actions, in order.
	if len(items) != 3 {
		t.Fatalf("want 3 action items, got %d: %+v", len(items), items)
	}
	want := []struct {
		kind  homeKind
		label string
	}{
		{homeNewDocument, "New document"},
		{homeNewProject, "New project"},
		{homeOpenOther, "Browse all files"},
	}
	for i, w := range want {
		if items[i].kind != w.kind || items[i].label != w.label {
			t.Fatalf("item %d = %+v, want kind %d label %q", i, items[i], w.kind, w.label)
		}
	}
}

func TestHomeRowsAndHitTest(t *testing.T) {
	items := []homeItem{
		{kind: homeProject, label: "novel", path: "/p/novel"},
		{kind: homeNewDocument, label: "New document"},
	}
	ic := resolveIcons()
	lines, itemRow, h := homeRows(items, 0, ic)
	if h != len(lines) {
		t.Fatalf("height %d != len(lines) %d", h, len(lines))
	}
	if len(itemRow) != len(items) {
		t.Fatalf("itemRow should have one entry per item, got %d", len(itemRow))
	}
	if itemRow[0] >= itemRow[1] {
		t.Fatal("item rows should be strictly increasing")
	}
	// A click on item 0's content row (centered in a tall screen) hits item 0.
	screenH := 40
	off := (screenH - h) / 2
	if got := homeItemAtY(items, 0, ic, screenH, off+itemRow[0]); got != 0 {
		t.Fatalf("hit-test at item 0's row = %d, want 0", got)
	}
	// A click on a non-item row (row 0 of content = a header/logo area) misses.
	if got := homeItemAtY(items, 0, ic, screenH, off+0); got == 0 && itemRow[0] != 0 {
		t.Fatal("hit-test on a non-item row should not return item 0")
	}
}
