# Session timer: stopwatch + time goal + pomodoro — design

**Date:** 2026-07-01
**Status:** Approved (direction: Goals-tab config + status-bar readout + a sprint key)
**Context:** okashi already frames a "session" (the status bar shows `+N session` word delta) and has
a goals system (`ctrl+g`, daily/project bars in the Goals inspector tab, per-dir `projectGoals`
persisted via `saveGoals`). A 1-second `autosaveTick` already runs and **reschedules every tick**
(`return m, autosaveTick()`), so anything time-based in `View()` updates live with no new
infrastructure. This adds the *time* half of the session: a stopwatch, a time goal, and a pomodoro
sprint.

Three sub-features, built in order (increasing surface): **stopwatch → time goal → pomodoro**.

---

## Decisions (locked, with one open item for review)

- **Readout:** the status bar, in the stats cluster next to `+N session`.
- **Config:** the Goals inspector tab (a new SESSION section), beside the word goals.
- **Sprint toggle:** a global key **`ctrl+u`** (free slot; rename if preferred) starts/stops a
  sprint from the editor.
- **Clock basis:** **wall-clock since launch** for v1 (matches how `+N session` works). Active-time
  (pauses on idle, using the existing `lastEditAt`) is the paired later upgrade.
- **Pomodoro:** default **25 min work / 5 min break**; a **visual cue** at each transition (accent
  status message + a single terminal `\a` bell — no audio dependency).
- **Time-goal scope — RESOLVED: per-session (v1).** The time goal is "write N minutes this session,"
  sharing the stopwatch's since-launch clock. A *daily-accumulated* time goal (like the daily word
  goal) is deferred: under wall-clock, an app left open idle all day would inflate it — daily
  accumulation only becomes honest once active-time lands. That pairing (active-time + daily
  accumulation) is the future upgrade.

---

## 1. Stopwatch (status bar)

**Model:** add `sessionStart time.Time`, stamped in `initialModel` (the launch moment). No tick
changes needed — the status bar already re-renders each second.

**Display (`main.go`, the stats string ~line 2242):** append the elapsed session time to
`"%s words · %s session"` → `"%s words · %s session · ⏱ %s"`, where the time is formatted by a new
pure helper:
```go
// fmtDuration renders a duration as M:SS under an hour, H:MM:SS at or above.
func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}
```
The elapsed value is `now.Sub(m.sessionStart)` — but `View()` has no `now`. Stamp `m.now time.Time`
from the `autosaveTickMsg` handler each tick (and once in `initialModel`), and render
`fmtDuration(m.now.Sub(m.sessionStart))`. (Using a model `now` keeps `View()` pure and avoids
calling `time.Now()` in the render path.)

**Tests (`main_test.go` or a new `timer_test.go`):**
- `fmtDuration`: `0 → "0:00"`, `65s → "1:05"`, `3725s → "1:02:05"`.

## 2. Session time goal (Goals inspector tab)

**Model / persistence (`goals.go`):** add `SessionGoalMin int` to `projectGoals` (JSON
`sessionGoalMin`); `applyEnvDefaults` may default it to 0 (off) or an `OKASHI_SESSION_GOAL` env
minutes. Persisted per-dir via the existing `saveGoals`.

**Goals-tab view (`inspector.go`):** extend `goalStats` (line 145) with
`sessionSecs, sessionGoalMin int`. In `case tabGoals`, after the Daily/Project sections add a
SESSION section (only when relevant, i.e. always show elapsed; show the bar when
`sessionGoalMin > 0`):
```go
b.WriteString("\n\n" + sectionHeader("Session", width) + "\n")
b.WriteString("  " + kvRow("Elapsed", /* fmtDuration of sessionSecs */, width-2) + "\n")
if goals.sessionGoalMin > 0 {
	mins := goals.sessionSecs / 60
	b.WriteString("  " + progressBar(mins, goals.sessionGoalMin, max(4, width-10)) + "\n")
	b.WriteString("  " + fmt.Sprintf("%d / %d min\n", mins, goals.sessionGoalMin))
	if mins >= goals.sessionGoalMin {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ time goal met"))
	} else {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(fmt.Sprintf("%d min to go", goals.sessionGoalMin-mins)))
	}
}
```
(`kvRow` shows a label/value row; pass the pre-formatted elapsed string. Find where `goalStats` is
constructed — grep `goalStats{` — and populate `sessionSecs = int(m.now.Sub(m.sessionStart).Seconds())`
and `sessionGoalMin` from `m.goalsAll[m.files.dir].applyEnvDefaults()`.)

