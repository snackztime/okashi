# Timer upgrades: active-time + daily-accumulated goal + pomodoro bell — design

**Date:** 2026-07-02
**Status:** Approved
**Context:** The session timer shipped v1 as a wall-clock stopwatch + a per-session time goal +
a pomodoro (visual cue only). These are the deferred upgrades: make the stopwatch count **active
writing time** (pause on idle), make the time goal **daily-accumulated** (honest now that idle
doesn't count), and add a **terminal bell** at pomodoro transitions.

Facts (verified): `m.lastEditAt time.Time` is set on every edit; the 1-second `autosaveTick`
reschedules every tick; `projectGoals` persists per-dir via `saveGoals`; the daily word goal rolls
over via `rolloverIfNeeded(pg, total, today)` keyed on `pg.Day`. `View()` is pure (reads `m.now`).

---

## Decisions (locked)
- **Idle threshold = 2 min** (`activeIdle = 2 * time.Minute`).
- **Stopwatch shows active time** (session), with a `⏸` when currently idle. (Wall-clock session
  length is dropped.)
- **Time goal is daily-accumulated active minutes.** A dedicated `TimeDay` field decouples the
  time rollover from the word rollover (no shared-`Day` coordination bug).
- **Bell rings at every pomodoro transition** (work→break, break→done), once, via a one-shot cmd.

## 1. Active-time engine + stopwatch (status bar)

**Model:** add `activeSecs int` (active seconds this session; runtime only).

**Pure helper (`main.go`):**
```go
const activeIdle = 2 * time.Minute

// isWritingActive reports whether the writer is currently active (edited within the idle window).
func isWritingActive(now, lastEdit time.Time, idle time.Duration) bool {
	return now.Sub(lastEdit) < idle
}
```

**Tick (`autosaveTickMsg` handler, after `m.now = now`):**
```go
	if isWritingActive(m.now, m.lastEditAt, activeIdle) {
		m.activeSecs++
		// (Task 2 adds the daily accumulator here.)
	}
```
Note: `lastEditAt` is the zero value until the first edit, so `now.Sub(zero)` is huge → not active
until the writer types — correct (the clock starts when writing starts).

**Status-bar display (the stats builder, ~`main.go:2260`):** replace the wall-clock stopwatch
segment with active time + an idle marker (the sprint-countdown swap from the shipped Task 3 stays):
```go
	timeSeg := "⏱ " + fmtDuration(time.Duration(m.activeSecs)*time.Second)
	if !isWritingActive(m.now, m.lastEditAt, activeIdle) {
		timeSeg += " ⏸"
	}
	// … unchanged: if m.sprintActive { timeSeg = sprintDisplay(...) with last-60s accent } …
```

**Tests (`timer_test.go`):** `isWritingActive` — active just after an edit, inactive past the idle
window, inactive at the zero-value `lastEdit`.

## 2. Daily-accumulated active-time goal (persistence)

**Model / persistence (`goals.go`):** add to `projectGoals`:
```go
	ActiveSecsToday int    `json:"activeSecsToday"`
	TimeDay         string `json:"timeDay"`
```
**Rollover helper (`goals.go`):**
```go
// rolloverActive resets the daily active-time accumulator when the date changed.
func rolloverActive(pg projectGoals, today string) (projectGoals, bool) {
	if pg.TimeDay != today {
		pg.TimeDay = today
		pg.ActiveSecsToday = 0
		return pg, true
	}
	return pg, false
}
```

**Tick (extend the active branch from Task 1):** when active, also accumulate into the current
dir's daily total, and persist ~once a minute:
```go
	if isWritingActive(m.now, m.lastEditAt, activeIdle) {
		m.activeSecs++
		if m.goalsAll == nil {
			m.goalsAll = map[string]projectGoals{}
		}
		pg := m.goalsAll[m.files.dir]
		pg, _ = rolloverActive(pg, today())
		pg.ActiveSecsToday++
		m.goalsAll[m.files.dir] = pg
		m.activeSaveCtr++
		if m.activeSaveCtr >= 60 { // persist ~once per active-minute
			saveGoals(goalsPath(), m.goalsAll)
			m.activeSaveCtr = 0
		}
	}
```
Add model field `activeSaveCtr int`. **No persist-on-quit** — okashi has many `ctrl+c`/`tea.Quit`
sites (each prompt handles its own), so the ~1/min cadence is the single, uniform persistence path;
a quit loses at most the last <60 s of accumulated active time. (Persist-on-quit is a possible
later nicety.)

**Tests (`goals_test.go`):** `rolloverActive` resets `ActiveSecsToday`+sets `TimeDay` on a date
change and is a no-op same-day; `ActiveSecsToday`/`TimeDay` round-trip through `saveGoals`/`loadGoals`.

## 3. Goals-tab TIME section (Session + Today)

**`goalStats` (`inspector.go:146`):** keep `sessionSecs` (now = **active** session secs) and
`sessionGoalMin` (the goal minutes), and add:
```go
	todayActiveSecs int
	idle            bool
```
(So: `type goalStats struct{ today, dailyGoal, project, projectGoal, sessionSecs, sessionGoalMin, todayActiveSecs int; idle bool }`.)

**Populate at `main.go:1451`:** `sessionSecs: m.activeSecs`, `sessionGoalMin: pg.SessionGoalMin`,
`todayActiveSecs: pg.ActiveSecsToday`, `idle: !isWritingActive(m.now, m.lastEditAt, activeIdle)`.

**Render (`inspector.go` `case tabGoals`):** rename the section to **TIME** and show both clocks —
Session (active, with `⏸` when idle) and Today (accumulated, with the goal bar):
```go
		b.WriteString("\n\n" + sectionHeader("Time", width) + "\n")
		sess := "  Session   " + fmtDuration(time.Duration(goals.sessionSecs)*time.Second)
		if goals.idle {
			sess += " ⏸"
		}
		b.WriteString(sess + "\n")
		if goals.sessionGoalMin > 0 {
			mins := goals.todayActiveSecs / 60
			b.WriteString("  Today\n")
			b.WriteString("  " + progressBar(mins, goals.sessionGoalMin, max(4, width-10)) + "\n")
			b.WriteString("  " + fmt.Sprintf("%d / %d min\n", mins, goals.sessionGoalMin))
			if mins >= goals.sessionGoalMin {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(accent).Render("✓ time goal met"))
			} else {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(subtle).Render(fmt.Sprintf("%d min to go", goals.sessionGoalMin-mins)))
			}
		} else {
			b.WriteString("  Today     " + fmtDuration(time.Duration(goals.todayActiveSecs)*time.Second) + "\n")
		}
```
The `ctrl+g` prompt's field-3 label may read "daily minutes" now (the value still lands in
`SessionGoalMin`); update the label text if it currently says "session minutes".

