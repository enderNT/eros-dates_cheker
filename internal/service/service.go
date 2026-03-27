package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"verificador-citas-eros/internal/appmodel"
	"verificador-citas-eros/internal/calendly"
	"verificador-citas-eros/internal/config"
	"verificador-citas-eros/internal/store"
	"verificador-citas-eros/internal/termlog"
)

const historyLimit = 50

var runCounter atomic.Uint64

type calendlyClient interface {
	ListScheduledEvents(ctx context.Context, windowStart, windowEnd time.Time) (calendly.FetchResult, error)
	ResolveInviteeIdentities(ctx context.Context, appointments []appmodel.Appointment) ([]appmodel.Appointment, string, error)
}

type Service struct {
	store  *store.FileStore
	client calendlyClient
	log    *termlog.Logger
	now    func() time.Time

	mu      sync.RWMutex
	cfg     config.SchedulerConfig
	history []appmodel.ValidationRun
	running bool
}

func New(fileStore *store.FileStore, client calendlyClient, logger *termlog.Logger) (*Service, error) {
	if logger == nil {
		logger = termlog.New(nil)
	}
	cfg, err := fileStore.LoadConfig()
	if err != nil {
		return nil, err
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		cfg = config.DefaultSchedulerConfig()
	}
	if err := fileStore.SaveConfig(cfg); err != nil {
		return nil, err
	}

	history, err := fileStore.LoadHistory()
	if err != nil {
		return nil, err
	}

	return &Service{
		store:   fileStore,
		client:  client,
		log:     logger,
		now:     time.Now,
		cfg:     cfg,
		history: history,
	}, nil
}

func (s *Service) GetConfig() config.SchedulerConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Service) UpdateConfig(cfg config.SchedulerConfig) (config.SchedulerConfig, error) {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return config.SchedulerConfig{}, err
	}
	if err := s.store.SaveConfig(cfg); err != nil {
		return config.SchedulerConfig{}, err
	}

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()

	return cfg, nil
}

func (s *Service) GetHistory() []appmodel.ValidationRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.history)
}

func (s *Service) GetStatus(now time.Time) appmodel.StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := appmodel.StatusSnapshot{
		Running:    s.running,
		ServerTime: s.inConfigLocation(now, s.cfg),
	}
	if len(s.history) > 0 {
		lastRun := s.history[0]
		snapshot.LastRun = &lastRun
		var lastTime *time.Time
		if !lastRun.EndedAt.IsZero() {
			lastEnded := lastRun.EndedAt
			lastTime = &lastEnded
		}
		snapshot.NextRunAt, _ = s.cfg.NextEligibleTime(now, lastTime)
		return snapshot
	}

	snapshot.NextRunAt, _ = s.cfg.NextEligibleTime(now, nil)
	return snapshot
}

func (s *Service) NextScheduledRun(now time.Time) *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.running || !s.cfg.Enabled {
		return nil
	}
	if len(s.history) == 0 {
		next, _ := s.cfg.NextEligibleTime(now, nil)
		return next
	}
	lastEnded := s.history[0].EndedAt
	next, _ := s.cfg.NextEligibleTime(now, &lastEnded)
	return next
}

func (s *Service) RunValidation(ctx context.Context, trigger string) (*appmodel.ValidationRun, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, errors.New("ya hay una validacion en curso")
	}
	s.running = true
	cfg := s.cfg
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	now := s.currentBusinessTime(cfg)
	run := appmodel.ValidationRun{
		ID:                       fmt.Sprintf("run-%d", runCounter.Add(1)),
		Trigger:                  trigger,
		StartedAt:                now,
		WindowStart:              now,
		WindowEnd:                now.Add(time.Duration(cfg.LookaheadMinutes) * time.Minute),
		Status:                   "running",
		IdentityResolutionStatus: "not_started",
		Events:                   []appmodel.Appointment{},
	}
	s.logRunStart(run, cfg)

	fetchResult, err := s.client.ListScheduledEvents(ctx, run.WindowStart, run.WindowEnd)
	run.ScopeUsed = fetchResult.ScopeUsed
	run.Events = fetchResult.FilteredAppointments
	run.EventsFound = len(fetchResult.FilteredAppointments)
	s.logFetchResult(fetchResult)
	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		run.EndedAt = s.currentBusinessTime(cfg)
		s.log.Error("consulta a Calendly fallo", "run_id", run.ID, "error", err)
		s.logRunFinished(run)
		s.finishRun(run)
		return &run, err
	}

	s.log.Step("resolviendo identidad de invitados", "run_id", run.ID, "appointments", len(run.Events))
	appointments, identityStatus, identityErr := s.client.ResolveInviteeIdentities(ctx, run.Events)
	run.Events = appointments
	run.IdentityResolutionStatus = identityStatus
	run.EndedAt = s.currentBusinessTime(cfg)

	switch {
	case identityErr != nil:
		run.Status = "partial"
		run.Error = identityErr.Error()
	case len(appointments) == 0:
		run.Status = "success"
		if run.IdentityResolutionStatus == "not_started" {
			run.IdentityResolutionStatus = "skipped"
		}
	default:
		run.Status = "success"
	}

	s.logIdentityResult(run)
	s.logRunFinished(run)
	s.finishRun(run)
	if identityErr != nil {
		return &run, identityErr
	}
	return &run, nil
}

