package scheduler

import (
	"context"
	"time"

	"verificador-citas-eros/internal/service"
	"verificador-citas-eros/internal/termlog"
)

const heartbeat = 15 * time.Second

type Scheduler struct {
	service *service.Service
	log     *termlog.Logger
}

func New(service *service.Service, logger *termlog.Logger) *Scheduler {
	if logger == nil {
		logger = termlog.New(nil)
	}
	return &Scheduler{service: service, log: logger}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.tryRun(ctx)
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tryRun(ctx)
		}
	}
}

func (s *Scheduler) tryRun(ctx context.Context) {
	now := time.Now().UTC()
	next := s.service.NextScheduledRun(now)
	if next == nil || next.After(now) {
		return
	}
	if _, err := s.service.RunValidation(ctx, "scheduled"); err != nil {
		s.log.Warn("scheduler encontro un problema al ejecutar", "error", err)
	}
}
