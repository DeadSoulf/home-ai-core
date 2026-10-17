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

// inspectNVRModuleAddress is deliberately read-only. The updater helper is the
// only process permitted to inspect Docker; Core never receives docker.sock.
// Only a running, HOME AI-managed NVR on the HOME AI-managed Docker network
// can return an address, and the API will use it solely on port 8090.
func inspectNVRModuleAddress(ctx context.Context) (string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return "", errors.New("Docker is unavailable")
	}
	const name = "home-ai-module-nvr.module"
	output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("NVR container is unavailable: %s", commandError(output, err))
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
		return "", errors.New("NVR container is not running")
	}
	if container.Config.Labels["home-ai.managed"] != "true" ||
		container.Config.Labels["home-ai.module.id"] != "nvr.module" {
		return "", errors.New("refusing to connect to an unmanaged NVR container")
	}
	network, ok := container.NetworkSettings.Networks[dockerNetworkName]
	if !ok || network.NetworkID == "" {
		return "", errors.New("NVR is not connected to the managed module network")
	}
	output, err = exec.CommandContext(ctx, dockerPath, "network", "inspect", "--format",
		`{{index .Labels "home-ai.managed"}}|{{.Id}}`, dockerNetworkName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect managed module network: %s", commandError(output, err))
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
	if len(parts) != 2 || parts[0] != "true" || parts[1] != network.NetworkID {
		return "", errors.New("NVR Docker network ownership does not match")
	}
	address, err := netip.ParseAddr(strings.TrimSpace(network.IPAddress))
	if err != nil || !address.Is4() || !address.IsPrivate() || address.IsLoopback() {
		return "", errors.New("NVR has no valid private IPv4 container address")
	}
	return address.String(), nil
}
