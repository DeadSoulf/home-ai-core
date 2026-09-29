package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const wireGuardDir = "/etc/wireguard"

var packageInstallMu sync.Mutex

func performNetworkOperation(ctx context.Context, request updaterhelper.Request) (string, error) {
	switch request.Operation {
	case "network.link.up", "network.link.down":
		iface, err := requireNetworkInterface(request.Interface)
		if err != nil {
			return "", err
		}
		state := strings.TrimPrefix(request.Operation, "network.link.")
		if err := runNetworkCommand(ctx, "ip", "link", "set", "dev", iface, state); err != nil {
			return "", err
		}
		return fmt.Sprintf("interface %s is %s", iface, state), nil

	case "network.mtu":
		iface, err := requireNetworkInterface(request.Interface)
		if err != nil {
			return "", err
		}
		if request.MTU < 576 || request.MTU > 65535 {
			return "", errors.New("MTU must be between 576 and 65535")
		}
		if err := runNetworkCommand(ctx, "ip", "link", "set", "dev", iface, "mtu", strconv.Itoa(request.MTU)); err != nil {
			return "", err
		}
		return fmt.Sprintf("MTU for %s set to %d", iface, request.MTU), nil

	case "network.address.add", "network.address.delete":
		iface, err := requireNetworkInterface(request.Interface)
		if err != nil {
			return "", err
		}
		address := strings.TrimSpace(request.Address)
		if _, _, err := net.ParseCIDR(address); err != nil {
			return "", errors.New("address must be a valid CIDR")
		}
		action := "add"
		if request.Operation == "network.address.delete" {
			action = "del"
		}
		if err := runNetworkCommand(ctx, "ip", "address", action, address, "dev", iface); err != nil {
			return "", err
		}
		return fmt.Sprintf("address %s %s on %s", address, action, iface), nil

	case "network.gateway.set", "network.gateway.delete":
		iface, err := requireNetworkInterface(request.Interface)
		if err != nil {
			return "", err
		}
		gateway := strings.TrimSpace(request.Gateway)
		args := []string{}
		if request.Operation == "network.gateway.set" {
			ip := net.ParseIP(gateway)
			if ip == nil {
				return "", errors.New("gateway must be a valid IP address")
			}
			if ip.To4() == nil {
				args = append(args, "-6")
			}
			args = append(args, "route", "replace", "default", "via", gateway, "dev", iface)
		} else {
			if gateway != "" {
				ip := net.ParseIP(gateway)
				if ip == nil {
					return "", errors.New("gateway must be a valid IP address")
				}
				if ip.To4() == nil {
					args = append(args, "-6")
				}
				args = append(args, "route", "del", "default", "via", gateway, "dev", iface)
			} else {
				args = append(args, "route", "del", "default", "dev", iface)
			}
		}
		if err := runNetworkCommand(ctx, "ip", args...); err != nil {
			return "", err
		}
		return fmt.Sprintf("default gateway updated for %s", iface), nil

	case "wireguard.install":
		return installWireGuardTools(ctx)
	case "wireguard.create":
		return createWireGuardTunnel(ctx, request)
	case "wireguard.up":
		return setWireGuardTunnelState(ctx, request.Tunnel, true)
	case "wireguard.down":
		return setWireGuardTunnelState(ctx, request.Tunnel, false)
	case "wireguard.delete":
		return deleteWireGuardTunnel(ctx, request.Tunnel)
	case "wireguard.peer.add":
		return addWireGuardPeer(ctx, request)
	case "wireguard.peer.delete":
		return deleteWireGuardPeer(ctx, request)
	default:
		return "", errors.New("unsupported network operation")
	}
}

func requireNetworkInterface(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validInterfaceName(name) {
		return "", errors.New("invalid network interface name")
	}
	if name == "lo" {
		return "", errors.New("loopback interface cannot be managed here")
	}
	if info, err := os.Stat(filepath.Join("/sys/class/net", name)); err != nil || !info.IsDir() {
		return "", fmt.Errorf("network interface %s not found", name)
	}
	return name, nil
}

func validInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func runNetworkCommand(ctx context.Context, command string, args ...string) error {
	_, err := runHostCommand(ctx, "", command, args...)
	return err
}

