package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	defaultTimezone          = "America/Mexico_City"
	defaultRunInterval       = 20
	defaultLookaheadMinutes  = 120
	defaultWindowLabel       = "Horario general"
	fullDayStart             = "00:00"
	fullDayEnd               = "23:59"
	maxScheduleSearchMinutes = 8 * 24 * 60
)

type SchedulerConfig struct {
	Enabled            bool         `json:"enabled"`
	Timezone           string       `json:"timezone"`
	RunIntervalMinutes int          `json:"run_interval_minutes"`
	LookaheadMinutes   int          `json:"lookahead_minutes"`
	ActiveDays         []int        `json:"active_days"`
	TimeWindows        []TimeWindow `json:"time_windows"`
}

type TimeWindow struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Days  []int  `json:"days"`
	Start string `json:"start"`
	End   string `json:"end"`
}

func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		Enabled:            true,
		Timezone:           defaultTimezone,
		RunIntervalMinutes: defaultRunInterval,
		LookaheadMinutes:   defaultLookaheadMinutes,
		ActiveDays:         []int{0, 1, 2, 3, 4, 5, 6},
		TimeWindows: []TimeWindow{
			{
				ID:    "default-window",
				Label: defaultWindowLabel,
				Days:  []int{0, 1, 2, 3, 4, 5, 6},
				Start: fullDayStart,
				End:   fullDayEnd,
			},
		},
	}
}

func (c SchedulerConfig) Validate() error {
	if _, err := c.Location(); err != nil {
		return err
	}
	if c.RunIntervalMinutes < 1 || c.RunIntervalMinutes > 1440 {
		return errors.New("run_interval_minutes debe estar entre 1 y 1440")
	}
	if c.LookaheadMinutes < 1 || c.LookaheadMinutes > 10080 {
		return errors.New("lookahead_minutes debe estar entre 1 y 10080")
	}
	if len(c.ActiveDays) == 0 {
		return errors.New("active_days debe tener al menos un dia")
	}
	for _, day := range c.ActiveDays {
		if day < 0 || day > 6 {
			return fmt.Errorf("active_days contiene un dia invalido: %d", day)
		}
	}
	if len(c.TimeWindows) == 0 {
		return errors.New("time_windows debe tener al menos una franja")
	}
	for _, window := range c.TimeWindows {
		if err := validateWindow(window); err != nil {
			return err
		}
	}
	return nil
}

func (c SchedulerConfig) Normalized() SchedulerConfig {
	out := c
	out.Timezone = strings.TrimSpace(out.Timezone)
	out.ActiveDays = uniqueSortedDays(out.ActiveDays)
	for i := range out.TimeWindows {
		out.TimeWindows[i].Days = uniqueSortedDays(out.TimeWindows[i].Days)
		if strings.TrimSpace(out.TimeWindows[i].Label) == "" {
			out.TimeWindows[i].Label = fmt.Sprintf("Horario %d", i+1)
		}
		if strings.TrimSpace(out.TimeWindows[i].ID) == "" {
			out.TimeWindows[i].ID = fmt.Sprintf("window-%d", i+1)
		}
	}
	return out
}

func (c SchedulerConfig) Location() (*time.Location, error) {
	if strings.TrimSpace(c.Timezone) == "" {
		return nil, errors.New("timezone es requerido")
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone invalida: %w", err)
	}
	return loc, nil
}

func (c SchedulerConfig) IsWithinWindow(at time.Time) bool {
	loc, err := c.Location()
	if err != nil {
		return false
	}
	local := at.In(loc)
	for _, window := range c.TimeWindows {
		if windowMatches(c, window, local) {
			return true
		}
	}
	return false
}

