package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"verificador-citas-eros/internal/appmodel"
	"verificador-citas-eros/internal/config"
)

type FileStore struct {
	configPath  string
	historyPath string
	mu          sync.Mutex
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("crear directorio de datos: %w", err)
	}
	return &FileStore{
		configPath:  filepath.Join(dir, "config.json"),
		historyPath: filepath.Join(dir, "history.json"),
	}, nil
}

func (s *FileStore) LoadConfig() (config.SchedulerConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := config.DefaultSchedulerConfig()
	if err := readJSONFile(s.configPath, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return config.SchedulerConfig{}, err
	}
	return cfg.Normalized(), nil
}

func (s *FileStore) SaveConfig(cfg config.SchedulerConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.configPath, cfg.Normalized())
}

func (s *FileStore) LoadHistory() ([]appmodel.ValidationRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var history []appmodel.ValidationRun
	if err := readJSONFile(s.historyPath, &history); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []appmodel.ValidationRun{}, nil
		}
		return nil, err
	}
	return history, nil
}

func (s *FileStore) SaveHistory(history []appmodel.ValidationRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(s.historyPath, history)
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("leer %s: %w", path, err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar %s: %w", path, err)
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("renombrar %s: %w", path, err)
	}
	return nil
}
