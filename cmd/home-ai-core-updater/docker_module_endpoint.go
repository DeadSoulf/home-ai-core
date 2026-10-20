package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

// inspectManagedModuleAddress is read-only and strictly restricted to the
// native Core integrations. It never accepts a caller-chosen network target.
// Docker labels and the managed network ID are verified before exposing an IP.
func inspectManagedModuleAddress(ctx context.Context, moduleID string) (string, error) {
	if moduleID != "nvr.module" && moduleID != "ai.agent" && moduleID != "frigate.module" {
		return "", errors.New("unsupported module endpoint")
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return "", errors.New("Docker is unavailable")
	}
	name := "home-ai-module-" + moduleID
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("module container is unavailable: %s", commandError(output, err))
	}
	var container struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
				NetworkID string `json:"NetworkID"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(output, &container); err != nil {
		return "", fmt.Errorf("decode NVR container metadata: %w", err)
	}
	if !container.State.Running {
		return "", errors.New("module container is not running")
	}
	if container.Config.Labels["home-ai.managed"] != "true" ||
		container.Config.Labels["home-ai.module.id"] != moduleID {
		return "", errors.New("refusing to connect to an unmanaged module container")
	}
	networkName := dockerNetworkName
	if moduleID == "frigate.module" {
		networkName = frigateNetwork
		if container.Config.Labels["home-ai.frigate.profile"] != "1" {
			return "", errors.New("unrecognized Frigate runtime profile")
		}
	}
	network, ok := container.NetworkSettings.Networks[networkName]
	if !ok || network.NetworkID == "" {
		return "", errors.New("module is not connected to the managed module network")
	}
	output, err = exec.CommandContext(ctx, dockerPath, "network", "inspect", "--format",
		`{{index .Labels "home-ai.managed"}}|{{.Id}}`, networkName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect managed module network: %s", commandError(output, err))
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
	if len(parts) != 2 || parts[0] != "true" || parts[1] != network.NetworkID {
		return "", errors.New("module network ownership does not match")
	}
	address, err := netip.ParseAddr(strings.TrimSpace(network.IPAddress))
	if err != nil || !address.Is4() || !address.IsPrivate() || address.IsLoopback() {
		return "", errors.New("module has no valid private IPv4 container address")
	}
	return address.String(), nil
}

// Backward-compatible fixed NVR endpoint for existing callers and tests.
func inspectNVRModuleAddress(ctx context.Context) (string, error) {
	return inspectManagedModuleAddress(ctx, "nvr.module")
}