**Setting the goal:** reuse the `ctrl+g` goal prompt flow (main.go ~536) — add a session-minutes
field to the prompt sequence, or a second `ctrl+g` step. (Detail for the plan: the prompt currently
sets DailyGoal/ProjectGoal via `nameInput` steps; add a `session minutes` step and persist.)

**Tests:** `projectGoals` round-trips `SessionGoalMin` through `saveGoals`/`loadGoals`;
`applyEnvDefaults` handles the env default; a Goals-tab render snapshot shows the SESSION section
with the bar when `sessionGoalMin > 0` and no bar when 0.

## 3. Pomodoro sprint (status bar countdown + `ctrl+u`)

**Model:** runtime state (not persisted):
- `sprintActive bool`, `sprintEnd time.Time`, `sprintOnBreak bool`.
- `SprintMin int` in `projectGoals` (persisted; default 25); break is fixed 5 min in v1 (or a
  `BreakMin` field, default 5).

**Start/stop (`main.go`, editor key handler):** `case "ctrl+u"`:
- If not active: start a work sprint — `sprintActive = true`, `sprintOnBreak = false`,
  `sprintEnd = m.now.Add(SprintMin minutes)`.
- If active: cancel — `sprintActive = false`.

**Tick handling (`autosaveTickMsg` handler):** after setting `m.now`, if `sprintActive` and
`m.now >= sprintEnd`: ring the bell + set an accent status message, then transition:
- work → break: `sprintOnBreak = true`, `sprintEnd = m.now.Add(5 min)`, status `"break — 5 min"`.
- break → done: `sprintActive = false`, status `"sprint complete"`.
Emit the terminal bell by writing `\a` (via a `tea.Printf`/`tea.Println` cmd, or append `\a` to the
status render once — the plan picks the least-intrusive way that doesn't corrupt the alt-screen).

**Display (status bar):** when `sprintActive`, the `⏱` stopwatch is replaced by the countdown:
`🍅 %s` (work) or `☕ %s` (break), where the value is `fmtDuration(m.sprintEnd.Sub(m.now))`. In the
final 60 s, render it in `errColor`/accent as the cue. When not active, show the plain stopwatch.

**Sprint length config:** set via the Goals-tab SESSION section (a `SprintMin` line) or the `ctrl+g`
prompt; default 25.

**Tests:**
- A pure `sprintDisplay(now, sprintEnd, onBreak)` helper (extract the format decision) returns
  `🍅 M:SS` for work, `☕ M:SS` for break, and the remaining time; test the boundary (at/after end).
- The transition logic (work→break→done) as a small pure function `advanceSprint(now, end, onBreak)
  (newEnd time.Time, nowOnBreak, stillActive bool, cue string)` so it's testable without the model.

## 4. Out of scope (v1)
- Active-time tracking (pause on idle) — the paired upgrade to the stopwatch + a daily-accumulated
  time goal.
- Auto-repeating pomodoro cycles; long-break-every-4; sound files.
- A status-bar popup menu (config lives in the Goals tab, per the approved architecture).

## 5. Sequencing (for the plan)
1. **Stopwatch** — `sessionStart`/`m.now`, `fmtDuration`, status-bar display (`main.go`) + helper test.
2. **Time goal** — `SessionGoalMin` in `projectGoals`, `goalStats` extension + SESSION section, goal
   prompt (`goals.go`/`inspector.go`/`main.go`) + persistence test.
3. **Pomodoro** — sprint state, `ctrl+u`, tick transitions + bell/cue, countdown display, `SprintMin`
   (`main.go`/`goals.go`) + `sprintDisplay`/`advanceSprint` tests.
