# Session timer: stopwatch + time goal + pomodoro — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a session stopwatch (status bar), a per-session time goal (Goals inspector tab), and a pomodoro sprint (`ctrl+u`, status-bar countdown) to okashi.

**Architecture:** Reuse the existing 1-second `autosaveTick` (it reschedules every tick, so `View()` refreshes live) and the per-dir `projectGoals` persistence. Render time from a model `m.now` stamped each tick, keeping `View()` free of `time.Now()`. Config lives in the Goals tab; the stopwatch/countdown live in the status bar.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea, lipgloss.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`.
- `View()` stays pure — no `time.Now()` in render; render from `m.now` (stamped in `initialModel` and each `autosaveTickMsg`).
- Time goal is **per-session** (elapsed since launch), not daily-accumulated (that + active-time is a deferred upgrade).
- Pomodoro cue is **visual only** in v1 (accent status message + last-60s color). No terminal bell (it races the alt-screen renderer).
- Defaults: sprint **25 min** work / **5 min** break; time goal default **0** (off).
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.
- Facts: the stats string is built in `main.go` (~line 2241: `delta := words - m.sessionBaseline` then `"%s words · %s session"`). `goalStats{}` is constructed at `main.go:1451`. The inspector Goals tab renders in `inspector.go` `case tabGoals:`. The goal prompt uses `m.goalPromptField` (0 off, 1 daily, 2 project). `projectGoals` is in `goals.go`, persisted via `saveGoals`. `errColor`/`accent`/`subtle`/`progressBar`/`sectionHeader`/`kvRow`/`commafy` already exist.

---

## Task 1: Session stopwatch (status bar)

**Files:**
- Modify: `main.go` (model fields `now`, `sessionStart`; `initialModel`; the `autosaveTickMsg` handler; the stats string builder; add `fmtDuration`)
- Test: `timer_test.go` (new)

**Interfaces:**
- Produces: `func fmtDuration(d time.Duration) string`; model fields `now, sessionStart time.Time`.

- [ ] **Step 1: Write the failing test**

Create `timer_test.go`:
```go
package main

