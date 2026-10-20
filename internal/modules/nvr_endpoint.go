package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

// ResolveNVREndpoint asks the trusted, UID-checked updater helper for the
// private IPv4 address of the running NVR. Core never opens docker.sock.
func ResolveNVREndpoint(ctx context.Context) (string, error) {
	return resolveModuleEndpoint(ctx, "nvr.module")
}

// ResolveAIEndpoint resolves only the managed AI agent on the private module
// network. Never accept user-supplied IDs, hosts or network endpoints here.
func ResolveAIEndpoint(ctx context.Context) (string, error) {
	return resolveModuleEndpoint(ctx, "ai.agent")
}

// ResolveFrigateEndpoint resolves the fixed official Frigate container only.
func ResolveFrigateEndpoint(ctx context.Context) (string, error) {
	return resolveModuleEndpoint(ctx, "frigate.module")
}

func resolveModuleEndpoint(ctx context.Context, moduleID string) (string, error) {
	if moduleID != "nvr.module" && moduleID != "ai.agent" && moduleID != "frigate.module" {
		return "", errors.New("unsupported module endpoint")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "unix", moduleHelperSocketPath)
	if err != nil {
		return "", fmt.Errorf("connect module helper: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(8 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "docker.module.endpoint",
		ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID:        moduleID,
	}); err != nil {
		return "", err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return "", fmt.Errorf("read module endpoint: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "module endpoint is unavailable"
		}
		return "", errors.New(response.Error)
	}
	address, err := netip.ParseAddr(response.ModuleAddress)
	if err != nil || !address.Is4() || !address.IsPrivate() || address.IsLoopback() {
		return "", errors.New("helper returned an invalid managed container address")
	}
	return address.String(), nil
}
