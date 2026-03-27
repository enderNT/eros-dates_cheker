package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"verificador-citas-eros/internal/appmodel"
	"verificador-citas-eros/internal/config"
	"verificador-citas-eros/internal/store"
)

const historyLimit = 50

var runCounter atomic.Uint64

type calendlyClient interface {
	ListScheduledEvents(ctx context.Context, windowStart, windowEnd time.Time) ([]appmodel.Appointment, string, error)
	ResolveInviteeIdentities(ctx context.Context, appointments []appmodel.Appointment) ([]appmodel.Appointment, string, error)
}

type Service struct {
	store  *store.FileStore
	client calendlyClient

	mu      sync.RWMutex
	cfg     config.SchedulerConfig
	history []appmodel.ValidationRun
	running bool
}

func New(fileStore *store.FileStore, client calendlyClient) (*Service, error) {
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
		ServerTime: now.UTC(),
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

	now := time.Now().UTC()
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

	appointments, scopeUsed, err := s.client.ListScheduledEvents(ctx, run.WindowStart, run.WindowEnd)
	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		run.EndedAt = time.Now().UTC()
		s.finishRun(run)
		return &run, err
	}

	run.ScopeUsed = scopeUsed
	run.Events = appointments
	run.EventsFound = len(appointments)

	appointments, identityStatus, identityErr := s.client.ResolveInviteeIdentities(ctx, appointments)
	run.Events = appointments
	run.IdentityResolutionStatus = identityStatus
	run.EndedAt = time.Now().UTC()

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
		log.Printf("error guardando historial: %v", err)
	}
}
