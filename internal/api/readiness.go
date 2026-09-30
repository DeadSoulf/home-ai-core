package api

import (
	"net/http"
	"time"

	homenetwork "github.com/DeadSoulf/home-ai-core/internal/network"
	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

type readinessResponse struct {
	CheckedAt time.Time              `json:"checked_at"`
	Network   *networkReadiness      `json:"network,omitempty"`
	WireGuard *wireGuardReadiness    `json:"wireguard,omitempty"`
	Updater   *updaterReadiness      `json:"updater,omitempty"`
}

type networkReadiness struct {
	Backend    string                 `json:"backend"`
	Profiles   any                    `json:"profiles"`
	Interfaces []systeminfo.NetworkInterface `json:"interfaces"`
	Error      string                 `json:"error,omitempty"`
}

type wireGuardReadiness struct {
	Available bool `json:"available"`
	Tunnels   any  `json:"tunnels"`
	Error     string `json:"error,omitempty"`
}

type updaterReadiness struct {
	CurrentVersion    string `json:"current_version"`
	Architecture      string `json:"architecture"`
	HelperAvailable   bool   `json:"helper_available"`
	HelperCompatible  bool   `json:"helper_compatible"`
	HelperVersion     string `json:"helper_version,omitempty"`
	HelperProtocol    int    `json:"helper_protocol,omitempty"`
	HelperError       string `json:"helper_error,omitempty"`
	RollbackAvailable bool   `json:"rollback_available"`
	RollbackVersion   string `json:"rollback_version,omitempty"`
}

func (s *server) systemReadiness(
	w http.ResponseWriter,
	r *http.Request,
	actor security.Actor,
	_ authSource,
) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}

	response := readinessResponse{CheckedAt: time.Now().UTC()}

	if actor.Has("network.read") {
		info := systeminfo.Collect(s.nodeID)
		network := &networkReadiness{
			Interfaces: info.NetworkInterfaces,
			Profiles:   []any{},
		}
		networkCtx, cancel := contextWithTimeout(r.Context(), 8*time.Second)
		profiles, err := homenetwork.InspectNetworkProfiles(networkCtx)
		cancel()
		if err != nil {
			network.Error = err.Error()
		} else {
			network.Backend = profiles.Backend
			network.Profiles = profiles.Profiles
		}
		response.Network = network

		wireGuard := &wireGuardReadiness{Tunnels: []any{}}
		wireGuardCtx, wireGuardCancel := contextWithTimeout(r.Context(), 8*time.Second)
		status, err := homenetwork.InspectWireGuard(wireGuardCtx)
		wireGuardCancel()
		if err != nil {
			wireGuard.Error = err.Error()
		} else {
			wireGuard.Available = status.Available
			wireGuard.Tunnels = status.Tunnels
			wireGuard.Error = status.Error
		}
		response.WireGuard = wireGuard
	}

	if actor.Has("updates.read") && s.updater != nil {
		updateCtx, cancel := contextWithTimeout(r.Context(), 5*time.Second)
		status := s.updater.LocalStatus(updateCtx)
		cancel()
		response.Updater = &updaterReadiness{
			CurrentVersion:    status.CurrentVersion,
			Architecture:      status.Architecture,
			HelperAvailable:   status.HelperAvailable,
			HelperCompatible:  status.HelperCompatible,
			HelperVersion:     status.HelperVersion,
			HelperProtocol:    status.HelperProtocol,
			HelperError:       status.HelperError,
			RollbackAvailable: status.RollbackAvailable,
			RollbackVersion:   status.RollbackVersion,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"readiness": response})
}
