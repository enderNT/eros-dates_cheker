package scheduler

import (
	"context"
	"log"
	"time"

	"verificador-citas-eros/internal/service"
)

const heartbeat = 15 * time.Second

type Scheduler struct {
	service *service.Service
}

func New(service *service.Service) *Scheduler {
	return &Scheduler{service: service}
}

func (s *Scheduler) Start(ctx context.Context) {
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
		log.Printf("scheduler: %v", err)
	}
}
