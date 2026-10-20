package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/api"
	"github.com/DeadSoulf/home-ai-core/internal/config"
)

// The original Frigate web application is served on a separate origin by Core,
// not by publishing the container's unauthenticated administrative port. Reuse
// Core's certificate and authorization, without forwarding its credentials.
func startFrigateUIServer(ctx context.Context, cfg config.Config, handler *api.Handler, tlsConfig *tls.Config, logger *slog.Logger) *http.Server {
	address := cfg.FrigateUIListen
	if address == "off" {
		address = ""
	}
	gateway := handler.ConfigureFrigateUI(api.FrigateUIConfig{ListenAddress: address, CoreListenAddress: cfg.ListenAddress, InsecureLoopback: cfg.InsecureLoopback, Context: ctx})
	if address == "" {
		handler.SetFrigateUIReady(false, "Full Frigate UI listener is disabled")
		return nil
	}
	if !cfg.InsecureLoopback && tlsConfig == nil {
		handler.SetFrigateUIReady(false, "TLS is required for the Frigate UI")
		return nil
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		handler.SetFrigateUIReady(false, "Frigate UI port is unavailable. Check HOME_AI_FRIGATE_UI_LISTEN and port conflicts.")
		logger.Error("Frigate UI listener unavailable; Core remains running", "listen", address, "error", err)
		return nil
	}
	server := &http.Server{Addr: address, Handler: gateway, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig.Clone()
	}
	handler.SetFrigateUIReady(true, "")
	go func() {
		logger.Info("official Frigate UI gateway listening; Core authentication required", "listen", address)
		var err error
		if cfg.InsecureLoopback {
			err = server.Serve(listener)
		} else {
			err = server.ServeTLS(listener, "", "")
		}
		handler.SetFrigateUIReady(false, "Frigate UI listener is not running")
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Frigate UI listener stopped", "error", err)
		}
	}()
	return server
}
