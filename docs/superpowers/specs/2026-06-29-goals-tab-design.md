# Goals tab (daily + project word goals) — design

**Date:** 2026-06-29
**Status:** Approved (pending spec review)
**Context:** Cycle 2 of the inspector roadmap. Adds the inspector's **Goals** tab: a daily
word-goal progress bar + a project-total progress bar, with goals set in-app (`ctrl+g`) and
persisted. Builds on the inspector tab framework (Words, Outline shipped).

## Persistence (`goals.go`)

- Path: `goalsPath()` → `UserConfigDir/okashi/goals.json` (mirrors `recentPath()`; "" if no
  config dir).
- Schema: a map keyed by **project dir path** →
  `projectGoals{ DailyGoal, ProjectGoal, DayBaseline int; Day string }` (`Day` = `YYYY-MM-DD`).
  `loadGoals(path) map[string]projectGoals` and `saveGoals(path, m)` via `encoding/json` +
  `atomicWrite` (exactly like `recent.go`). Unreadable/missing → empty map.
- Env defaults (applied when an entry's value is 0): `OKASHI_DAILY_GOAL` (default **500**),
  `OKASHI_PROJECT_GOAL` (default **0** = no project bar). Parsed as ints; invalid → default.

## Daily tracking (pure helpers in `goals.go`)

- `today() string` returns the local date `YYYY-MM-DD`.
- `rolloverIfNeeded(pg projectGoals, total int, today string) (projectGoals, bool)`: if
  `pg.Day != today`, set `pg.Day = today`, `pg.DayBaseline = total`, return `(pg, true)` (changed
  → caller persists); else `(pg, false)`.
- `todayWords(pg projectGoals, total int) int` = `max(0, total - pg.DayBaseline)` (net since the
  day's first activity; clamped ≥ 0 so a heavy-deletion day shows 0, not negative).

## Model wiring (`main.go`)

- Field: `goal projectGoals` (the current project's entry, loaded into the model).
- **Load on project change:** when `m.files.dir` is (re)set, `loadGoals` → take the entry for
  `dir` (or a zero entry); fill 0 `DailyGoal`/`ProjectGoal` from env defaults; compute the
  current project total; `rolloverIfNeeded`; if changed, write back via `saveGoals`. Store the
  result in `m.goal`.
- **Goal stats for the inspector** (computed in `View` when the Goals tab is up): the project
  total comes from `computeProjStats(...).words`; `goalStats{ today, dailyGoal, project,
  projectGoal int }` where `today = todayWords(m.goal, total)`, `project = total`.
- **`ctrl+g` two-step prompt** (writing screen): enter goal-prompt mode (field = daily),
  prefill the input with the current daily goal, focus the `textinput`. On **Enter**: parse the
  int (ignore/keep-old on parse error); if field == daily → set `m.goal.DailyGoal`, advance to
  field = project (prefill current project goal); if field == project → set
  `m.goal.ProjectGoal`, `saveGoals`, exit prompt. **Esc** cancels (no change). State:
  `goalPromptField int` (0 = off, 1 = daily, 2 = project); reuse `nameInput` (not used
  concurrently with rename/new). While prompting, the bottom bar shows `daily goal: …` /
  `project goal: …` (like the rename prompt).
- Persist on goal-set and day-rollover only — never per frame.

## Goals tab (`inspector.go`)

- Add `tabGoals` to the tab consts; `inspectorTabLabels()` → `{"Words", "Outline", "Goals"}`
  (so `ctrl+y` cycles Words → Outline → Goals → closed automatically).
- `View` gains a `goalStats` param (or a small struct): `View(width, doc, proj, outline,
  goals)`. The `tabGoals` body renders:
  ```
  Daily
  [██████░░░░]  312 / 500
  188 to go            (or "✓ goal met")

  Project
  [███░░░░░░░]  47,032 / 80,000
  ```
  - `progressBar(cur, goal, width int) string`: a `width`-cell bar, filled =
    `round(width * clamp(cur/goal, 0, 1))` cells in `accent`, the rest `░` in `subtle`; `goal
    <= 0` → an empty/▢ bar. Below it: `commafy(cur) / commafy(goal)` and the remaining
    (`commafy(goal-cur) to go`) or `✓ goal met` when `cur >= goal`.
  - The **Project** section is omitted entirely when `goalStats.projectGoal == 0`.
- The `View` signature grows by one param — update all call sites (main.go + inspector_test.go).

## Tab click-to-switch (mouse)

Make the inspector tab bar clickable (in addition to `ctrl+y` cycling).

- `inspector.go` helper `inspectorTabAtX(localX int) (inspectorTab, bool)`: walk
  `inspectorTabLabels()`, accumulating each chip's width (`len(label)+2`, the `" label "`
  form); return the tab whose span contains `localX` (`false` if past the last chip). This is
  the render's mirror, so click targets always match the drawn chips.
- `main.go` `MouseMsg` handler (left-button press): when `showInspector` (from
  `effectivePanels()`) and the click is on the inspector's **tab row** (`msg.Y == 0`, the top
  body row — the panel has a left border + horizontal padding, no top border), compute
  `localX = msg.X - (m.width - inspectorWidth) - 2` (panel-left + border(1) + padding(1)); if
  `localX >= 0` and `inspectorTabAtX(localX)` hits, set `m.inspector.tab` to it. A click
  outside the tab row / inspector region is unaffected (existing sidebar/editor handling
  stands).
- Clicking only switches among the visible tabs; it does not open a hidden inspector (no tab
  bar is drawn when hidden).

## Testing

- Store: `saveGoals`/`loadGoals` round-trip a map; missing file → empty map; env defaults
  applied for 0 values.
- `rolloverIfNeeded`: same day → no change; different day → baseline reset to total + changed=true.
- `todayWords`: `total - baseline` clamped ≥ 0.
- `progressBar`: 0% → all empty; 50% → ~half filled; ≥100% → full (no overflow); goal 0 → empty bar.
- Goals tab `View`: renders both bars + numbers; `projectGoal == 0` omits the Project section;
  `✓ goal met` when today ≥ daily goal.
- `ctrl+g` flow: first Enter sets daily + advances to project; second Enter sets project +
  persists + exits; Esc cancels. `ctrl+y` now reaches Goals as the 3rd tab.
- `inspectorTabAtX`: x in the first chip → Words; in the Outline chip → Outline; past the last
  → not ok. A left-click on a tab in the writing `View` switches `m.inspector.tab` (with the
  inspector visible at a known width).

## Out of scope (later)

- Streaks / history / charts. Per-chapter goals. Reading-time. A separate Analysis cycle
  (spellcheck/syntax) is next.
- Writing-session timers. The daily reset is date-based (local midnight), not a rolling 24h.
