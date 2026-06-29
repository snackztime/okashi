# Yazi-style File-Pane Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **TOOL-CALL WORKAROUND (carry forward):** one tool call per message, as the FIRST element
> of the reply, explanation AFTER the result. (See memory `tool-call-syntax-court-bug`.)

> **GLYPH SAFETY — CRITICAL:** every Nerd Font glyph in this plan is a Go `\uXXXX` escape
> (backslash, `u`, four hex digits), NEVER a raw glyph — raw PUA glyphs corrupt in transit.
> Type the escapes verbatim into the Go source. Go resolves `" "` to the folder glyph
> plus a space at compile time, so the source file stays pure ASCII and is always correct.
> (`▸`, `↑`, `…` are ordinary non-PUA characters and are fine to type literally.)

**Goal:** Give okashi's file pane (and home recents) yazi-style per-filetype Nerd Font icons, each colored by type from the user's Dracula palette, without changing the base palette.

**Architecture:** `iconSet` entries become glyph+color (`glyph{ch, color}`); `iconFor(e)` resolves type → glyph. A `renderIcon(g, selected)` helper colors the glyph (plain on the selected bar so the selection's white shows). The file pane and home recents render `gutter + colored-glyph + name`.

**Tech Stack:** Go, lipgloss, `charmbracelet/x/ansi` (ANSI-aware truncation, already used).

## Global Constraints

- Module `okashi`; Go invoked as `/opt/homebrew/bin/go`, gofmt as `/opt/homebrew/bin/gofmt`.
- Each glyph string keeps its **trailing space** and is a Go `\uXXXX` escape. Codepoints (first
  five confirmed from okashi's current `icons.go` comments; rest are standard nf-fa):

  | use | Go escape | Nerd Font name |
  |---|---|---|
  | folder | `" "` | nf-fa-folder |
  | parent (`..`) / arrow-up | `" "` | nf-fa-arrow_up |
  | generic file | `" "` | nf-fa-file |
  | text (`.md/.markdown/.txt/.wg`) | `" "` | nf-fa-file_text_o |
  | pdf | `" "` | nf-fa-file_pdf_o |
  | image | `" "` | nf-fa-file_image_o |
  | code/config | `" "` | nf-fa-file_code_o |
  | plus/action | `" "` | nf-fa-plus |

- Dracula icon colors: folder `#8be9fd`, parent `#6272a4`, text `#f8f8f2`, pdf `#ff5555`,
  image `#50fa7b`, code/config `#f1fa8c`, generic `#6272a4`, action = existing `accent`.
- `OKASHI_ICONS=plain`/`ascii` set: glyphs `"▸ "`/`"↑ "`/`"  "`/`"+ "` (literal, non-PUA),
  **color `""`** (rendered uncolored — monochrome, as today).
- Base palette (`accent` `#7D56F4`, `subtle`, `selectedStyle` white-on-purple) UNCHANGED.
- `gofmt -w`, `go vet ./...`, `go test ./...`, `go build ./...` clean before each commit.

---

## Task 1: Glyph+color icon model (`icons.go`, `styles.go`)

**Files:**
- Modify: `icons.go`, `styles.go`
- Test: `icons_test.go`

**Interfaces:**
- Produces: `type glyph struct { ch string; color lipgloss.Color }`; `iconSet` with fields `folder, parent, file, action glyph` and `byExt map[string]glyph`; `func (s iconSet) iconFor(e fileEntry) glyph`; `func (s iconSet) icon(e fileEntry) string` (returns `iconFor(e).ch`, back-compat). Icon-color constants in `styles.go`.

- [ ] **Step 1: Write the failing tests**

Replace the body of `icons_test.go` with (glyphs are `\uXXXX` escapes; `▸` is literal):

```go
package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestResolveIconsPlainViaEnv(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	s := resolveIcons()
	if s.folder.ch != "▸ " {
		t.Fatalf("plain folder glyph = %q, want '▸ '", s.folder.ch)
	}
	// Plain set is monochrome: no per-type color.
	if s.folder.color != "" || s.iconFor(fileEntry{name: "x.md"}).color != "" {
		t.Fatal("plain set must carry no color")
	}
}

func TestIconForGlyphAndColor(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "") // nerd set
	s := resolveIcons()
	cases := []struct {
		e     fileEntry
		ch    string
		color lipgloss.Color
	}{
		{fileEntry{name: "..", isDir: true}, " ", iconParentColor},
		{fileEntry{name: "ch", isDir: true}, " ", iconFolderColor},
		{fileEntry{name: "a.md"}, " ", iconTextColor},
		{fileEntry{name: "a.pdf"}, " ", iconPdfColor},
		{fileEntry{name: "a.png"}, " ", iconImageColor},
		{fileEntry{name: "a.toml"}, " ", iconCodeColor},
		{fileEntry{name: "a.bin"}, " ", iconGenericColor},
	}
	for _, c := range cases {
		g := s.iconFor(c.e)
		if g.ch != c.ch || g.color != c.color {
			t.Fatalf("iconFor(%q) = {%q,%v}, want {%q,%v}", c.e.name, g.ch, g.color, c.ch, c.color)
		}
	}
	// icon() back-compat returns the glyph string.
	if s.icon(fileEntry{name: "a.pdf"}) != " " {
		t.Fatalf("icon() = %q", s.icon(fileEntry{name: "a.pdf"}))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run 'TestResolveIcons|TestIconFor' 2>&1 | tail`
Expected: build errors — `glyph`, `iconFor`, `icon*Color` undefined.

- [ ] **Step 3: Add the color constants to `styles.go`**

In `styles.go`, after the `accent`/`subtle` block, add:

```go
// Per-type icon colors (Dracula palette). The base palette above is unchanged.
var (
	iconFolderColor  = lipgloss.Color("#8be9fd") // cyan
	iconParentColor  = lipgloss.Color("#6272a4") // comment grey
	iconTextColor    = lipgloss.Color("#f8f8f2") // foreground
	iconPdfColor     = lipgloss.Color("#ff5555") // red
	iconImageColor   = lipgloss.Color("#50fa7b") // green
	iconCodeColor    = lipgloss.Color("#f1fa8c") // yellow
	iconGenericColor = lipgloss.Color("#6272a4") // comment grey
)
```

- [ ] **Step 4: Rewrite `icons.go` with the glyph+color model**

Replace the entire `icons.go` with (every Nerd Font glyph is a `\uXXXX` escape):

```go
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// glyph is a file-pane icon: a Nerd Font (or ASCII) glyph plus its color. The
// ch string includes its own trailing space for alignment. color "" means the
// glyph is rendered uncolored (the plain/ascii set).
type glyph struct {
	ch    string
	color lipgloss.Color
}

// iconSet is the glyph set for the file pane and launch lists.
type iconSet struct {
	folder, parent, file, action glyph
	byExt                        map[string]glyph
}

// resolveIcons picks the glyph set once at startup. OKASHI_ICONS=plain (or
// ascii) avoids Nerd Font glyphs (and color) for terminals without a patched font.
func resolveIcons() iconSet {
	switch strings.ToLower(os.Getenv("OKASHI_ICONS")) {
	case "plain", "ascii":
		return iconSet{
			folder: glyph{ch: "▸ "},
			parent: glyph{ch: "↑ "},
			file:   glyph{ch: "  "},
			action: glyph{ch: "+ "},
			byExt:  map[string]glyph{},
		}
	}
	text := glyph{ch: " ", color: iconTextColor} // nf-fa-file_text_o
	img := glyph{ch: " ", color: iconImageColor} // nf-fa-file_image_o
	code := glyph{ch: " ", color: iconCodeColor} // nf-fa-file_code_o
	return iconSet{
		folder: glyph{ch: " ", color: iconFolderColor},  // nf-fa-folder
		parent: glyph{ch: " ", color: iconParentColor},  // nf-fa-arrow_up
		file:   glyph{ch: " ", color: iconGenericColor}, // nf-fa-file
		action: glyph{ch: " ", color: accent},           // nf-fa-plus
		byExt: map[string]glyph{
			".md":       text,
			".markdown": text,
			".txt":      text,
			".wg":       text,
			".pdf":      {ch: " ", color: iconPdfColor}, // nf-fa-file_pdf_o
			".png":      img,
			".jpg":      img,
			".jpeg":     img,
			".gif":      img,
			".webp":     img,
			".json":     code,
			".toml":     code,
			".yml":      code,
			".yaml":     code,
			".sh":       code,
		},
	}
}

// iconFor returns the glyph (ch + color) for an entry.
func (s iconSet) iconFor(e fileEntry) glyph {
	switch {
	case e.name == "..":
		return s.parent
	case e.isDir:
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}

// icon returns just the glyph string for an entry (back-compat).
func (s iconSet) icon(e fileEntry) string { return s.iconFor(e).ch }
```

- [ ] **Step 5: Run tests; gofmt; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestResolveIcons|TestIconFor' -v 2>&1 | tail -20
/opt/homebrew/bin/gofmt -w icons.go styles.go icons_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3
git add icons.go styles.go icons_test.go
git commit -m "icons: glyph+color model — curated yazi-style nerd set with per-type Dracula colors"
```

---

## Task 2: Render colored icons in the pane + home recents (`filelist.go`, `home.go`)

**Files:**
- Modify: `filelist.go` (`View`, `sectionRow`, add `renderIcon`), `home.go` (`homeRows`)
- Test: `filelist_test.go`, `home_test.go`

**Interfaces:**
- Consumes: `glyph`, `iconSet.iconFor` (Task 1), `accent`/`subtle`/`selectedStyle`.
- Produces: `func renderIcon(g glyph, selected bool) string`.

- [ ] **Step 1: Write the failing test**

Add to `filelist_test.go` (no glyphs needed — uses plain ASCII test glyphs):

```go
func TestPaneIconColoredWhenNotSelected(t *testing.T) {
	g := glyph{ch: "X", color: iconPdfColor}
	if renderIcon(g, false) == "X" {
		t.Fatal("non-selected icon should be color-wrapped (ANSI), got the bare glyph")
	}
	// Selected → plain glyph so the selection bar's white foreground applies.
	if renderIcon(g, true) != "X" {
		t.Fatalf("selected icon should be the bare glyph, got %q", renderIcon(g, true))
	}
	// No color → bare glyph (plain icon set).
	if renderIcon(glyph{ch: "Y"}, false) != "Y" {
		t.Fatalf("uncolored glyph should render bare, got %q", renderIcon(glyph{ch: "Y"}, false))
	}
}
```

(The existing `TestFilelistViewShowsIconsNoSlash` — `OKASHI_ICONS=plain` — must still pass: plain glyphs render bare, no trailing slash on dirs.)

- [ ] **Step 2: Run to verify it fails**

Run: `/opt/homebrew/bin/go test . -run TestPaneIconColored 2>&1 | tail`
Expected: build error — `renderIcon` undefined.

- [ ] **Step 3: Add `renderIcon` and use it in `filelist.go`**

Add the helper (top of `filelist.go`, after imports):

```go
// renderIcon colors a glyph by its type, except on the selected row (where the
// selection style's foreground must win) or when the glyph carries no color
// (the plain/ascii set).
func renderIcon(g glyph, selected bool) string {
	if selected || g.color == "" {
		return g.ch
	}
	return lipgloss.NewStyle().Foreground(g.color).Render(g.ch)
}
```

In `View`, replace the per-row build (the block starting `e := f.entries[i]` … through the `switch`) with:

```go
		e := f.entries[i]
		g := f.icons.iconFor(e)
		section := !e.isDir && chapterSet[e.name]
		switch {
		case i == f.selected:
			var content string
			if section {
				content = f.sectionRow(e, false) // selected: count + icon plain
			} else {
				content = " " + renderIcon(g, true) + e.name
			}
			b.WriteString(selectedStyle.Width(f.width).Render(ansi.Truncate(content, f.width, "…")))
		case e.isDir:
			row := " " + renderIcon(g, false) + lipgloss.NewStyle().Foreground(accent).Render(e.name)
			b.WriteString(ansi.Truncate(row, f.width, "…"))
		case section:
			b.WriteString(f.sectionRow(e, true))
		default:
			ext := filepath.Ext(e.name)
			icon := " " + renderIcon(g, false)
			if ext != "" && lipgloss.Width(icon+e.name) <= f.width {
				stem := icon + strings.TrimSuffix(e.name, ext)
				b.WriteString(stem + lipgloss.NewStyle().Foreground(subtle).Render(ext))
			} else {
				b.WriteString(ansi.Truncate(icon+e.name, f.width, "…"))
			}
		}
```

In `sectionRow`, the glyph is plain on the selected row (`dimCount == false`) and colored otherwise. Replace the existing `left :=` line:

```go
	g := f.icons.iconFor(e)
	left := " " + renderIcon(g, !dimCount) + f.chapterTitle(e.name)
```

(Selected section rows are rendered via `sectionRow(e, false)` → `!dimCount == true` → plain glyph, then `selectedStyle` colors the whole bar. Non-selected sections pass `true` → colored glyph.)

- [ ] **Step 4: Update `home.go` recents to use colored glyphs**

In `home.go` `homeRows`, the loop currently sets `icon` as a `string` from `icons.folder` / `icons.action` / `icons.icon(...)`. Because `iconSet` fields are now `glyph` (not `string`), those direct uses must change — resolve a `glyph` and render it:

```go
		var g glyph
		switch {
		case <existing folder branch>:
			g = icons.folder
		case <existing action/new branch>:
			g = icons.action
		default:
			g = icons.iconFor(fileEntry{name: it.label})
		}
		selected := <existing selected test for this row>
		row := "  " + renderIcon(g, selected) + it.label
		if selected {
			row = selectedStyle.Render(" " + renderIcon(g, true) + it.label + " ")
		}
```

**Implementer note:** `home.go` already has folder/action/file branching and a selected-row check — keep its exact identifiers and structure; only swap the `string` icon for a `glyph` fed through `renderIcon`. Preserve the behavior: folder→`icons.folder`, action→`icons.action`, else→`icons.iconFor(label)`; the selected row renders the glyph via `renderIcon(g, true)` inside `selectedStyle`.

- [ ] **Step 5: Run tests; gofmt; build; commit**

```bash
/opt/homebrew/bin/go test . -run 'TestPaneIcon|TestFilelistView|TestHomeView' -v 2>&1 | tail -25
/opt/homebrew/bin/gofmt -w filelist.go home.go filelist_test.go home_test.go
/opt/homebrew/bin/go vet ./... && /opt/homebrew/bin/go test ./... 2>&1 | tail -3 && /opt/homebrew/bin/go build ./...
git add filelist.go home.go filelist_test.go home_test.go
git commit -m "icons: render per-type colored glyphs in the file pane + home recents"
```

---

## Self-Review

**Spec coverage:** glyph+color model + curated nerd set + plain monochrome + color constants → Task 1; `renderIcon` + pane render (dir/section/loose/selected) + home recents → Task 2. Selected-row icon plain (selection fg wins) → both `View` and `sectionRow`/home. `icon()` back-compat retained → Task 1.

**Placeholder scan:** Task 1 fully concrete (all glyphs `\uXXXX` escapes). Task 2's `home.go` block intentionally leaves `<existing folder branch>` / `<selected test>` as "use the file's real identifiers" — the existing code already has this branching; the `renderIcon` contract + folder/action/file mapping are exact. Not a logic placeholder.

**Type consistency:** `glyph{ch, color}`, `iconFor() glyph`, `icon() string`, `renderIcon(glyph, bool) string`, and the `icon*Color` vars are defined in Task 1 and used with those exact signatures in Task 2. `iconSet` fields are `glyph` (not string) — so `home.go`'s former `icons.folder`/`icons.action` (string) become `glyph` fed through `renderIcon` (Task 2 Step 4).

**Glyph safety:** every Nerd Font (PUA) glyph in the plan is a `\uXXXX` escape; only non-PUA chars (`▸`, `↑`, `…`, `X`) appear literally.