func runHostCommand(ctx context.Context, stdin, command string, args ...string) (string, error) {
	commandPath, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("%s is unavailable", command)
	}
	systemdRun, err := exec.LookPath("systemd-run")
	if err != nil {
		return "", errors.New("systemd-run is unavailable")
	}
	runArgs := transientHostCommand(commandPath, args...)
	cmd := exec.CommandContext(ctx, systemdRun, runArgs...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s failed: %s", command, message)
	}
	return strings.TrimSpace(string(output)), nil
}

func transientHostCommand(command string, args ...string) []string {
	result := []string{
		"--quiet",
		"--wait",
		"--pipe",
		"--collect",
		"--service-type=exec",
		"--",
		command,
	}
	return append(result, args...)
}

func wireGuardToolsAvailable() bool {
	_, wgErr := exec.LookPath("wg")
	_, quickErr := exec.LookPath("wg-quick")
	return wgErr == nil && quickErr == nil
}

func installWireGuardTools(ctx context.Context) (string, error) {
	if wireGuardToolsAvailable() {
		return "WireGuard tools are already installed", nil
	}
	if !packageInstallMu.TryLock() {
		return "", errors.New("package installation is already in progress")
	}
	defer packageInstallMu.Unlock()

	apt, err := exec.LookPath("apt-get")
	if err != nil {
		return "", errors.New("apt-get is unavailable")
	}
	systemdRun, err := exec.LookPath("systemd-run")
	if err != nil {
		return "", errors.New("systemd-run is unavailable")
	}

	for _, args := range [][]string{{"update"}, {"install", "-y", "wireguard-tools"}} {
		commandArgs := transientPackageCommand(apt, args...)
		cmd := exec.CommandContext(ctx, systemdRun, commandArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = err.Error()
			}
			return "", fmt.Errorf("install WireGuard tools: %s", message)
		}
	}
	if !wireGuardToolsAvailable() {
		return "", errors.New("wireguard-tools installation completed but wg/wg-quick are unavailable")
	}
	return "WireGuard tools installed", nil
}

func transientPackageCommand(command string, args ...string) []string {
	result := transientHostCommand(command, args...)
	insertAt := len(result) - len(args) - 1
	result = append(result[:insertAt],
		append([]string{"--setenv=DEBIAN_FRONTEND=noninteractive"}, result[insertAt:]...)...,
	)
	return result
}

