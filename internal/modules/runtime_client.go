package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const moduleHelperSocketPath = "/run/home-ai-core-updater.sock"

type helperModuleResult struct {
	State       string
	ContainerID string
	Message     string
}

func callModuleHelper(
	ctx context.Context,
	operation, moduleID, image string,
	health HealthSpec,
	registryUser, registryToken string,
) (helperModuleResult, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil {
		return helperModuleResult{}, fmt.Errorf("connect privileged module helper: %w", err)
	}
	defer conn.Close()

	request := updaterhelper.Request{
		Operation:       operation,
		ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID:         moduleID,
		Image:            image,
		ModuleHealthPort: health.Port,
		ModuleHealthPath: health.Path,
		RegistryUsername: registryUser,
		RegistryToken:    registryToken,
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return helperModuleResult{}, err
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return helperModuleResult{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "privileged module helper rejected operation"
		}
		return helperModuleResult{}, errors.New(response.Error)
	}
	return helperModuleResult{
		State:       response.ModuleState,
		ContainerID: response.ContainerID,
		Message:     response.Message,
	}, nil
}
