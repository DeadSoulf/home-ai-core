package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const helperSocketPath = "/run/home-ai-core-updater.sock"

type Request struct {
	Operation     string
	Interface     string
	Address       string
	Gateway       string
	MTU           int
	Tunnel        string
	ListenPort    int
	PrivateKey    string
	PeerPublicKey string
	PresharedKey  string
	AllowedIPs    []string
	Endpoint      string
	Keepalive     int
	NetworkMethod string
	DNS           []string
}

type NetworkProfileStatus struct {
	Backend  string                             `json:"backend"`
	Profiles []updaterhelper.NetworkProfileStat `json:"profiles"`
}

type WireGuardStatus struct {
	Available bool                                `json:"available"`
	Error     string                              `json:"error,omitempty"`
	Tunnels   []updaterhelper.WireGuardTunnelStat `json:"tunnels"`
}

func Execute(ctx context.Context, input Request) (string, error) {
	operation := strings.TrimSpace(input.Operation)
	switch operation {
	case "link.up", "link.down", "mtu", "address.add", "address.delete", "gateway.set", "gateway.delete",
		"profile.save",
		"wireguard.install", "wireguard.create", "wireguard.up", "wireguard.down", "wireguard.delete",
		"wireguard.peer.add", "wireguard.peer.delete":
	default:
		return "", errors.New("unsupported network operation")
	}
	if err := ensureCompatibleHelper(ctx); err != nil {
		return "", err
	}

	request := updaterhelper.Request{
		ProtocolVersion: updaterhelper.ProtocolVersion,
		Interface:       strings.TrimSpace(input.Interface),
		Address:         strings.TrimSpace(input.Address),
		Gateway:         strings.TrimSpace(input.Gateway),
		MTU:             input.MTU,
		Tunnel:          strings.TrimSpace(input.Tunnel),
		ListenPort:      input.ListenPort,
		PrivateKey:      strings.TrimSpace(input.PrivateKey),
		PeerPublicKey:   strings.TrimSpace(input.PeerPublicKey),
		PresharedKey:    strings.TrimSpace(input.PresharedKey),
		AllowedIPs:      trimStrings(input.AllowedIPs),
		Endpoint:        strings.TrimSpace(input.Endpoint),
		Keepalive:       input.Keepalive,
		NetworkMethod:   strings.TrimSpace(input.NetworkMethod),
		DNS:             trimStrings(input.DNS),
	}
	if strings.HasPrefix(operation, "wireguard.") {
		request.Operation = operation
	} else {
		request.Operation = "network." + operation
	}
	response, err := callHelper(ctx, request)
	if err != nil {
		return "", err
	}
	return response.Message, nil
}

func InspectNetworkProfiles(ctx context.Context) (NetworkProfileStatus, error) {
	if err := ensureCompatibleHelper(ctx); err != nil {
		return NetworkProfileStatus{}, err
	}
	response, err := callHelper(ctx, updaterhelper.Request{
		Operation:       "network.profile.inspect",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	})
	if err != nil {
		return NetworkProfileStatus{}, err
	}
	return NetworkProfileStatus{
		Backend:  response.NetworkBackend,
		Profiles: append([]updaterhelper.NetworkProfileStat(nil), response.NetworkProfiles...),
	}, nil
}

func InspectWireGuard(ctx context.Context) (WireGuardStatus, error) {
	if err := ensureCompatibleHelper(ctx); err != nil {
		return WireGuardStatus{}, err
	}
	response, err := callHelper(ctx, updaterhelper.Request{
		Operation:       "wireguard.inspect",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	})
	if err != nil {
		return WireGuardStatus{}, err
	}
	return WireGuardStatus{
		Available: response.WireGuardAvailable,
		Error:     response.WireGuardError,
		Tunnels:   append([]updaterhelper.WireGuardTunnelStat(nil), response.WireGuardTunnels...),
	}, nil
}

func ensureCompatibleHelper(ctx context.Context) error {
	response, err := callHelper(ctx, updaterhelper.Request{Operation: "info"})
	if err != nil {
		return err
	}
	if !response.OK || response.ProtocolVersion < updaterhelper.ProtocolVersion {
		return errors.New("system network helper is outdated; update Home-AI-Core first")
	}
	return nil
}

func callHelper(ctx context.Context, request updaterhelper.Request) (updaterhelper.Response, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return updaterhelper.Response{}, fmt.Errorf("connect network helper: %w", err)
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return updaterhelper.Response{}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return updaterhelper.Response{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "network helper rejected operation"
		}
		return updaterhelper.Response{}, errors.New(response.Error)
	}
	return response, nil
}

func trimStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value := strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
