package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/helperipc"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

// RuntimeStatus reports evidence from a read-only helper inspection, not the
// host.docker capability (which only indicates that a socket exists).
type RuntimeStatus struct {
	Available    bool      `json:"available"`
	Active       bool      `json:"active"`
	EngineReady  bool      `json:"engine_ready"`
	NetworkReady bool      `json:"network_ready"`
	Version      string    `json:"version"`
	Error        string    `json:"error,omitempty"`
	CheckedAt    time.Time `json:"checked_at"`
}

func (r *Registry) Runtime(ctx context.Context) (RuntimeStatus, error) {
	conn, cleanup, err := helperipc.Dial(ctx, moduleHelperSocketPath, 25*time.Second)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("inspect Docker runtime: %w", err)
	}
	defer cleanup()
	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{Operation: "docker.inspect", ProtocolVersion: updaterhelper.ProtocolVersion}); err != nil {
		return RuntimeStatus{}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return RuntimeStatus{}, err
	}
	return runtimeStatusFromResponse(response)
}

func runtimeStatusFromResponse(response updaterhelper.Response) (RuntimeStatus, error) {
	if !response.OK {
		return RuntimeStatus{}, errors.New("privileged helper rejected Docker inspection")
	}
	if response.ProtocolVersion != updaterhelper.ProtocolVersion {
		return RuntimeStatus{}, errors.New("incompatible Docker helper protocol")
	}
	// A Docker server version is populated only after a successful engine query.
	engine := response.DockerAvailable && response.DockerActive && response.DockerVersion != ""
	return RuntimeStatus{Available: response.DockerAvailable, Active: response.DockerActive,
		EngineReady: engine, NetworkReady: engine && response.DockerNetworkReady,
		Version: response.DockerVersion, Error: response.DockerError, CheckedAt: time.Now().UTC()}, nil
}
