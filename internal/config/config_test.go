package config

import (
	"testing"
	"time"
)

func TestValidateRejectsInvalidInterval(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.RunIntervalMinutes = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid interval")
	}
}

func TestNextEligibleTimeWithinWindow(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.Timezone = "America/Mexico_City"
	cfg.ActiveDays = []int{1}
	cfg.TimeWindows = []TimeWindow{
		{ID: "w1", Label: "Lunes", Days: []int{1}, Start: "09:00", End: "11:00"},
	}

	loc, _ := cfg.Location()
	now := time.Date(2026, 3, 30, 9, 15, 30, 0, loc)
	next, ok := cfg.NextEligibleTime(now, nil)
	if !ok || next == nil {
		t.Fatal("expected next eligible time")
	}
	if next.In(loc).Format("15:04:05") != "09:15:30" {
		t.Fatalf("unexpected next time: %s", next.In(loc).Format(time.RFC3339))
	}
}

func TestNextEligibleTimeMovesToNextWindow(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.Timezone = "America/Mexico_City"
	cfg.ActiveDays = []int{1}
	cfg.TimeWindows = []TimeWindow{
		{ID: "w1", Label: "Lunes", Days: []int{1}, Start: "09:00", End: "10:00"},
	}

	loc, _ := cfg.Location()
	now := time.Date(2026, 3, 30, 11, 0, 0, 0, loc)
	next, ok := cfg.NextEligibleTime(now, nil)
	if !ok || next == nil {
		t.Fatal("expected next eligible time on the following monday")
	}
	if next.In(loc).Weekday() != time.Monday || next.In(loc).Format("15:04") != "09:00" {
		t.Fatalf("unexpected next time: %s", next.In(loc).Format(time.RFC3339))
	}
}

func TestNextEligibleTimeRunsNowWhenIntervalAlreadyElapsed(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.Timezone = "America/Mexico_City"
	cfg.ActiveDays = []int{1}
	cfg.RunIntervalMinutes = 1
	cfg.TimeWindows = []TimeWindow{
		{ID: "w1", Label: "Lunes", Days: []int{1}, Start: "09:00", End: "11:00"},
	}

	loc, _ := cfg.Location()
	lastRun := time.Date(2026, 3, 30, 9, 0, 0, 0, loc)
	now := time.Date(2026, 3, 30, 9, 5, 45, 0, loc)
	next, ok := cfg.NextEligibleTime(now, &lastRun)
	if !ok || next == nil {
		t.Fatal("expected next eligible time")
	}
	if next.In(loc).Format("15:04:05") != "09:05:45" {
		t.Fatalf("unexpected next time: %s", next.In(loc).Format(time.RFC3339))
	}
}

func TestIsWithinWindowHandlesOvernight(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.Timezone = "America/Mexico_City"
	cfg.ActiveDays = []int{5}
	cfg.TimeWindows = []TimeWindow{
		{ID: "w1", Label: "Viernes noche", Days: []int{5}, Start: "22:00", End: "02:00"},
	}

	loc, _ := cfg.Location()
	saturdayEarly := time.Date(2026, 4, 4, 1, 0, 0, 0, loc)
	if !cfg.IsWithinWindow(saturdayEarly) {
		t.Fatal("expected overnight window to include early saturday")
	}
}
