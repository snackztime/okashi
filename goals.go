package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// projectGoals is one project's goal state, stored in the global goals.json
// keyed by project dir path.
type projectGoals struct {
	DailyGoal      int    `json:"dailyGoal"`
	ProjectGoal    int    `json:"projectGoal"`
	SessionGoalMin int    `json:"sessionGoalMin"`
	SprintMin      int    `json:"sprintMin"`
	DayBaseline    int    `json:"dayBaseline"`
	Day            string `json:"day"`
}

func goalsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "goals.json")
}

func loadGoals(path string) map[string]projectGoals {
	m := map[string]projectGoals{}
	if path == "" {
		return m
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &m)
	}
	return m
}

func saveGoals(path string, m map[string]projectGoals) {
	if path == "" {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = atomicWrite(path, data, 0o644)
}

func envGoal(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// applyEnvDefaults fills zero goals from env (OKASHI_DAILY_GOAL=500, OKASHI_PROJECT_GOAL=0).
func (pg projectGoals) applyEnvDefaults() projectGoals {
	if pg.DailyGoal == 0 {
		pg.DailyGoal = envGoal("OKASHI_DAILY_GOAL", 500)
	}
	if pg.ProjectGoal == 0 {
		pg.ProjectGoal = envGoal("OKASHI_PROJECT_GOAL", 0)
	}
	if pg.SprintMin == 0 {
		pg.SprintMin = 25
	}
	return pg
}

func today() string { return time.Now().Format("2006-01-02") }

// rolloverIfNeeded resets the daily baseline when the date changed; changed=true
// means the caller should persist.
func rolloverIfNeeded(pg projectGoals, total int, today string) (projectGoals, bool) {
	if pg.Day != today {
		pg.Day = today
		pg.DayBaseline = total
		return pg, true
	}
	return pg, false
}

// todayWords is net words written today (clamped at 0).
func todayWords(pg projectGoals, total int) int {
	if d := total - pg.DayBaseline; d > 0 {
		return d
	}
	return 0
}
