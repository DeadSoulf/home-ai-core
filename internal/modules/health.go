package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const moduleHealthTimeout = 60 * time.Second

func validateModuleHTTPHealth(port int, path string) error {
	if port < 1 || port > 65535 {
		return errors.New("module health port is invalid")
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		len(path) > 200 || strings.Contains(path, "..") ||
		strings.ContainsAny(path, "?#\\\r\n\t") {
		return errors.New("module health path is unsafe")
	}
	return nil
}

// waitForModuleHTTPHealth is run by the unprivileged Core after the helper
// creates a replacement. Helper cannot open IP sockets in its sandbox.
func waitForModuleHTTPHealth(ctx context.Context, ip string, health HealthSpec) error {
	if err := validateModuleHTTPHealth(health.Port, health.Path); err != nil {
		return err
	}
	parsed := net.ParseIP(ip)
	if parsed == nil || !parsed.IsPrivate() {
		return errors.New("helper did not provide a private managed module IP")
	}
	healthCtx, cancel := context.WithTimeout(ctx, moduleHealthTimeout)
	defer cancel()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	url := "http://" + net.JoinHostPort(ip, strconv.Itoa(health.Port)) + health.Path
	var lastError error
	for {
		if err := healthCtx.Err(); err != nil {
			return fmt.Errorf("module health check did not pass: %v: %w", lastError, err)
		}
		request, err := http.NewRequestWithContext(healthCtx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 300 {
				return nil
			}
			lastError = fmt.Errorf("HTTP %d", response.StatusCode)
		} else {
			lastError = err
		}
		select {
		case <-healthCtx.Done():
			return fmt.Errorf("module health check did not pass: %v: %w", lastError, healthCtx.Err())
		case <-time.After(750 * time.Millisecond):
		}
	}
}