**Tests (`inspector_test.go`):** update `TestInspectorGoalsSessionSection` — the bar now reflects
`todayActiveSecs` (set `todayActiveSecs: 600, sessionGoalMin: 30` → "10 / 30 min"); assert the
`"TIME"` header; add a case with `idle: true` showing `⏸`.

## 4. Pomodoro bell

**Cmd (`main.go`):**
```go
// bellCmd rings the terminal bell once (BEL). Fired at a pomodoro transition.
func bellCmd() tea.Cmd {
	return func() tea.Msg {
		os.Stdout.Write([]byte{0x07})
		return nil
	}
}
```
**Wire into the tick transition:** where the sprint transition fires (`advanceSprint`), collect a
bell into the returned command batch. Refactor the tick handler's return to gather cmds:
```go
	cmds := []tea.Cmd{autosaveTick()}
	// … existing grammar-recheck branch appends checkGrammarCmd(...) …
	if m.sprintActive && !m.now.Before(m.sprintEnd) {
		end, onBreak, active, msg := advanceSprint(m.now, m.sprintEnd, m.sprintOnBreak, 5*time.Minute)
		m.sprintEnd, m.sprintOnBreak, m.sprintActive = end, onBreak, active
		m.status = msg
		cmds = append(cmds, bellCmd())
	}
	return m, tea.Batch(cmds...)
```
(Keep the existing autosave/grammar behavior inside this consolidated return.) The BEL is a C0
control the terminal acts on immediately; writing one byte from a cmd goroutine does not disturb the
alt-screen buffer. No test (side-effecting I/O); verified by inspection + manual.

## 5. Out of scope
- A configurable idle threshold or a "paused" toast; per-project vs global active time (stays
  per-dir, like the word goal).
- Persisting `activeSecs` (session clock) across restarts — only the daily accumulator persists.

## 6. Sequencing (for the plan)
1. **Active-time engine + stopwatch** — `activeSecs`, `isWritingActive`, tick increment, status-bar
   active-time + `⏸` (`main.go`) + helper test.
2. **Daily accumulation** — `ActiveSecsToday`/`TimeDay`, `rolloverActive`, tick accumulate + persist
   (~1/min + on quit) (`goals.go`/`main.go`) + rollover/round-trip tests.
3. **Goals-tab TIME section** — `goalStats` fields, TIME render, prompt label (`inspector.go`/`main.go`)
   + render test.
4. **Pomodoro bell** — `bellCmd`, wire into the tick transition (`main.go`).