func (s *Service) finishRun(run appmodel.ValidationRun) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append([]appmodel.ValidationRun{run}, s.history...)
	if len(s.history) > historyLimit {
		s.history = s.history[:historyLimit]
	}
	if err := s.store.SaveHistory(s.history); err != nil {
		s.log.Error("error guardando historial", "error", err)
	}
}

func (s *Service) logRunStart(run appmodel.ValidationRun, cfg config.SchedulerConfig) {
	s.log.RunStart("Nueva ejecucion de validacion",
		"run_id", run.ID,
		"trigger", run.Trigger,
		"window_start", run.WindowStart,
		"window_end", run.WindowEnd,
	)
	s.log.Step("configuracion activa",
		"timezone", cfg.Timezone,
		"interval_minutes", cfg.RunIntervalMinutes,
		"lookahead_minutes", cfg.LookaheadMinutes,
		"active_days", len(cfg.ActiveDays),
		"time_windows", len(cfg.TimeWindows),
	)
}

func (s *Service) logFetchResult(result calendly.FetchResult) {
	s.log.Divider()
	s.log.Step("respuesta general de Calendly",
		"scope", result.ScopeUsed,
		"pages", result.PagesFetched,
		"raw_events", len(result.RawEvents),
		"filtered_events", len(result.FilteredAppointments),
	)
	excludedSummary := map[string]int{}
	for _, item := range result.RawEvents {
		if item.Included {
			continue
		}
		reason := item.ExcludeReason
		if reason == "" {
			reason = "otro"
		}
		excludedSummary[reason]++
	}
	if len(excludedSummary) > 0 {
		s.log.Info("resumen de exclusiones", "details", strings.Join(termlog.SortedKVLines(excludedSummary), ", "))
	}
	s.log.Divider()
	s.log.Step("citas filtradas para el objetivo de proximidad", "count", len(result.FilteredAppointments), "showing", minInt(len(result.FilteredAppointments), 8))
	rows := make([][]string, 0, minInt(len(result.FilteredAppointments), 8))
	for _, item := range limitAppointments(result.FilteredAppointments, 8) {
		rows = append(rows, []string{
			item.StartTime.Local().Format("2006-01-02 15:04"),
			item.EndTime.Local().Format("15:04"),
			clip(item.EventName, 28),
			shortEventURI(item.EventURI),
		})
	}
	s.log.Table("Proximas citas filtradas", []string{"Inicio", "Fin", "Evento", "ID"}, rows)
	if len(result.FilteredAppointments) > 8 {
		s.log.Info("citas filtradas restantes no impresas", "count", len(result.FilteredAppointments)-8)
	}
}

func (s *Service) logIdentityResult(run appmodel.ValidationRun) {
	resolved := 0
	for _, appointment := range run.Events {
		if appointment.IdentityResolved {
			resolved++
		}
	}
	s.log.Divider()
	if run.Error != "" {
		s.log.Warn("resultado de resolucion de identidad",
			"run_id", run.ID,
			"identity_status", run.IdentityResolutionStatus,
			"resolved", resolved,
			"events", len(run.Events),
			"error", run.Error,
		)
		return
	}
	s.log.Success("resultado de resolucion de identidad",
		"run_id", run.ID,
		"identity_status", run.IdentityResolutionStatus,
		"resolved", resolved,
		"events", len(run.Events),
	)
}

func (s *Service) logRunFinished(run appmodel.ValidationRun) {
	s.log.Divider()
	ok := run.Status == "success"
	switch run.Status {
	case "failed":
		s.log.Error("ejecucion finalizada",
			"run_id", run.ID,
			"status", run.Status,
			"events_found", run.EventsFound,
			"duration", run.EndedAt.Sub(run.StartedAt),
			"error", run.Error,
		)
	case "partial":
		s.log.Warn("ejecucion finalizada",
			"run_id", run.ID,
			"status", run.Status,
			"events_found", run.EventsFound,
			"duration", run.EndedAt.Sub(run.StartedAt),
			"error", run.Error,
		)
	default:
		s.log.Success("ejecucion finalizada",
			"run_id", run.ID,
			"status", run.Status,
			"events_found", run.EventsFound,
			"duration", run.EndedAt.Sub(run.StartedAt),
		)
	}
	s.log.RunEnd("Fin de ejecucion", ok,
		"run_id", run.ID,
		"status", run.Status,
		"events_found", run.EventsFound,
		"duration", run.EndedAt.Sub(run.StartedAt),
	)
}

func limitAppointments(items []appmodel.Appointment, limit int) []appmodel.Appointment {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func shortEventURI(uri string) string {
	parts := strings.Split(strings.TrimRight(uri, "/"), "/")
	if len(parts) == 0 {
		return uri
	}
	return parts[len(parts)-1]
}

func clip(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 1 {
		return value[:limit]
	}
	return value[:limit-1] + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) currentBusinessTime(cfg config.SchedulerConfig) time.Time {
	return s.inConfigLocation(s.now(), cfg)
}

func (s *Service) inConfigLocation(value time.Time, cfg config.SchedulerConfig) time.Time {
	loc, err := cfg.Location()
	if err != nil {
		return value.UTC()
	}
	return value.In(loc)
}