func (c SchedulerConfig) NextEligibleTime(from time.Time, lastRun *time.Time) (*time.Time, bool) {
	loc, err := c.Location()
	if err != nil {
		return nil, false
	}

	candidate := from.In(loc)
	if c.IsWithinWindow(candidate) && c.intervalSatisfied(candidate, lastRun, loc) {
		candidateUTC := candidate.UTC()
		return &candidateUTC, true
	}

	searchStart := ceilToMinute(candidate)
	if lastRun != nil {
		nextByInterval := lastRun.In(loc).Add(time.Duration(c.RunIntervalMinutes) * time.Minute)
		if nextByInterval.After(searchStart) {
			searchStart = ceilToMinute(nextByInterval)
		}
	}
	for i := 0; i <= maxScheduleSearchMinutes; i++ {
		probe := searchStart.Add(time.Duration(i) * time.Minute)
		if c.IsWithinWindow(probe) {
			probeUTC := probe.UTC()
			return &probeUTC, true
		}
	}
	return nil, false
}

func (c SchedulerConfig) intervalSatisfied(candidate time.Time, lastRun *time.Time, loc *time.Location) bool {
	if lastRun == nil {
		return true
	}
	nextByInterval := lastRun.In(loc).Add(time.Duration(c.RunIntervalMinutes) * time.Minute)
	return !candidate.Before(nextByInterval)
}

func validateWindow(window TimeWindow) error {
	if strings.TrimSpace(window.Start) == "" || strings.TrimSpace(window.End) == "" {
		return errors.New("cada franja debe incluir start y end")
	}
	startMinutes, err := parseClock(window.Start)
	if err != nil {
		return fmt.Errorf("start invalido en franja %q: %w", window.Label, err)
	}
	endMinutes, err := parseClock(window.End)
	if err != nil {
		return fmt.Errorf("end invalido en franja %q: %w", window.Label, err)
	}
	if startMinutes == endMinutes {
		return fmt.Errorf("la franja %q no puede tener la misma hora de inicio y fin", window.Label)
	}
	for _, day := range window.Days {
		if day < 0 || day > 6 {
			return fmt.Errorf("franja %q contiene un dia invalido: %d", window.Label, day)
		}
	}
	return nil
}

func windowMatches(cfg SchedulerConfig, window TimeWindow, local time.Time) bool {
	minutes, err := minuteOfDay(local.Format("15:04"))
	if err != nil {
		return false
	}
	startMinutes, err := parseClock(window.Start)
	if err != nil {
		return false
	}
	endMinutes, err := parseClock(window.End)
	if err != nil {
		return false
	}

	currentDay := int(local.Weekday())
	if startMinutes < endMinutes {
		return cfg.dayAllowed(currentDay) && cfg.windowDayAllowed(window, currentDay) && minutes >= startMinutes && minutes < endMinutes
	}

	if cfg.dayAllowed(currentDay) && cfg.windowDayAllowed(window, currentDay) && minutes >= startMinutes {
		return true
	}

	previousDay := int(local.Add(-24 * time.Hour).Weekday())
	return cfg.dayAllowed(previousDay) && cfg.windowDayAllowed(window, previousDay) && minutes < endMinutes
}

func (c SchedulerConfig) dayAllowed(day int) bool {
	return slices.Contains(c.ActiveDays, day)
}

func (c SchedulerConfig) windowDayAllowed(window TimeWindow, day int) bool {
	days := window.Days
	if len(days) == 0 {
		days = c.ActiveDays
	}
	return slices.Contains(days, day)
}

func parseClock(value string) (int, error) {
	return minuteOfDay(strings.TrimSpace(value))
}

func minuteOfDay(value string) (int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, errors.New("formato esperado HH:MM")
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, err
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func uniqueSortedDays(days []int) []int {
	if len(days) == 0 {
		return nil
	}
	seen := map[int]struct{}{}
	out := make([]int, 0, len(days))
	for _, day := range days {
		if _, ok := seen[day]; ok {
			continue
		}
		seen[day] = struct{}{}
		out = append(out, day)
	}
	slices.Sort(out)
	return out
}

func ceilToMinute(value time.Time) time.Time {
	if value.Second() == 0 && value.Nanosecond() == 0 {
		return value
	}
	return value.Truncate(time.Minute).Add(time.Minute)
}
