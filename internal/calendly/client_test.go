package calendly

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"verificador-citas-eros/internal/envx"
)

func TestClientListsEventsAndResolvesInvitees(t *testing.T) {
	client := NewClient(envx.CalendlySettings{
		BaseURL:      "https://example.test",
		Token:        "token",
		PageSize:     10,
		EventTypeURI: "https://api.calendly.com/event_types/TYPE-1",
	})
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			var payload any

			switch {
			case req.URL.Path == "/users/me":
				payload = map[string]any{
					"resource": map[string]any{
						"uri":                  "https://api.calendly.com/users/USER-1",
						"current_organization": "https://api.calendly.com/organizations/ORG-1",
					},
				}
			case req.URL.Path == "/scheduled_events" && req.URL.Query().Get("page_token") == "":
				payload = map[string]any{
					"collection": []map[string]any{
						{
							"uri":        "https://api.calendly.com/scheduled_events/EV-1",
							"name":       "Llamada 1",
							"status":     "active",
							"start_time": "2026-03-30T10:00:00Z",
							"end_time":   "2026-03-30T10:30:00Z",
							"event_type": "https://api.calendly.com/event_types/TYPE-1",
						},
					},
					"pagination": map[string]any{
						"next_page": "/scheduled_events?page_token=2",
					},
				}
			case req.URL.Path == "/scheduled_events" && req.URL.Query().Get("page_token") == "2":
				payload = map[string]any{
					"collection": []map[string]any{
						{
							"uri":        "https://api.calendly.com/scheduled_events/EV-2",
							"name":       "Llamada 2",
							"status":     "active",
							"start_time": "2026-03-30T11:00:00Z",
							"end_time":   "2026-03-30T11:30:00Z",
							"event_type": "https://api.calendly.com/event_types/TYPE-1",
						},
					},
					"pagination": map[string]any{},
				}
			case req.URL.Path == "/scheduled_events/EV-1/invitees":
				payload = map[string]any{
					"collection": []map[string]any{
						{
							"name":   "Ana",
							"email":  "ana@example.com",
							"status": "active",
						},
					},
				}
			case req.URL.Path == "/scheduled_events/EV-2/invitees":
				payload = map[string]any{
					"collection": []map[string]any{
						{
							"name":                 "Beto",
							"email":                "beto@example.com",
							"status":               "active",
							"text_reminder_number": "+5215512345678",
						},
					},
				}
			default:
				t.Fatalf("unexpected request path: %s", req.URL.String())
			}

			body, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}),
	}

	result, err := client.ListScheduledEvents(context.Background(), time.Now().UTC(), time.Now().UTC().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListScheduledEvents returned error: %v", err)
	}
	if result.ScopeUsed != "organization" {
		t.Fatalf("unexpected scope used: %s", result.ScopeUsed)
	}
	if len(result.RawEvents) != 2 {
		t.Fatalf("expected 2 raw events, got %d", len(result.RawEvents))
	}
	appointments := result.FilteredAppointments
	if len(appointments) != 2 {
		t.Fatalf("expected 2 filtered appointments, got %d", len(appointments))
	}

	appointments, identityStatus, err := client.ResolveInviteeIdentities(context.Background(), appointments)
	if err != nil {
		t.Fatalf("ResolveInviteeIdentities returned error: %v", err)
	}
	if identityStatus != "completed" {
		t.Fatalf("unexpected identity status: %s", identityStatus)
	}
	if appointments[0].InviteeEmail != "ana@example.com" {
		t.Fatalf("unexpected invitee email: %s", appointments[0].InviteeEmail)
	}
	if appointments[1].InviteePhone != "+5215512345678" {
		t.Fatalf("unexpected invitee phone: %s", appointments[1].InviteePhone)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
