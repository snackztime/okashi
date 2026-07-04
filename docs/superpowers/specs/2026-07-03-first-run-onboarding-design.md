# First-run onboarding — design

**Date:** 2026-07-03
**Status:** Approved (direction) — Tier-1 adoption item 1.2 from the full review.
**Context:** First launch drops the user into an empty home rendering `(empty)` with no guidance on
how to make a manuscript — the review's "cheapest high-impact win." Meanwhile `demo/the-lighthouse`
(a real 3-chapter sample) sits unused in the repo. Seed it on first run so the first thing a new
user sees is a real manuscript to open, read, preview, and export — teaching the model by example.

**Design decision — auto-seed on first run** (not a "press X to load sample" prompt): higher impact
(the app "just works" with content), and safe because it is marker-gated, only seeds into an empty
writing dir, and never overwrites. If the user deletes everything, the improved empty-state guides
them.

---

## 1. Embed the sample

The `//go:embed` pattern is already used (fonts.go, spell.go). Add (in a new `onboarding.go`, package
main):
```go
import "embed"

//go:embed demo/the-lighthouse
var sampleFS embed.FS
```
The embedded tree is `demo/the-lighthouse/{manifest.json,01-the-keeper.md,02-the-fog.md,03-the-light.md}`.

## 2. Seed once, into an empty writing dir

```go
// seedMarkerPath is the once-only first-run marker (UserConfigDir/okashi/.seeded).
func seedMarkerPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", ".seeded")
}

// maybeSeedSample seeds the sample manuscript into writingDir on the very first run, but only when
// the dir has no existing projects/documents — then writes the marker so it never runs again.
// Path-parameterized (like the config stores) so tests pass temp paths.
func maybeSeedSample(writingDir, marker string) {
	if marker == "" {
		return
	}
	if _, err := os.Stat(marker); err == nil {
		return // already ran once
	}
	if !dirIsEmptyish(writingDir) {
		_ = writeMarker(marker) // existing corpus — never seed, but don't re-check every launch
		return
	}
	// Copy the embedded sample into <writingDir>/the-lighthouse/.
	dst := filepath.Join(writingDir, "the-lighthouse")
	_ = fs.WalkDir(sampleFS, "demo/the-lighthouse", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := sampleFS.ReadFile(p)
		if rerr != nil {
			return nil
		}
		_ = os.MkdirAll(dst, 0o755)
		_ = atomicWrite(filepath.Join(dst, filepath.Base(p)), data, 0o644)
		return nil
	})
	_ = writeMarker(marker)
}

// dirIsEmptyish reports whether writingDir has no non-dotfile .md files and no subdirectories
// (i.e. no projects or loose documents) — safe to seed.
func dirIsEmptyish(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true // can't read → treat as empty (seed is best-effort)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() || strings.HasSuffix(name, ".md") {
			return false
		}
	}
	return true
}

// writeMarker creates the marker file (and its dir). Best-effort.
func writeMarker(marker string) error {
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	return atomicWrite(marker, []byte("1"), 0o644)
}
```
Call `maybeSeedSample(writingDir(), seedMarkerPath())` at the START of `initialModel()`, before it
reads the writing dir to build the home — so the seeded project appears on the first frame. (Wire:
one line at the top of `initialModel`.)

## 3. Empty-home affordance

The LIBRARY and FILES columns render `homeDim("(empty)")` (home.go:758, 787). Replace with actionable
guidance so a user who has deleted everything still knows what to do:
- LIBRARY empty → `homeDim("no projects — + to create")`
- FILES empty → `homeDim("no files — ctrl+n for a doc")`

(The existing `F1 · ?  keybindings` hint at the bottom of the hub already covers key discovery.)

## Tests
- `maybeSeedSample`: empty temp dir + absent marker → seeds `the-lighthouse/manifest.json` + the 3
  chapter files; the marker now exists. A second call is a no-op. A non-empty dir (has a `.md`) +
  absent marker → does NOT seed but writes the marker.
- `dirIsEmptyish`: empty → true; a `.md` present → false; a subdir present → false; only dotfiles → true.

## Out of scope
- A guided tour / interactive tutorial (the sample manuscript IS the tour).
- Re-seeding or a "reset to sample" command.

## Sequencing (for the plan)
1. **Embed + seed** — `sampleFS`, `seedMarkerPath`, `maybeSeedSample`, `dirIsEmptyish`, `writeMarker`,
   wired into `initialModel` (onboarding.go + main.go) + tests.
2. **Empty-home affordance** — the two `(empty)` strings (home.go).
