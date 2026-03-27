package appmodel

import "time"

type Appointment struct {
	EventURI         string    `json:"event_uri"`
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	EventName        string    `json:"event_name"`
	InviteeName      string    `json:"invitee_name,omitempty"`
	InviteeEmail     string    `json:"invitee_email,omitempty"`
	InviteePhone     string    `json:"invitee_phone,omitempty"`
	IdentityResolved bool      `json:"identity_resolved"`
}

type ValidationRun struct {
	ID                       string        `json:"id"`
	Trigger                  string        `json:"trigger"`
	StartedAt                time.Time     `json:"started_at"`
	EndedAt                  time.Time     `json:"ended_at"`
	WindowStart              time.Time     `json:"window_start"`
	WindowEnd                time.Time     `json:"window_end"`
	ScopeUsed                string        `json:"scope_used,omitempty"`
	EventsFound              int           `json:"events_found"`
	IdentityResolutionStatus string        `json:"identity_resolution_status"`
	Status                   string        `json:"status"`
	Error                    string        `json:"error,omitempty"`
	Events                   []Appointment `json:"events"`
}

type StatusSnapshot struct {
	Running    bool           `json:"running"`
	ServerTime time.Time      `json:"server_time"`
	NextRunAt  *time.Time     `json:"next_run_at,omitempty"`
	LastRun    *ValidationRun `json:"last_run,omitempty"`
}