import (
	"testing"
	"time"
)

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{65 * time.Second, "1:05"},
		{9 * time.Second, "0:09"},
		{3725 * time.Second, "1:02:05"},
		{-5 * time.Second, "0:00"},
	}
	for _, c := range cases {
		if got := fmtDuration(c.d); got != c.want {
			t.Fatalf("fmtDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestFmtDuration -v`
Expected: FAIL (`undefined: fmtDuration`).

- [ ] **Step 3: Add `fmtDuration` + the model fields + stamping**

Add the helper (near the other small helpers in `main.go`):
```go
// fmtDuration renders a duration as M:SS under an hour, H:MM:SS at or above. Negative clamps to 0.
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
Add model fields (near `sessionBaseline`):
```go
	now         time.Time
	sessionStart time.Time
```
In `initialModel`, stamp both (add to the struct literal or set before returning):
```go
	// … in the returned model literal or just before return:
	now:          time.Now(),
	sessionStart: time.Now(),
```
In the `autosaveTickMsg` handler (`main.go` ~706), set `m.now = now` at the top (after `now := time.Time(t)`).

- [ ] **Step 4: Show the stopwatch in the stats string**

Find the stats builder (`main.go` ~2241, the `"%s words · %s session"` line). Append the elapsed session time:
```go
	return fmt.Sprintf("%s words · %s session · ⏱ %s", commafy(words), signedComma(delta), fmtDuration(m.now.Sub(m.sessionStart)))
```
(If `m.now` is the zero value on the very first render before any tick, `initialModel` already stamped it, so `Sub` is ~0 → "0:00".)

- [ ] **Step 5: Build + test + vet + eyeball**

```
/opt/homebrew/bin/gofmt -w main.go timer_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Optionally `go run .` and confirm the status bar shows `⏱ 0:00` ticking up.

- [ ] **Step 6: Commit**

```
git add main.go timer_test.go
git commit -m "feat(timer): session stopwatch in the status bar"
```

---

## Task 2: Per-session time goal (Goals tab)

**Files:**
- Modify: `goals.go` (`projectGoals.SessionGoalMin`), `inspector.go` (`goalStats` + SESSION section), `main.go` (`goalStats{}` population at 1451; the `ctrl+g` prompt gains a session step)
- Test: `goals_test.go`, `inspector_test.go`

**Interfaces:**
- Consumes: `fmtDuration`, `m.now`, `m.sessionStart`.
- Produces: `projectGoals.SessionGoalMin int`; `goalStats.sessionSecs, sessionGoalMin int`.

- [ ] **Step 1: Write the failing tests**

Add to `goals_test.go`:
```go
func TestSessionGoalRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	saveGoals(path, map[string]projectGoals{"/p": {DailyGoal: 500, SessionGoalMin: 30}})
	got := loadGoals(path)
	if got["/p"].SessionGoalMin != 30 {
		t.Fatalf("SessionGoalMin round-trip = %d, want 30", got["/p"].SessionGoalMin)
	}
}
```
Add to `inspector_test.go` (the Goals tab shows the SESSION bar when a goal is set):
```go
func TestInspectorGoalsSessionSection(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabGoals}
	out := ansi.Strip(in.View(28, docStats{}, projStats{}, "", goalStats{sessionSecs: 600, sessionGoalMin: 30}, analysisState{}))
	if !strings.Contains(out, "Session") || !strings.Contains(out, "10 / 30 min") {
		t.Fatalf("expected a Session section with 10/30 min, got:\n%s", out)
	}
}
```
(Confirm `inspectorModel`/`View` field names from `inspector.go`; match the existing `inspector_test.go` construction style.)

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run 'TestSessionGoalRoundTrips|TestInspectorGoalsSessionSection' -v`
Expected: FAIL (`SessionGoalMin` / `sessionSecs` undefined).

- [ ] **Step 3: Add the persisted field**

In `goals.go`, add to `projectGoals`:
```go
	SessionGoalMin int    `json:"sessionGoalMin"`
```
(Leave `applyEnvDefaults` as-is — default 0 means the time goal is off.)

- [ ] **Step 4: Extend `goalStats` + render the SESSION section**

In `inspector.go`, extend the struct (line ~145):
```go
type goalStats struct{ today, dailyGoal, project, projectGoal, sessionSecs, sessionGoalMin int }
```
In `case tabGoals:`, after the Daily (and Project) sections, add:
```go
		b.WriteString("\n\n" + sectionHeader("Session", width) + "\n")
		b.WriteString("  " + kvRow("Elapsed", 0, width-2) + "\n") // replaced below with a formatted string
```
Because `kvRow` takes an int value, render the elapsed as its own line instead:
```go
		b.WriteString("\n\n" + sectionHeader("Session", width) + "\n")
		b.WriteString("  Elapsed  " + fmtDuration(time.Duration(goals.sessionSecs)*time.Second) + "\n")
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
(Add `"time"` to `inspector.go` imports if not present.)

- [ ] **Step 5: Populate `goalStats` at the call site**

At `main.go:1451`, add the two fields:
```go
		gs := goalStats{today: todayWords(pg, proj.words), dailyGoal: pg.DailyGoal, project: proj.words, projectGoal: pg.ProjectGoal,
			sessionSecs: int(m.now.Sub(m.sessionStart).Seconds()), sessionGoalMin: pg.SessionGoalMin}
```

- [ ] **Step 6: Add a session step to the `ctrl+g` prompt**

Update `m.goalPromptField` comment to `// 0 off, 1 daily, 2 project, 3 session`. In the prompt `enter` handler (`main.go` ~857): when `goalPromptField == 2`, instead of saving, set `pg.ProjectGoal`, advance to field 3, and seed `m.nameInput` with `pg.applyEnvDefaults().SessionGoalMin` (or `pg.SessionGoalMin`); add a new final branch for field 3 that sets `pg.SessionGoalMin = n` (if `n >= 0`), saves, and closes. Update whatever renders the prompt label (search for the prompt's field label text) to show `session minutes` for field 3.

- [ ] **Step 7: Build + test + vet**

```
/opt/homebrew/bin/gofmt -w goals.go inspector.go main.go goals_test.go inspector_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: both new tests PASS; existing inspector tests still pass (goalStats literals default the new fields to 0).

- [ ] **Step 8: Commit**

```
git add goals.go inspector.go main.go goals_test.go inspector_test.go
git commit -m "feat(timer): per-session time goal in the Goals tab"
```

---

## Task 3: Pomodoro sprint (ctrl+u + status-bar countdown)

**Files:**
- Modify: `goals.go` (`projectGoals.SprintMin` + default), `main.go` (sprint state fields, `ctrl+u` handler, tick transition, stats-string countdown), and add `sprintDisplay`/`advanceSprint` helpers
- Test: `timer_test.go`

**Interfaces:**
- Consumes: `m.now`, `fmtDuration`, `accent`, `errColor`.
- Produces: `func sprintDisplay(remaining time.Duration, onBreak bool) string`; `func advanceSprint(now, end time.Time, onBreak bool, breakDur time.Duration) (newEnd time.Time, nowOnBreak, stillActive bool, msg string)`; model fields `sprintActive bool`, `sprintEnd time.Time`, `sprintOnBreak bool`.

- [ ] **Step 1: Write the failing tests**

Add to `timer_test.go`:
```go
func TestSprintDisplay(t *testing.T) {
	if got := sprintDisplay(90*time.Second, false); got != "🍅 1:30" {
		t.Fatalf("work display = %q", got)
	}
	if got := sprintDisplay(30*time.Second, true); got != "☕ 0:30" {
		t.Fatalf("break display = %q", got)
	}
}

func TestAdvanceSprint(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	// work ends → break begins (5 min), still active.
	end, onBreak, active, _ := advanceSprint(base, base, false, 5*time.Minute)
	if !active || !onBreak || !end.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("work→break wrong: active=%v onBreak=%v end=%v", active, onBreak, end)
	}
	// break ends → sprint complete, inactive.
	_, _, active2, _ := advanceSprint(base, base, true, 5*time.Minute)
	if active2 {
		t.Fatalf("break end should deactivate the sprint")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run 'TestSprintDisplay|TestAdvanceSprint' -v`
Expected: FAIL (undefined helpers).

- [ ] **Step 3: Add the helpers**

In `main.go`:
```go
// sprintDisplay renders a pomodoro countdown: 🍅 for work, ☕ for a break.
func sprintDisplay(remaining time.Duration, onBreak bool) string {
	icon := "🍅"
	if onBreak {
		icon = "☕"
	}
	return icon + " " + fmtDuration(remaining)
}

// advanceSprint transitions a sprint that has reached its end: work → a break of breakDur, then
// break → complete (inactive). Returns the new end, the new on-break flag, whether it's still
// active, and a status message for the transition.
func advanceSprint(now, end time.Time, onBreak bool, breakDur time.Duration) (time.Time, bool, bool, string) {
	if onBreak {
		return end, false, false, "✓ sprint complete"
	}
	return now.Add(breakDur), true, true, "🍅 done — break time"
}
```

- [ ] **Step 4: Add the model fields + `SprintMin` default**

In `main.go` model, near the timer fields:
```go
	sprintActive  bool
	sprintEnd     time.Time
	sprintOnBreak bool
```
In `goals.go`, add `SprintMin int json:"sprintMin"` to `projectGoals`, and in `applyEnvDefaults` default it to 25 when 0:
```go
	if pg.SprintMin == 0 {
		pg.SprintMin = 25
	}
```

- [ ] **Step 5: `ctrl+u` starts/stops a sprint**

In the editor-screen key handler (where `ctrl+g` etc. live, `main.go` ~1223), add:
```go
		case "ctrl+u":
			if m.sprintActive {
				m.sprintActive = false
				m.status = "sprint stopped"
			} else {
				mins := m.goalsAll[m.files.dir].applyEnvDefaults().SprintMin
				m.sprintActive = true
				m.sprintOnBreak = false
				m.sprintEnd = m.now.Add(time.Duration(mins) * time.Minute)
				m.status = fmt.Sprintf("sprint started — %d min", mins)
			}
			return m, nil
```

- [ ] **Step 6: Advance the sprint on each tick**

In the `autosaveTickMsg` handler, after `m.now = now`, add:
```go
		if m.sprintActive && !m.now.Before(m.sprintEnd) {
			end, onBreak, active, msg := advanceSprint(m.now, m.sprintEnd, m.sprintOnBreak, 5*time.Minute)
			m.sprintEnd, m.sprintOnBreak, m.sprintActive = end, onBreak, active
			m.status = msg
		}
```

- [ ] **Step 7: Swap the stopwatch for the countdown in the stats string**

In the stats builder (from Task 1 Step 4), when a sprint is active show the countdown (with a last-60s accent cue) instead of the plain `⏱`:
```go
	timeSeg := "⏱ " + fmtDuration(m.now.Sub(m.sessionStart))
	if m.sprintActive {
		remaining := m.sprintEnd.Sub(m.now)
		disp := sprintDisplay(remaining, m.sprintOnBreak)
		if remaining <= time.Minute {
			disp = lipgloss.NewStyle().Foreground(accent).Render(disp)
		}
		timeSeg = disp
	}
	return fmt.Sprintf("%s words · %s session · %s", commafy(words), signedComma(delta), timeSeg)
```

- [ ] **Step 8: Build + test + vet + eyeball**

```
/opt/homebrew/bin/gofmt -w main.go goals.go timer_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Optionally `go run .`, press `ctrl+u`, and watch the `🍅` countdown (drop `SprintMin` low via `ctrl+g` to see the break transition quickly).

- [ ] **Step 9: Commit**

```
git add main.go goals.go timer_test.go
git commit -m "feat(timer): pomodoro sprint (ctrl+u) with status-bar countdown"
```

---

## Task 4: Document the ctrl+u shortcut

**Files:**
- Modify: `main.go` (`helpText`)

- [ ] **Step 1: Add the F1 line**

In the `helpText` const, add a line near the other writing shortcuts:
```
ctrl+u   start/stop writing sprint
```

- [ ] **Step 2: Build + commit**

```
/opt/homebrew/bin/go build ./...
git add main.go
git commit -m "docs: document ctrl+u sprint in the F1 cheatsheet"
```

---

## Self-review notes
- **Spec coverage:** stopwatch → Task 1; time goal → Task 2; pomodoro → Task 3; help doc → Task 4. All covered.
- **Type consistency:** `fmtDuration(time.Duration) string`, `sprintDisplay(time.Duration, bool) string`, `advanceSprint(time.Time, time.Time, bool, time.Duration) (time.Time, bool, bool, string)`, model `now/sessionStart/sprintActive/sprintEnd/sprintOnBreak`, `projectGoals.SessionGoalMin/SprintMin`, `goalStats.sessionSecs/sessionGoalMin` — consistent across tasks.
- **Purity:** `View()` reads `m.now` (stamped in `initialModel` + each tick); no `time.Now()` in render.
- **No bell in v1** (visual cue only). Time goal is per-session. Existing `goalStats{}` literals in `inspector_test.go` still compile (new fields default to 0).
