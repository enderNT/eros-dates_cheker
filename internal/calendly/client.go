package calendly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"verificador-citas-eros/internal/appmodel"
	"verificador-citas-eros/internal/envx"
)

type Client struct {
	baseURL      string
	token        string
	organization string
	eventTypeURI string
	pageSize     int
	httpClient   *http.Client
}

type scopeInfo struct {
	Kind string
	URI  string
}

type scheduledEventsResponse struct {
	Collection []scheduledEvent `json:"collection"`
	Pagination paginationLinks  `json:"pagination"`
}

type scheduledEvent struct {
	URI       string    `json:"uri"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	EventType string    `json:"event_type"`
}

type paginationLinks struct {
	NextPage string `json:"next_page"`
}

type inviteesResponse struct {
	Collection []invitee `json:"collection"`
}

type invitee struct {
	Name               string              `json:"name"`
	Email              string              `json:"email"`
	Status             string              `json:"status"`
	TextReminderNumber string              `json:"text_reminder_number"`
	Questions          []questionAndAnswer `json:"questions_and_answers"`
}

type questionAndAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type usersMeResponse struct {
	Resource struct {
		URI          string `json:"uri"`
		Organization string `json:"current_organization"`
	} `json:"resource"`
}

func NewClient(settings envx.CalendlySettings) *Client {
	return &Client{
		baseURL:      strings.TrimRight(settings.BaseURL, "/"),
		token:        settings.Token,
		organization: settings.Organization,
		eventTypeURI: settings.EventTypeURI,
		pageSize:     settings.PageSize,
		httpClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

func (c *Client) ListScheduledEvents(ctx context.Context, windowStart, windowEnd time.Time) ([]appmodel.Appointment, string, error) {
	scope, err := c.resolveScope(ctx)
	if err != nil {
		return nil, "", err
	}

	query := url.Values{}
	query.Set(scope.Kind, scope.URI)
	query.Set("status", "active")
	query.Set("min_start_time", windowStart.UTC().Format(time.RFC3339))
	query.Set("max_start_time", windowEnd.UTC().Format(time.RFC3339))
	query.Set("count", fmt.Sprintf("%d", c.pageSize))

	var appointments []appmodel.Appointment
	nextURL := c.baseURL + "/scheduled_events?" + query.Encode()
	for nextURL != "" {
		var payload scheduledEventsResponse
		if err := c.getJSON(ctx, nextURL, &payload); err != nil {
			return nil, scope.Kind, err
		}

		for _, item := range payload.Collection {
			if item.Status != "" && item.Status != "active" {
				continue
			}
			if c.eventTypeURI != "" && item.EventType != "" && item.EventType != c.eventTypeURI {
				continue
			}
			appointments = append(appointments, appmodel.Appointment{
				EventURI:  item.URI,
				StartTime: item.StartTime.UTC(),
				EndTime:   item.EndTime.UTC(),
				EventName: item.Name,
			})
		}

		nextURL = payload.Pagination.NextPage
		if nextURL != "" && strings.HasPrefix(nextURL, "/") {
			nextURL = c.baseURL + nextURL
		}
	}

	return appointments, scope.Kind, nil
}

func (c *Client) ResolveInviteeIdentities(ctx context.Context, appointments []appmodel.Appointment) ([]appmodel.Appointment, string, error) {
	if len(appointments) == 0 {
		return appointments, "skipped", nil
	}

	resolvedCount := 0
	var resolutionErrors []string

	for index := range appointments {
		inviteesURL := fmt.Sprintf("%s/scheduled_events/%s/invitees", c.baseURL, extractUUID(appointments[index].EventURI))
		var payload inviteesResponse
		if err := c.getJSON(ctx, inviteesURL, &payload); err != nil {
			resolutionErrors = append(resolutionErrors, fmt.Sprintf("%s: %v", appointments[index].EventName, err))
			continue
		}

		selected, ok := selectInvitee(payload.Collection)
		if !ok {
			continue
		}
		appointments[index].InviteeName = selected.Name
		appointments[index].InviteeEmail = selected.Email
		appointments[index].InviteePhone = pickPhone(selected)
		appointments[index].IdentityResolved = appointments[index].InviteeEmail != "" || appointments[index].InviteePhone != "" || appointments[index].InviteeName != ""
		if appointments[index].IdentityResolved {
			resolvedCount++
		}
	}

	switch {
	case len(resolutionErrors) == 0 && resolvedCount == len(appointments):
		return appointments, "completed", nil
	case len(resolutionErrors) == 0:
		return appointments, "partial", nil
	default:
		return appointments, "partial", errors.New(strings.Join(resolutionErrors, "; "))
	}
}

func (c *Client) resolveScope(ctx context.Context) (scopeInfo, error) {
	if c.organization != "" {
		return scopeInfo{Kind: "organization", URI: c.organization}, nil
	}

	var payload usersMeResponse
	if err := c.getJSON(ctx, c.baseURL+"/users/me", &payload); err != nil {
		return scopeInfo{}, fmt.Errorf("resolver scope con /users/me: %w", err)
	}

	if payload.Resource.Organization != "" {
		return scopeInfo{Kind: "organization", URI: payload.Resource.Organization}, nil
	}
	if payload.Resource.URI != "" {
		return scopeInfo{Kind: "user", URI: payload.Resource.URI}, nil
	}
	return scopeInfo{}, errors.New("Calendly no devolvio organization ni user URI")
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("Calendly respondio %d: %s", resp.StatusCode, message)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decodificar respuesta de Calendly: %w", err)
	}
	return nil
}

func extractUUID(uri string) string {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return ""
	}
	return path.Base(trimmed)
}

func selectInvitee(invitees []invitee) (invitee, bool) {
	if len(invitees) == 0 {
		return invitee{}, false
	}
	for _, item := range invitees {
		if item.Status == "active" {
			return item, true
		}
	}
	return invitees[0], true
}

func pickPhone(invitee invitee) string {
	if strings.TrimSpace(invitee.TextReminderNumber) != "" {
		return strings.TrimSpace(invitee.TextReminderNumber)
	}
	for _, qa := range invitee.Questions {
		question := strings.ToLower(qa.Question)
		if strings.Contains(question, "phone") || strings.Contains(question, "telefono") || strings.Contains(question, "tel") || strings.Contains(question, "cel") {
			return strings.TrimSpace(qa.Answer)
		}
	}
	return ""
}
