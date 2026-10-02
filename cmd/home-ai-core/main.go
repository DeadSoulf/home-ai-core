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

	"github.com/DeadSoulf/home-ai-core/internal/aiagent"
	"github.com/DeadSoulf/home-ai-core/internal/api"
	"github.com/DeadSoulf/home-ai-core/internal/cloudai"
	"github.com/DeadSoulf/home-ai-core/internal/config"
	"github.com/DeadSoulf/home-ai-core/internal/events"
	"github.com/DeadSoulf/home-ai-core/internal/identity"
	"github.com/DeadSoulf/home-ai-core/internal/jobs"
	"github.com/DeadSoulf/home-ai-core/internal/modules"
	"github.com/DeadSoulf/home-ai-core/internal/realtime"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/updater"
	"github.com/DeadSoulf/home-ai-core/internal/version"
	"github.com/DeadSoulf/home-ai-core/internal/webui"
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

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer startupCancel()

	store, err := state.Open(startupCtx, cfg.StateDir)
	if err != nil {
		logger.Error("failed to initialize core state", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	hostname, err := os.Hostname()
	if err != nil {
		logger.Error("failed to read hostname", "error", err)
		os.Exit(1)
	}
	if err := store.EnsureNode(startupCtx, nodeID, hostname); err != nil {
		logger.Error("failed to register local node", "error", err)
		os.Exit(1)
	}

	securityService, err := security.New(startupCtx, store, cfg.StateDir)
	if err != nil {
		logger.Error("failed to initialize security", "error", err)
		os.Exit(1)
	}

	realtimeHub := realtime.New(nodeID, logger)
	eventService := events.New(nodeID, store, realtimeHub)
	jobService := jobs.New(nodeID, store, eventService, 2)
	moduleRegistry := modules.NewRegistry(store)
	if err := moduleRegistry.Register(startupCtx, aiagent.NewModule()); err != nil {
		logger.Error("failed to register AI Agent module", "error", err)
		os.Exit(1)
	}
	if err := moduleRegistry.Register(startupCtx, cloudai.NewModule()); err != nil {
		logger.Error("failed to register Cloud AI module", "error", err)
		os.Exit(1)
	}

	var localProvider aiagent.Provider
	if cfg.AIProvider == "ollama" {
		provider, err := aiagent.NewOllamaProvider(cfg.AIEndpoint, cfg.AIModel)
		if err != nil {
			logger.Error("failed to initialize local AI provider", "error", err)
			os.Exit(1)
		}
		localProvider = provider
	}

	var cloudProvider aiagent.Provider
	if cfg.CloudAIEndpoint != "" && cfg.CloudAIModel != "" && cfg.CloudAIAPIKey != "" {
		provider, err := aiagent.NewOpenAICompatibleProvider(cfg.CloudAIEndpoint, cfg.CloudAIModel, cfg.CloudAIAPIKey)
		if err != nil {
			logger.Error("failed to initialize cloud AI provider", "error", err)
			os.Exit(1)
		}
		cloudProvider = provider
	}

	var aiProvider aiagent.Provider
	if localProvider != nil || cloudProvider != nil {
		router := aiagent.NewRoutingProvider(localProvider, cloudProvider)
		if item, err := moduleRegistry.Get(startupCtx, "ai.cloud"); err == nil {
			switch item.Status {
			case "enabled":
				if cloudProvider != nil {
					router.SetCloudEnabled(true)
				} else {
					_ = moduleRegistry.SetStatus(startupCtx, "ai.cloud", "error", "Cloud AI provider is not configured")
				}
			case "registered":
				_ = moduleRegistry.SetStatus(startupCtx, "ai.cloud", "disabled", "")
			}
		}
		aiProvider = router
	} else if item, err := moduleRegistry.Get(startupCtx, "ai.cloud"); err == nil && item.Status == "registered" {
		_ = moduleRegistry.SetStatus(startupCtx, "ai.cloud", "disabled", "")
	}
	updaterService := updater.New(version.Version, cfg.StateDir)
	jobCtx, jobCancel := context.WithCancel(context.Background())
	defer jobCancel()
	go func() {
		if err := jobService.Run(jobCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("job engine stopped", "error", err)
		}
	}()

	apiHandler := api.New(
		nodeID,
		logger,
		store,
		securityService,
		jobService,
		eventService,
		moduleRegistry,
		updaterService,
		realtimeHub,
		aiProvider,
	)
	handler := webui.New(apiHandler, cfg.WebDir)

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// AI chat may legitimately run close to the 90s provider deadline,
		// especially on older GPUs or during a bounded tool loop.
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go api.ReconcileFileAccess(ctx, logger, store, securityService)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("home-ai-core starting",
			"listen", cfg.ListenAddress,
			"state_dir", cfg.StateDir,
			"web_dir", cfg.WebDir,
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
