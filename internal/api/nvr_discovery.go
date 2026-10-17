package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/nvrdiscovery"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

// Discovery runs only on the Core host's directly connected private networks.
// The NVR Docker container is intentionally not given host networking.
func (s *server) nvrDiscoveryNetworks(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	networks, err := s.availableNVRDiscoveryNetworks()
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_discovery_networks_unavailable",
			"Unable to inspect local networks", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"networks": networks})
}

func (s *server) availableNVRDiscoveryNetworks() ([]nvrdiscovery.Network, error) {
	if s.nvrNetworks != nil {
		return s.nvrNetworks()
	}
	return nvrdiscovery.Networks()
}

func (s *server) nvrDiscover(w http.ResponseWriter, r *http.Request, actor security.Actor, source authSource) {
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}
	if !s.nvrReady(w, r) {
		return
	}
	var input struct {
		NetworkID string `json:"network_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_discovery_network",
			"Select a network from the local network list", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_discovery_network",
			"Discovery request must contain one JSON object", nil)
		return
	}
	networks, err := s.availableNVRDiscoveryNetworks()
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_discovery_networks_unavailable",
			"Unable to inspect local networks", nil)
		return
	}
	scope, err := nvrdiscovery.SelectNetwork(networks, input.NetworkID)
	if err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_discovery_network",
			"Discovery is restricted to a directly connected private subnet", nil)
		return
	}
	if !s.nvrScanMu.TryLock() {
		writeAPIError(w, r, http.StatusConflict, "nvr_discovery_busy",
			"Another camera discovery scan is already running", nil)
		return
	}
	defer s.nvrScanMu.Unlock()
	if since := time.Since(s.nvrLastScan); since >= 0 && since < 10*time.Second {
		w.Header().Set("Retry-After", "10")
		writeAPIError(w, r, http.StatusTooManyRequests, "nvr_discovery_rate_limited",
			"Wait before starting another discovery scan", nil)
		return
	}
	s.nvrLastScan = time.Now()
	scan := s.nvrScanner
	if scan == nil {
		scan = nvrdiscovery.Scan
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	started := time.Now()
	found, err := scan(ctx, scope)
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "nvr_discovery_failed",
			"Camera discovery failed", nil)
		return
	}
	if found == nil {
		found = []nvrdiscovery.Device{}
	}
	s.security.RecordAudit(r.Context(), s.securityRequestContext(r), actor,
		"nvr.discovery.scan", "module", "nvr.module", "success",
		map[string]any{"network": scope.CIDR, "devices": len(found)})
	writeJSON(w, http.StatusOK, map[string]any{
		"devices": found,
		"network": scope,
		"scanned_hosts": scope.Hosts,
		"duration_ms": time.Since(started).Milliseconds(),
	})
}