func createWireGuardTunnel(ctx context.Context, request updaterhelper.Request) (string, error) {
	if !wireGuardToolsAvailable() {
		return "", errors.New("wireguard-tools are not installed")
	}
	name := strings.TrimSpace(request.Tunnel)
	if !validWireGuardName(name) {
		return "", errors.New("invalid WireGuard tunnel name")
	}
	address := strings.TrimSpace(request.Address)
	if _, _, err := net.ParseCIDR(address); err != nil {
		return "", errors.New("WireGuard address must be a valid CIDR")
	}
	port := request.ListenPort
	if port == 0 {
		port = 51820
	}
	if port < 1 || port > 65535 {
		return "", errors.New("WireGuard listen port must be between 1 and 65535")
	}

	if err := os.MkdirAll(wireGuardDir, 0o700); err != nil {
		return "", fmt.Errorf("create WireGuard directory: %w", err)
	}
	if err := os.Chmod(wireGuardDir, 0o700); err != nil {
		return "", fmt.Errorf("secure WireGuard directory: %w", err)
	}
	path := wireGuardConfigPath(name)
	if _, err := os.Stat(path); err == nil {
		return "", errors.New("WireGuard tunnel already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	privateKey := strings.TrimSpace(request.PrivateKey)
	if privateKey == "" {
		output, err := runHostCommand(ctx, "", "wg", "genkey")
		if err != nil {
			return "", fmt.Errorf("generate WireGuard private key: %w", err)
		}
		privateKey = strings.TrimSpace(output)
	}
	publicKey, err := wireGuardPublicKey(ctx, privateKey)
	if err != nil {
		return "", err
	}

	config := fmt.Sprintf(
		"[Interface]\nAddress = %s\nListenPort = %d\nPrivateKey = %s\n",
		address,
		port,
		privateKey,
	)
	if err := writeWireGuardConfig(path, config); err != nil {
		return "", err
	}
	if err := runNetworkCommand(ctx, "wg-quick", "up", name); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	_ = exec.CommandContext(ctx, "systemctl", "enable", "wg-quick@"+name+".service").Run()
	return fmt.Sprintf("WireGuard tunnel %s created; public key %s", name, publicKey), nil
}

func setWireGuardTunnelState(ctx context.Context, name string, up bool) (string, error) {
	if !wireGuardToolsAvailable() {
		return "", errors.New("wireguard-tools are not installed")
	}
	name = strings.TrimSpace(name)
	if !validWireGuardName(name) {
		return "", errors.New("invalid WireGuard tunnel name")
	}
	if _, err := os.Stat(wireGuardConfigPath(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("WireGuard tunnel does not exist")
		}
		return "", err
	}
	action := "down"
	if up {
		action = "up"
	}
	if err := runNetworkCommand(ctx, "wg-quick", action, name); err != nil {
		return "", err
	}
	if up {
		_ = exec.CommandContext(ctx, "systemctl", "enable", "wg-quick@"+name+".service").Run()
	} else {
		_ = exec.CommandContext(ctx, "systemctl", "disable", "wg-quick@"+name+".service").Run()
	}
	return fmt.Sprintf("WireGuard tunnel %s is %s", name, action), nil
}

func deleteWireGuardTunnel(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validWireGuardName(name) {
		return "", errors.New("invalid WireGuard tunnel name")
	}
	path := wireGuardConfigPath(name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("WireGuard tunnel does not exist")
		}
		return "", err
	}
	if wireGuardToolsAvailable() {
		if wireGuardActive(ctx, name) {
			if err := runNetworkCommand(ctx, "wg-quick", "down", name); err != nil {
				return "", err
			}
		}
	}
	_ = exec.CommandContext(ctx, "systemctl", "disable", "wg-quick@"+name+".service").Run()
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove WireGuard config: %w", err)
	}
	return fmt.Sprintf("WireGuard tunnel %s deleted", name), nil
}

func addWireGuardPeer(ctx context.Context, request updaterhelper.Request) (string, error) {
	if !wireGuardToolsAvailable() {
		return "", errors.New("wireguard-tools are not installed")
	}
	name := strings.TrimSpace(request.Tunnel)
	if !validWireGuardName(name) {
		return "", errors.New("invalid WireGuard tunnel name")
	}
	publicKey := strings.TrimSpace(request.PeerPublicKey)
	if err := validateWireGuardKey(publicKey); err != nil {
		return "", fmt.Errorf("invalid peer public key: %w", err)
	}
	if len(request.AllowedIPs) == 0 {
		return "", errors.New("at least one allowed IP is required")
	}
	allowed := make([]string, 0, len(request.AllowedIPs))
	for _, raw := range request.AllowedIPs {
		value := strings.TrimSpace(raw)
		if _, _, err := net.ParseCIDR(value); err != nil {
			return "", fmt.Errorf("invalid allowed IP %q", value)
		}
		allowed = append(allowed, value)
	}
	endpoint := strings.TrimSpace(request.Endpoint)
	if endpoint != "" {
		if _, _, err := net.SplitHostPort(endpoint); err != nil {
			return "", errors.New("endpoint must use host:port")
		}
	}
	if request.Keepalive < 0 || request.Keepalive > 65535 {
		return "", errors.New("persistent keepalive must be between 0 and 65535")
	}
	preshared := strings.TrimSpace(request.PresharedKey)
	if preshared != "" {
		if err := validateWireGuardKey(preshared); err != nil {
			return "", fmt.Errorf("invalid preshared key: %w", err)
		}
	}

	path := wireGuardConfigPath(name)
	original, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("WireGuard tunnel does not exist")
		}
		return "", err
	}
	updated, err := appendWireGuardPeer(string(original), publicKey, preshared, allowed, endpoint, request.Keepalive)
	if err != nil {
		return "", err
	}
	if err := writeWireGuardConfig(path, updated); err != nil {
		return "", err
	}
	if wireGuardActive(ctx, name) {
		if err := applyWireGuardPeer(ctx, name, publicKey, preshared, allowed, endpoint, request.Keepalive); err != nil {
			_ = writeWireGuardConfig(path, string(original))
			return "", err
		}
	}
	return fmt.Sprintf("WireGuard peer added to %s", name), nil
}

