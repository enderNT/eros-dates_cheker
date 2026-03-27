package service

import (
	"context"
	"testing"
	"time"

	"verificador-citas-eros/internal/appmodel"
	"verificador-citas-eros/internal/calendly"
	"verificador-citas-eros/internal/config"
	"verificador-citas-eros/internal/store"
)

type fakeCalendlyClient struct{}

func (fakeCalendlyClient) ListScheduledEvents(_ context.Context, windowStart, windowEnd time.Time) (calendly.FetchResult, error) {
	return calendly.FetchResult{
		ScopeUsed:            "organization",
		RawEvents:            []calendly.FetchedEvent{},
		FilteredAppointments: []appmodel.Appointment{},
	}, nil
}

func (fakeCalendlyClient) ResolveInviteeIdentities(_ context.Context, appointments []appmodel.Appointment) ([]appmodel.Appointment, string, error) {
	return appointments, "skipped", nil
}

func TestRunValidationUsesConfiguredTimezoneForWindow(t *testing.T) {
	fileStore, err := store.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore error: %v", err)
	}

	cfg := config.DefaultSchedulerConfig()
	cfg.Timezone = "America/Mexico_City"
	cfg.LookaheadMinutes = 120
	if err := fileStore.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	svc, err := New(fileStore, fakeCalendlyClient{}, nil)
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	fixedUTC := time.Date(2026, 3, 27, 11, 27, 30, 0, time.UTC)
	svc.now = func() time.Time { return fixedUTC }

	run, err := svc.RunValidation(context.Background(), "manual")
	if err != nil {
		t.Fatalf("RunValidation error: %v", err)
	}

	if run.WindowStart.Location().String() != "America/Mexico_City" {
		t.Fatalf("expected WindowStart location America/Mexico_City, got %s", run.WindowStart.Location())
	}
	if run.WindowStart.Format(time.RFC3339) != "2026-03-27T05:27:30-06:00" {
		t.Fatalf("unexpected WindowStart: %s", run.WindowStart.Format(time.RFC3339))
	}
	if run.WindowEnd.Format(time.RFC3339) != "2026-03-27T07:27:30-06:00" {
		t.Fatalf("unexpected WindowEnd: %s", run.WindowEnd.Format(time.RFC3339))
	}
}
