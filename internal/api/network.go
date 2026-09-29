package api

import (
	"net/http"
	"strings"

	homenetwork "github.com/DeadSoulf/home-ai-core/internal/network"
	"github.com/DeadSoulf/home-ai-core/internal/security"
)

func (s *server) wireGuardStatus(
	w http.ResponseWriter,
	r *http.Request,
	_ security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	status, err := homenetwork.InspectWireGuard(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "network_inspection_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wireguard": status})
}

func (s *server) networkOperation(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	source authSource,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if source == authCookie && !actor.ValidCSRF(r.Header.Get("X-CSRF-Token")) {
		writeAPIError(w, r, http.StatusForbidden, "csrf_required", "valid CSRF token required", nil)
		return
	}

	var input struct {
		Operation     string   `json:"operation"`
		Interface     string   `json:"interface,omitempty"`
		Address       string   `json:"address,omitempty"`
		Gateway       string   `json:"gateway,omitempty"`
		MTU           int      `json:"mtu,omitempty"`
		Tunnel        string   `json:"tunnel,omitempty"`
		ListenPort    int      `json:"listen_port,omitempty"`
		PrivateKey    string   `json:"private_key,omitempty"`
		PeerPublicKey string   `json:"peer_public_key,omitempty"`
		PresharedKey  string   `json:"preshared_key,omitempty"`
		AllowedIPs    []string `json:"allowed_ips,omitempty"`
		Endpoint      string   `json:"endpoint,omitempty"`
		Keepalive     int      `json:"keepalive,omitempty"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_network_request", err.Error(), nil)
		return
	}
	input.Operation = strings.TrimSpace(input.Operation)
	if input.Operation == "" {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_network_request", "network operation is required", nil)
		return
	}

	message, err := homenetwork.Execute(r.Context(), homenetwork.Request{
		Operation:     input.Operation,
		Interface:     input.Interface,
		Address:       input.Address,
		Gateway:       input.Gateway,
		MTU:           input.MTU,
		Tunnel:        input.Tunnel,
		ListenPort:    input.ListenPort,
		PrivateKey:    input.PrivateKey,
		PeerPublicKey: input.PeerPublicKey,
		PresharedKey:  input.PresharedKey,
		AllowedIPs:    input.AllowedIPs,
		Endpoint:      input.Endpoint,
		Keepalive:     input.Keepalive,
	})
	if err != nil {
		writeAPIError(w, r, http.StatusBadGateway, "network_operation_failed", err.Error(), nil)
		return
	}

	targetType := "network_interface"
	targetID := strings.TrimSpace(input.Interface)
	if strings.HasPrefix(input.Operation, "wireguard.") {
		targetType = "wireguard_tunnel"
		targetID = strings.TrimSpace(input.Tunnel)
	}
	s.security.RecordAudit(
		r.Context(),
		s.securityRequestContext(r),
		actor,
		"network."+input.Operation,
		targetType,
		targetID,
		"success",
		map[string]any{
			"address": input.Address,
			"gateway": input.Gateway,
			"mtu":     input.MTU,
		},
	)
	s.realtime.Publish(
		"system.network.changed",
		map[string]any{"operation": input.Operation, "target": targetID},
		requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusOK, map[string]any{"message": message})
}