func deleteWireGuardPeer(ctx context.Context, request updaterhelper.Request) (string, error) {
	name := strings.TrimSpace(request.Tunnel)
	if !validWireGuardName(name) {
		return "", errors.New("invalid WireGuard tunnel name")
	}
	publicKey := strings.TrimSpace(request.PeerPublicKey)
	if err := validateWireGuardKey(publicKey); err != nil {
		return "", fmt.Errorf("invalid peer public key: %w", err)
	}
	path := wireGuardConfigPath(name)
	original, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("WireGuard tunnel does not exist")
		}
		return "", err
	}
	updated, found := removeWireGuardPeer(string(original), publicKey)
	if !found {
		return "", errors.New("WireGuard peer not found")
	}
	if err := writeWireGuardConfig(path, updated); err != nil {
		return "", err
	}
	if wireGuardToolsAvailable() && wireGuardActive(ctx, name) {
		if err := runNetworkCommand(ctx, "wg", "set", name, "peer", publicKey, "remove"); err != nil {
			_ = writeWireGuardConfig(path, string(original))
			return "", err
		}
	}
	return fmt.Sprintf("WireGuard peer removed from %s", name), nil
}

func applyWireGuardPeer(
	ctx context.Context,
	name, publicKey, preshared string,
	allowed []string,
	endpoint string,
	keepalive int,
) error {
	args := []string{"set", name, "peer", publicKey, "allowed-ips", strings.Join(allowed, ",")}
	if endpoint != "" {
		args = append(args, "endpoint", endpoint)
	}
	if keepalive > 0 {
		args = append(args, "persistent-keepalive", strconv.Itoa(keepalive))
	}
	var temp string
	if preshared != "" {
		file, err := os.CreateTemp("", "home-ai-wg-psk-*")
		if err != nil {
			return err
		}
		temp = file.Name()
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			_ = os.Remove(temp)
			return err
		}
		if _, err := file.WriteString(preshared + "\n"); err != nil {
			_ = file.Close()
			_ = os.Remove(temp)
			return err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(temp)
			return err
		}
		defer os.Remove(temp)
		args = append(args, "preshared-key", temp)
	}
	return runNetworkCommand(ctx, "wg", args...)
}

func wireGuardPublicKey(ctx context.Context, privateKey string) (string, error) {
	output, err := runHostCommand(ctx, strings.TrimSpace(privateKey)+"\n", "wg", "pubkey")
	if err != nil {
		return "", errors.New("invalid WireGuard private key")
	}
	publicKey := strings.TrimSpace(output)
	if err := validateWireGuardKey(publicKey); err != nil {
		return "", errors.New("failed to derive WireGuard public key")
	}
	return publicKey, nil
}

func validateWireGuardKey(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return errors.New("key must be a base64-encoded 32-byte value")
	}
	return nil
}

func validWireGuardName(name string) bool {
	return validInterfaceName(name) && name != "lo"
}

func wireGuardConfigPath(name string) string {
	return filepath.Join(wireGuardDir, name+".conf")
}

func writeWireGuardConfig(path, content string) error {
	if !strings.HasPrefix(filepath.Clean(path), wireGuardDir+string(os.PathSeparator)) {
		return errors.New("unsafe WireGuard config path")
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write WireGuard config: %w", err)
	}
	if err := os.Chmod(temp, 0o600); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("replace WireGuard config: %w", err)
	}
	return nil
}

func appendWireGuardPeer(
	config, publicKey, preshared string,
	allowed []string,
	endpoint string,
	keepalive int,
) (string, error) {
	if wireGuardConfigHasPeer(config, publicKey) {
		return "", errors.New("WireGuard peer already exists")
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(config))
	b.WriteString("\n\n[Peer]\n")
	b.WriteString("PublicKey = " + publicKey + "\n")
	if preshared != "" {
		b.WriteString("PresharedKey = " + preshared + "\n")
	}
	b.WriteString("AllowedIPs = " + strings.Join(allowed, ", ") + "\n")
	if endpoint != "" {
		b.WriteString("Endpoint = " + endpoint + "\n")
	}
	if keepalive > 0 {
		b.WriteString("PersistentKeepalive = " + strconv.Itoa(keepalive) + "\n")
	}
	return b.String(), nil
}

