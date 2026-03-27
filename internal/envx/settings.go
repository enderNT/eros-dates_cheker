package envx

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Settings struct {
	ServerAddr string
	DataDir    string
	Calendly   CalendlySettings
}

type CalendlySettings struct {
	BaseURL      string
	Token        string
	Organization string
	EventTypeURI string
	PageSize     int
}

func LoadSettings() (Settings, error) {
	pageSize := 20
	if raw := strings.TrimSpace(os.Getenv("CALENDLY_VALIDATION_PAGE_SIZE")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return Settings{}, fmt.Errorf("CALENDLY_VALIDATION_PAGE_SIZE invalido: %q", raw)
		}
		pageSize = parsed
	}

	settings := Settings{
		ServerAddr: resolveServerAddr(),
		DataDir:    valueOrDefault("APP_DATA_DIR", "data"),
		Calendly: CalendlySettings{
			BaseURL:      valueOrDefault("CALENDLY_API_BASE_URL", "https://api.calendly.com"),
			Token:        strings.TrimSpace(os.Getenv("CALENDLY_API_TOKEN")),
			Organization: strings.TrimSpace(os.Getenv("CALENDLY_ORGANIZATION_URI")),
			EventTypeURI: strings.TrimSpace(os.Getenv("CALENDLY_EVENT_TYPE_URI")),
			PageSize:     pageSize,
		},
	}

	if settings.Calendly.Token == "" {
		return Settings{}, errors.New("CALENDLY_API_TOKEN es requerido")
	}
	return settings, nil
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func resolveServerAddr() string {
	if value := strings.TrimSpace(os.Getenv("SERVER_ADDR")); value != "" {
		return value
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		if strings.HasPrefix(port, ":") {
			return port
		}
		return ":" + port
	}
	return ":8080"
}
