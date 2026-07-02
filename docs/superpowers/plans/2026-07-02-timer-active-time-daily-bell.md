# Timer upgrades: active-time + daily goal + bell — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the session stopwatch count active writing time (pause on idle), make the time goal daily-accumulated active minutes, and ring a bell at pomodoro transitions.

**Architecture:** All timing already runs off the 1-second `autosaveTick` and a model `m.now`. The active clock increments per tick only when the writer edited within the idle window (`m.lastEditAt`). The daily accumulator lives in `projectGoals` (per-dir, its own `TimeDay` rollover), persisted ~once a minute.

**Tech Stack:** Go 1.25 (invoke `/opt/homebrew/bin/go` and `/opt/homebrew/bin/gofmt` — not on PATH), Bubble Tea, lipgloss.

## Global Constraints

- `go` → `/opt/homebrew/bin/go`; `gofmt` → `/opt/homebrew/bin/gofmt`. Module `okashi`, flat `package main`.
- `View()` stays pure — timing reads `m.now`/`m.activeSecs`/`m.lastEditAt`, never `time.Now()` in render.
- `activeIdle = 2 * time.Minute`. Daily persistence is ~1/min (no persist-on-quit).
- The daily time accumulator uses its OWN `TimeDay` field (do NOT reuse the word goal's `Day`/`DayBaseline` — that stays owned by the word rollover).
- The stopwatch now shows ACTIVE time (not wall-clock); the pomodoro countdown swap (shipped) is unchanged.
- After every task: `/opt/homebrew/bin/gofmt -w <files>`, `/opt/homebrew/bin/go build ./...`, `/opt/homebrew/bin/go test ./...`, `/opt/homebrew/bin/go vet ./...` all clean before commit.
- Facts: `m.lastEditAt` set on edits (main.go:275); tick handler is the `autosaveTickMsg` branch (main.go ~708, `m.now = now`); stats builder ~main.go:2260 (`timeSeg := "⏱ " + fmtDuration(m.now.Sub(m.sessionStart))`); `goalStats` at inspector.go:146; the SESSION render at inspector.go ~324; `goalStats{}` built at main.go:1451; `projectGoals`/`saveGoals`/`loadGoals`/`today()`/`rolloverIfNeeded` in goals.go; `fmtDuration`/`isWritingActive` (Task 1) shared.

---

## Task 1: Active-time engine + stopwatch

**Files:**
- Modify: `main.go` (model field `activeSecs`; `isWritingActive` + `activeIdle`; tick increment; stats builder)
- Test: `timer_test.go`

**Interfaces:**
- Produces: `const activeIdle`; `func isWritingActive(now, lastEdit time.Time, idle time.Duration) bool`; model field `activeSecs int`.

- [ ] **Step 1: Write the failing test**

Append to `timer_test.go`:
```go
func TestIsWritingActive(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if !isWritingActive(now, now.Add(-30*time.Second), activeIdle) {
		t.Fatal("edited 30s ago should be active")
	}
	if isWritingActive(now, now.Add(-3*time.Minute), activeIdle) {
		t.Fatal("edited 3 min ago should be idle")
	}
	if isWritingActive(now, time.Time{}, activeIdle) {
		t.Fatal("never edited (zero lastEdit) should be idle")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestIsWritingActive -v`
Expected: FAIL (`undefined: isWritingActive` / `activeIdle`).

- [ ] **Step 3: Add the const, helper, field, and tick increment**

In `main.go`:
```go
const activeIdle = 2 * time.Minute

// isWritingActive reports whether the writer edited within the idle window.
func isWritingActive(now, lastEdit time.Time, idle time.Duration) bool {
	return now.Sub(lastEdit) < idle
}
```
Add model field near `sessionStart`:
```go
	activeSecs int
```
In the `autosaveTickMsg` handler, right after `m.now = now`:
```go
		if isWritingActive(m.now, m.lastEditAt, activeIdle) {
			m.activeSecs++
		}
```

- [ ] **Step 4: Show active time + `⏸` in the stats string**

Change the `timeSeg` line (stats builder ~main.go:2260) from the wall-clock stopwatch to active time:
```go
	timeSeg := "⏱ " + fmtDuration(time.Duration(m.activeSecs)*time.Second)
	if !isWritingActive(m.now, m.lastEditAt, activeIdle) {
		timeSeg += " ⏸"
	}
```
Leave the following `if m.sprintActive { … }` swap (shipped) exactly as-is below this.

- [ ] **Step 5: Build + test + vet**

```
/opt/homebrew/bin/gofmt -w main.go timer_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: `TestIsWritingActive` PASS. If any existing statsText test hard-codes the old `⏱ <wallclock>` value, it uses `HasPrefix "… · ⏱ "` (from the shipped Task 1) which still holds — but a freshly-constructed model with `lastEditAt` zero will now show `⏱ 0:00 ⏸`; update any such test's prefix to tolerate the trailing ` ⏸` (use `strings.Contains(got, "· ⏱ ")`).

- [ ] **Step 6: Commit**

```
git add main.go timer_test.go
git commit -m "feat(timer): active-time stopwatch (pauses on idle)"
```

---

## Task 2: Daily-accumulated active time (persistence)

**Files:**
- Modify: `goals.go` (`projectGoals` fields + `rolloverActive`), `main.go` (tick accumulate + persist; model field `activeSaveCtr`)
- Test: `goals_test.go`

**Interfaces:**
- Consumes: `isWritingActive`, `today()`, `saveGoals`, `goalsPath`.
- Produces: `projectGoals.ActiveSecsToday int`, `projectGoals.TimeDay string`; `func rolloverActive(pg projectGoals, today string) (projectGoals, bool)`; model field `activeSaveCtr int`.

- [ ] **Step 1: Write the failing tests**

Append to `goals_test.go`:
```go
func TestRolloverActive(t *testing.T) {
	pg := projectGoals{TimeDay: "2026-07-01", ActiveSecsToday: 900}
	got, changed := rolloverActive(pg, "2026-07-02")
	if !changed || got.ActiveSecsToday != 0 || got.TimeDay != "2026-07-02" {
		t.Fatalf("date change should reset: %+v changed=%v", got, changed)
	}
	same, changed2 := rolloverActive(got, "2026-07-02")
	if changed2 || same.ActiveSecsToday != 0 {
		t.Fatalf("same day should be a no-op: %+v changed=%v", same, changed2)
	}
}

func TestActiveSecsTodayRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goals.json")
	saveGoals(path, map[string]projectGoals{"/p": {ActiveSecsToday: 1234, TimeDay: "2026-07-02"}})
	got := loadGoals(path)
	if got["/p"].ActiveSecsToday != 1234 || got["/p"].TimeDay != "2026-07-02" {
		t.Fatalf("round-trip lost active time: %+v", got["/p"])
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run 'TestRolloverActive|TestActiveSecsTodayRoundTrips' -v`
Expected: FAIL (undefined fields / `rolloverActive`).

- [ ] **Step 3: Add the fields + rollover helper**

In `goals.go`, add to `projectGoals`:
```go
	ActiveSecsToday int    `json:"activeSecsToday"`
	TimeDay         string `json:"timeDay"`
```
Add:
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

- [ ] **Step 4: Accumulate + persist in the tick**

Add model field `activeSaveCtr int`. Extend the active branch in the tick handler (from Task 1 Step 3):
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
			if m.activeSaveCtr >= 60 {
				saveGoals(goalsPath(), m.goalsAll)
				m.activeSaveCtr = 0
			}
		}
```

- [ ] **Step 5: Build + test + vet**

```
/opt/homebrew/bin/gofmt -w goals.go main.go goals_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: both new tests PASS; full suite green.

- [ ] **Step 6: Commit**

```
git add goals.go main.go goals_test.go
git commit -m "feat(timer): daily-accumulated active time (per-dir, ~1/min persist)"
```

---

## Task 3: Goals-tab TIME section (Session + Today)

**Files:**
- Modify: `inspector.go` (`goalStats` + TIME render), `main.go` (`goalStats{}` population + prompt label)
- Test: `inspector_test.go`

**Interfaces:**
- Consumes: `fmtDuration`, `m.activeSecs`, `m.lastEditAt`, `isWritingActive`, `pg.ActiveSecsToday`, `pg.SessionGoalMin`.
- Produces: `goalStats` gains `todayActiveSecs int`, `idle bool`.

- [ ] **Step 1: Write the failing test**

Update `TestInspectorGoalsSessionSection` in `inspector_test.go` (the bar now reflects TODAY's active time; header is "TIME"):
```go
func TestInspectorGoalsSessionSection(t *testing.T) {
	in := inspectorModel{visible: true, tab: tabGoals}
	out := ansi.Strip(in.View(28, docStats{}, projStats{}, "", goalStats{sessionSecs: 300, todayActiveSecs: 600, sessionGoalMin: 30, idle: true}, analysisState{}))
	if !strings.Contains(out, "TIME") || !strings.Contains(out, "10 / 30 min") {
		t.Fatalf("expected a TIME section with 10/30 min, got:\n%s", out)
	}
	if !strings.Contains(out, "⏸") {
		t.Fatalf("idle should show ⏸, got:\n%s", out)
	}
}
```
(Match the existing `inspector_test.go` construction style — same `in.View(...)` signature.)

- [ ] **Step 2: Run to confirm failure**

Run: `/opt/homebrew/bin/go test . -run TestInspectorGoalsSessionSection -v`
Expected: FAIL (`todayActiveSecs`/`idle` undefined; header still "SESSION").

- [ ] **Step 3: Extend `goalStats` + render the TIME section**

In `inspector.go`, extend the struct (line 146):
```go
type goalStats struct {
	today, dailyGoal, project, projectGoal, sessionSecs, sessionGoalMin, todayActiveSecs int
	idle                                                                                 bool
}
```
Replace the SESSION render block (inspector.go ~324, the `sectionHeader("Session"...)` + Elapsed + bar) with the TIME block from the spec §3:
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

- [ ] **Step 4: Populate the new fields at main.go:1451**

```go
		gs := goalStats{today: todayWords(pg, proj.words), dailyGoal: pg.DailyGoal, project: proj.words, projectGoal: pg.ProjectGoal,
			sessionSecs: m.activeSecs, sessionGoalMin: pg.SessionGoalMin,
			todayActiveSecs: pg.ActiveSecsToday, idle: !isWritingActive(m.now, m.lastEditAt, activeIdle)}
```
(Also: if the `ctrl+g` field-3 prompt label reads "session minutes", change it to "daily minutes" — grep the prompt label in `statusBar()`.)

- [ ] **Step 5: Build + test + vet**

```
/opt/homebrew/bin/gofmt -w inspector.go main.go inspector_test.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Expected: the updated inspector test PASSES; other inspector tests still compile (new fields default to 0/false).

- [ ] **Step 6: Commit**

```
git add inspector.go main.go inspector_test.go
git commit -m "feat(timer): Goals-tab TIME section (active session + daily accumulated)"
```

---

## Task 4: Pomodoro bell

**Files:**
- Modify: `main.go` (`bellCmd` + wire into the tick transition)

**Interfaces:**
- Consumes: `advanceSprint`, `autosaveTick`.
- Produces: `func bellCmd() tea.Cmd`.

- [ ] **Step 1: Add `bellCmd`**

```go
// bellCmd rings the terminal bell once. Fired at a pomodoro transition.
func bellCmd() tea.Cmd {
	return func() tea.Msg {
		os.Stdout.Write([]byte{0x07})
		return nil
	}
}
```
(`os` is already imported in main.go.)

- [ ] **Step 2: Wire it into the tick transition**

In the `autosaveTickMsg` handler, when the sprint transition fires (the `if m.sprintActive && !m.now.Before(m.sprintEnd)` block from the shipped Task 3), append a bell to the returned command batch. Ensure the handler returns `tea.Batch(cmds...)` including `autosaveTick()`, any grammar cmd, and `bellCmd()` on a transition. Example consolidation:
```go
		cmds := []tea.Cmd{autosaveTick()}
		if m.autoRecheckDue(now) {
			m.checkingGrammar = true
			m.lastGrammarCheck = now
			cmds = append(cmds, checkGrammarCmd(m.grammarChecker, m.currentFile, m.editor.Value()))
		}
		if m.sprintActive && !m.now.Before(m.sprintEnd) {
			end, onBreak, active, msg := advanceSprint(m.now, m.sprintEnd, m.sprintOnBreak, 5*time.Minute)
			m.sprintEnd, m.sprintOnBreak, m.sprintActive = end, onBreak, active
			m.status = msg
			cmds = append(cmds, bellCmd())
		}
		return m, tea.Batch(cmds...)
```
Preserve the existing autosave (`if m.autosaveDue(now) { m.save() }`) behavior above this — only the *return*/command assembly is being consolidated, not the autosave/grammar/sprint logic.

- [ ] **Step 3: Build + test + vet**

```
/opt/homebrew/bin/gofmt -w main.go
/opt/homebrew/bin/go build ./... && /opt/homebrew/bin/go test ./... && /opt/homebrew/bin/go vet ./...
```
Optionally `go run .`, start a short sprint (`ctrl+g` set a low sprint length, `ctrl+u`), and confirm a bell + status message at the work→break transition.

- [ ] **Step 4: Commit**

```
git add main.go
git commit -m "feat(timer): ring the terminal bell at pomodoro transitions"
```

---

## Self-review notes
- **Spec coverage:** active engine+stopwatch → Task 1; daily accumulation → Task 2; TIME section → Task 3; bell → Task 4. All covered.
- **Type consistency:** `isWritingActive`, `activeIdle`, `activeSecs`, `activeSaveCtr`, `rolloverActive`, `projectGoals.ActiveSecsToday/TimeDay`, `goalStats.todayActiveSecs/idle` — consistent across tasks.
- **Purity:** the stats builder + inspector render read `m.activeSecs`/`m.lastEditAt`/`m.now` (stamped in Update), never `time.Now()`.
- **Decoupling:** `TimeDay` is separate from the word goal's `Day`/`DayBaseline`; the two rollovers never touch each other's fields.
- **No persist-on-quit** — ~1/min cadence is the single persistence path (loss bound <60 s).
