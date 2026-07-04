# Revision diff + Pace intelligence — design spec

**Date:** 2026-07-04
**Status:** Approved design (decisions confirmed 2026-07-04). Two independent standout features built
together this cycle: **(A) snapshot diff** ("see your revisions") and **(B) pace intelligence** ("stay
on pace"). Both are pure-Go, no new dependency, no shared-contract impact.

---

## Feature A — Snapshot diff

**Why:** okashi just shipped snapshots (`.okashi-bak/` ring, `b` screen). Almost no TUI writer can show
*what changed* between drafts; a diff is the terminal's home turf. High leverage, builds on shipped code.

**Storage:** none new — diffs the `.okashi-bak/` snapshots and the current file.

**Diff engine (`diff.go`):** a pure-Go **Myers O(ND) line diff** — no dependency.
- `type diffOp int` ∈ {`opEqual`, `opDel`, `opAdd`}.
- `type diffLine struct { op diffOp; text string }`.
- `func diffLines(a, b []string) []diffLine` — Myers shortest-edit-script over lines; typical prose
  edits are small D so it's fast on chapter-sized inputs.
- **Intra-line word highlight:** when a run of `opDel` lines is immediately followed by a run of
  `opAdd` lines of equal length, pair them up and, for each pair that is "similar" (shares a token
  prefix/suffix or >50% word overlap), compute a word-level diff (`diffWords(a, b string) (aRuns,
  bRuns []wordRun)` where `wordRun{text string; changed bool}`) so only the changed words are
  emphasized. Prose that changed a few words reads cleanly instead of a whole red/green block.

**UI (`diff.go`, `screenDiff` + `diffModel`):** a scrollable, windowed diff view.
- `type diffModel struct { aLabel, bLabel string; lines []diffLine; wordRuns map[int][]wordRun; offset, height int }`
  (`wordRuns` keyed by line index for the highlighted del/add pairs).
- Rendering: full-file diff, **windowed** to `lines[offset:offset+height]` (View() O(visible), per
  CLAUDE.md). `opDel` red, `opAdd` green, `opEqual` dim/normal; changed words within a paired line get
  a brighter fg. A left gutter marks `-`/`+`/` `.
- Keys: `↑/↓`/`j/k`, `PgUp/PgDn` scroll; `n`/`N` jump to next/prev change (skip equal runs); `esc`
  back to the Snapshots screen. Header: `<aLabel>  →  <bLabel>` (e.g. `2026-07-04 14:22 → current`).

**Entry (from the Snapshots screen, `snapshots.go`):**
- `d` — diff the **selected snapshot** against the **current file** (aLabel = snapshot stamp, bLabel =
  "current"). The quick path.
- `D` (`shift+d`) — **mark two endpoints**: first `D` marks the selected snapshot as A (status
  "marked A · pick another, D to diff"); second `D` on a different snapshot sets B and opens the
  A↔B diff. A third `D`/`esc` clears the mark. `markA int` on `snapshotsModel` (-1 = none).
- Builds `m.diff = newDiffModel(aLabel, aContent, bLabel, bContent)`; `m.screen = screenDiff`.
  "current" content = the live file on disk (or `m.editor.Value()` when it's the open file).

**Tests (`diff_test.go`):** `diffLines` on {identical → all equal; pure insert; pure delete;
replace; interleaved} yields the right op sequence and reconstructs b from the (equal+add) lines;
`diffWords` highlights only changed tokens; the diff view windows and never renders > height rows;
`n`/`N` land on change boundaries; entry from snapshots builds the right endpoints (selected↔current
and A↔B).

**Tasks:** A1 `diffLines` + `diffWords` engine + tests. A2 `diffModel` + `screenDiff` view/scroll/jump
+ dispatch wiring in main.go. A3 snapshots-screen `d`/`D` entry + `markA` + help/README.

---

## Feature B — Pace intelligence

**Why:** the *motivation-over-months* hook long-form writers stay for. Extends the existing
goals/timer. Two payoffs on one foundation.

### B0 — Daily history foundation (`goals.go`)

Add `History map[string]int` to `projectGoals` (`json:"history,omitempty"`) — calendar-day
(`"2006-01-02"`) → net words written that day.
- **Record:** wherever `todayWords(pg, total)` is known each tick/save, set
  `pg.History[today()] = todayWords(pg, total)` (a helper `recordToday(pg, total, today) projectGoals`
  that lazily makes the map). Past days keep their final value (only `today()`'s entry updates). The
  existing `rolloverIfNeeded` already resets the baseline at midnight, so yesterday's entry freezes
  correctly.
- **Storage:** nests under each project in the existing `goals.json` (already per-project-keyed, atomic
  writes). ~365 small ints/project/year — negligible.
- Backward-compatible: `omitempty` + lazy map init; old goals.json files load fine.

### B1 — Deadline burndown (`goals.go` + inspector Goals tab)

Add `Deadline string` to `projectGoals` (`json:"deadline,omitempty"`, `"2006-01-02"`), paired with the
existing `ProjectGoal` (target total words).
- **Compute** (`func paceLine(pg, projectWords int, today string) (string, bool)`): `remaining =
  ProjectGoal - projectWords`; `daysLeft = days(deadline) - days(today)`. Show
  `≈<perDay>/day to hit <goal> by <deadline> (<daysLeft>d)`. Edge cases: deadline passed → "deadline
  passed"; goal met → "✓ target met"; no deadline or no ProjectGoal → hidden.
- **Entry:** extend the `ctrl+g` prompt chain with a **4th step** (`goalPromptField == 4`, "deadline
  YYYY-MM-DD · blank to clear"); parse `time.Parse("2006-01-02", …)`, blank clears. Update the
  `goalPromptField` state machine (main.go ~996–1034), the statusBar prompt label (main.go ~2559–2565),
  and the `ctrl+g` help text.
- **Display:** a line in the **Goals** inspector tab under Project (uses `computeProjStats` words).

### B2 — History heatmap (`heatmap.go`, dedicated screen) + Goals-tab sparkline

- **Dedicated screen** (`screenHeatmap` + `heatmapModel`): a git-contributions-style grid — columns =
  ISO weeks, 7 weekday rows, each cell a block glyph colored by that day's word count bucketed into 5
  levels (0 / low / mid / high / max, thresholds relative to the project's max day). **Windowed** to
  the most-recent weeks that fit `m.width`. Header `history · <project>`, a legend row, and totals
  (current streak, best day, N-day total). Built from `pg.History`.
- **Streak:** `func streak(history map[string]int, today string) int` — consecutive days up to today
  with `> 0` words.
- **Sparkline (Goals tab):** last ~14 days as unicode bars (`▁▂▃▄▅▆▇█` scaled to the window max) plus
  `<n>-day streak`. Clicking the sparkline row opens `screenHeatmap`.
- **Entry:** click the Goals-tab history row (mouse) opens the heatmap; `esc` closes back to writing.
  **Open (minor):** a keyboard-only opener — deferred; the click path ships v1 (noted in README).

**Tests (`goals_test.go`, `heatmap_test.go`):** `recordToday` writes today's delta and leaves past
days untouched; `History` round-trips through goals.json and old files load (no `history` key) without
error; `paceLine` for {normal, met, past-deadline, no-deadline}; `streak` counts consecutive days and
resets on a gap; the heatmap buckets values into 5 levels and windows to width; sparkline scales.

**Tasks:** B0 history foundation + record wiring + tests. B1 deadline field + `ctrl+g` 4th step +
paceLine + Goals-tab line + tests. B2 heatmap screen + streak + sparkline + click entry + tests.

---

## Global constraints
- Pure-Go, **no new dependency** (implement Myers + the heatmap ourselves).
- No shared-contract impact: diffs read `.okashi-bak/`; history/deadline live in `goals.json`
  (okashi-private, already app-global). No `manifest.json` change → HARD GATE untriggered.
- `View()` stays O(visible) for the diff and heatmap screens (window to the visible rows).
- Atomic writes (already true for `goals.json`).
- New screens dispatch modally in `Update`/`View` and handle `WindowSizeMsg`, mirroring
  `screenSnapshots`/`screenProperties`.

## Build order
A1 → A2 → A3 (diff), then B0 → B1 → B2 (pace). Each task: TDD, build/vet/test green, commit; merge the
diff feature and the pace feature as they complete. Adversarial review pass on the diff engine (edit-
script correctness) and the history-recording/rollover interaction before merge.

## Non-goals
Word-processor track-changes/accept-reject; three-way merge; per-word blame; cross-project analytics;
a full calendar UI. Keep both features read-only-analytical and lean.
