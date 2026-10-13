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

func callModuleHelper(ctx context.Context, operation, moduleID, image string) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{
		Operation: operation, ModuleID: moduleID, Image: image,
	})
}

func callModuleUpgradeHelper(ctx context.Context, moduleID, image, previousImage string, health HealthSpec) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{
		Operation: "docker.module.upgrade",
		ModuleID: moduleID,
		Image: image,
		PreviousImage: previousImage,
		ModuleHealthPort: health.Port,
		ModuleHealthPath: health.Path,
	})
}

func callModuleHelperRequest(ctx context.Context, request updaterhelper.Request) (helperModuleResult, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil {
		return helperModuleResult{}, fmt.Errorf("connect privileged module helper: %w", err)
	}
	defer conn.Close()

	request.ProtocolVersion = updaterhelper.ProtocolVersion
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
