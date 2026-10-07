package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	moduleID      = "reference.module"
	moduleVersion = "0.1.0"
	statePath     = "/data/state.json"
)

type persistedState struct {
	BootCount int       `json:"boot_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type service struct {
	mu      sync.RWMutex
	state   persistedState
	started time.Time
}

func main() {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	state, err := loadState(statePath)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}
	now := time.Now().UTC()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	state.BootCount++
	state.UpdatedAt = now
	if err := saveState(statePath, state); err != nil {
		log.Fatalf("save state: %v", err)
	}

	s := &service{state: state, started: now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /state", s.stateHandler)
	mux.HandleFunc("GET /", s.index)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("%s %s started; boot_count=%d", moduleID, moduleVersion, state.BootCount)
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server: %v", err)
	}
}

func (s *service) health(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"module_id":  moduleID,
		"version":    moduleVersion,
		"boot_count": state.BootCount,
		"started_at": s.started,
	})
}

func (s *service) stateHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	state := s.state
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, state)
}

func (s *service) index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"module_id": moduleID,
		"version":   moduleVersion,
		"health":    "/health",
		"state":     "/state",
	})
}

func loadState(path string) (persistedState, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return persistedState{}, nil
	}
	if err != nil {
		return persistedState{}, err
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return persistedState{}, fmt.Errorf("decode state: %w", err)
	}
	return state, nil
}

func saveState(path string, state persistedState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