func wireGuardConfigHasPeer(config, publicKey string) bool {
	inPeer := false
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inPeer = strings.EqualFold(line, "[Peer]")
			continue
		}
		if !inPeer {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "PublicKey") &&
			strings.TrimSpace(value) == publicKey {
			return true
		}
	}
	return false
}

func removeWireGuardPeer(config, publicKey string) (string, bool) {
	lines := strings.Split(config, "\n")
	var out []string
	found := false
	for i := 0; i < len(lines); {
		if !strings.EqualFold(strings.TrimSpace(lines[i]), "[Peer]") {
			out = append(out, lines[i])
			i++
			continue
		}
		start := i
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			i++
		}
		block := strings.Join(lines[start:i], "\n")
		if wireGuardConfigHasPeer(block, publicKey) {
			found = true
			continue
		}
		out = append(out, lines[start:i]...)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n", found
}

func wireGuardActive(ctx context.Context, name string) bool {
	_, err := runHostCommand(ctx, "", "wg", "show", name)
	return err == nil
}

func inspectWireGuard(ctx context.Context) (bool, string, []updaterhelper.WireGuardTunnelStat) {
	if !wireGuardToolsAvailable() {
		return false, "wireguard-tools are not installed", nil
	}
	names := map[string]bool{}
	if configs, err := filepath.Glob(filepath.Join(wireGuardDir, "*.conf")); err == nil {
		for _, path := range configs {
			name := strings.TrimSuffix(filepath.Base(path), ".conf")
			if validWireGuardName(name) {
				names[name] = false
			}
		}
	}
	if output, err := runHostCommand(ctx, "", "wg", "show", "interfaces"); err == nil {
		for _, name := range strings.Fields(output) {
			if validWireGuardName(name) {
				names[name] = true
			}
		}
	}

	result := make([]updaterhelper.WireGuardTunnelStat, 0, len(names))
	for name, active := range names {
		item := updaterhelper.WireGuardTunnelStat{Name: name, Active: active}
		if data, err := os.ReadFile(wireGuardConfigPath(name)); err == nil {
			item.Address, item.ListenPort = wireGuardInterfaceConfig(string(data))
		}
		if active {
			populateWireGuardRuntime(ctx, &item)
		}
		result = append(result, item)
	}
	sortWireGuardTunnels(result)
	return true, "", result
}

func wireGuardInterfaceConfig(config string) (string, int) {
	inInterface := false
	var address string
	var port int
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inInterface = strings.EqualFold(line, "[Interface]")
			continue
		}
		if !inInterface {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "address":
			address = strings.TrimSpace(value)
		case "listenport":
			port, _ = strconv.Atoi(strings.TrimSpace(value))
		}
	}
	return address, port
}

func populateWireGuardRuntime(ctx context.Context, item *updaterhelper.WireGuardTunnelStat) {
	output, err := runHostCommand(ctx, "", "wg", "show", item.Name, "dump")
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return
	}
	fields := strings.Split(lines[0], "\t")
	if len(fields) >= 4 {
		item.PublicKey = fields[1]
		if port, err := strconv.Atoi(fields[2]); err == nil && port > 0 {
			item.ListenPort = port
		}
	}
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}
		peer := updaterhelper.WireGuardPeerStat{
			PublicKey:  fields[0],
			Endpoint:   dashEmpty(fields[2]),
			AllowedIPs: splitCSV(fields[3]),
		}
		peer.LatestHandshake, _ = strconv.ParseInt(fields[4], 10, 64)
		peer.TransferRX, _ = strconv.ParseUint(fields[5], 10, 64)
		peer.TransferTX, _ = strconv.ParseUint(fields[6], 10, 64)
		peer.Keepalive, _ = strconv.Atoi(fields[7])
		item.Peers = append(item.Peers, peer)
	}
}

func dashEmpty(value string) string {
	if value == "(none)" {
		return ""
	}
	return value
}

func splitCSV(value string) []string {
	if value == "" || value == "(none)" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func sortWireGuardTunnels(items []updaterhelper.WireGuardTunnelStat) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Name < items[j-1].Name; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
