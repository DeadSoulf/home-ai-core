package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/api"
	"github.com/DeadSoulf/home-ai-core/internal/config"
	"github.com/DeadSoulf/home-ai-core/internal/identity"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if os.Geteuid() == 0 {
		logger.Error("refusing to run the network-facing core as root")
		os.Exit(1)
	}

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	nodeID, err := identity.LoadOrCreate(cfg.StateDir)
	if err != nil {
		logger.Error("failed to initialize node identity", "error", err)
		os.Exit(1)
	}

	handler := api.New(nodeID, logger)

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("home-ai-core starting",
			"listen", cfg.ListenAddress,
			"state_dir", cfg.StateDir,
			"node_id", nodeID,
		)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
	case err := <-errCh:
		if err != nil {
			logger.Error("http server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("home-ai-core stopped")
}
