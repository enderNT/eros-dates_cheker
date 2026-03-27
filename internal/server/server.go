package server

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"verificador-citas-eros/internal/config"
	"verificador-citas-eros/internal/service"
)

const ShutdownTimeout = 10 * time.Second

//go:embed ui/*
var embeddedUI embed.FS

type Server struct {
	service *service.Service
	mux     *http.ServeMux
}

func New(service *service.Service) http.Handler {
	server := &Server{
		service: service,
		mux:     http.NewServeMux(),
	}
	server.routes()
	return server.mux
}

func (s *Server) routes() {
	uiFS, _ := fs.Sub(embeddedUI, "ui")
	fileServer := http.FileServer(http.FS(uiFS))

	s.mux.Handle("/assets/", http.StripPrefix("/assets/", fileServer))
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/validate/run", s.handleRunValidation)
	s.mux.HandleFunc("/api/validate/status", s.handleStatus)
	s.mux.HandleFunc("/api/validate/history", s.handleHistory)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, embeddedUI, "ui/index.html")
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.service.GetConfig())
	case http.MethodPut:
		var cfg config.SchedulerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		updated, err := s.service.UpdateConfig(cfg)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRunValidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	run, err := s.service.RunValidation(r.Context(), "manual")
	if err != nil {
		if run != nil {
			writeJSON(w, http.StatusOK, run)
			return
		}
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.service.GetStatus(time.Now().UTC()))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.service.GetHistory())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		err = errors.New("error desconocido")
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
