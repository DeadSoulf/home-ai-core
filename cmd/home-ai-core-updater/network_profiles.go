package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const networkdProfileDir = "/etc/systemd/network"

func inspectNetworkProfiles(ctx context.Context) (string, []updaterhelper.NetworkProfileStat) {
	backend := detectNetworkBackend(ctx)
	names := listNetworkInterfaceNames()
	result := make([]updaterhelper.NetworkProfileStat, 0, len(names))
	for _, name := range names {
		if name == "lo" || wireGuardConfigExists(name) {
			continue
		}
		var profile updaterhelper.NetworkProfileStat
		switch backend {
		case "networkmanager":
			profile = inspectNetworkManagerProfile(ctx, name)
		case "systemd-networkd":
			profile = inspectNetworkdProfile(name)
		case "ifupdown":
			profile = updaterhelper.NetworkProfileStat{
				Interface: name,
				Backend:   backend,
				Supported: false,
				Source:    "/etc/network/interfaces",
				Error:     "persistent profile editing for ifupdown is not supported yet",
			}
		default:
			profile = updaterhelper.NetworkProfileStat{
				Interface: name,
				Backend:   backend,
				Supported: false,
				Error:     "no supported network configuration backend detected",
			}
		}
		result = append(result, profile)
	}
	return backend, result
}

func saveNetworkProfile(ctx context.Context, request updaterhelper.Request) (string, error) {
	iface, err := requireNetworkInterface(request.Interface)
	if err != nil {
		return "", err
	}
	if wireGuardConfigExists(iface) {
		return "", errors.New("WireGuard interfaces are managed in the WireGuard section")
	}

	method := strings.ToLower(strings.TrimSpace(request.NetworkMethod))
	if method != "dhcp" && method != "static" {
		return "", errors.New("network method must be dhcp or static")
	}
	address := strings.TrimSpace(request.Address)
	gateway := strings.TrimSpace(request.Gateway)
	dns, err := validateDNSAddresses(request.DNS)
	if err != nil {
		return "", err
	}

	if method == "static" {
		ip, _, err := net.ParseCIDR(address)
		if err != nil {
			return "", errors.New("static address must be a valid CIDR")
		}
		if gateway != "" {
			gatewayIP := net.ParseIP(gateway)
			if gatewayIP == nil {
				return "", errors.New("gateway must be a valid IP address")
			}
			if (ip.To4() == nil) != (gatewayIP.To4() == nil) {
				return "", errors.New("address and gateway must use the same IP family")
			}
		}
	} else {
		address = ""
		gateway = ""
	}

	switch backend := detectNetworkBackend(ctx); backend {
	case "networkmanager":
		return saveNetworkManagerProfile(ctx, iface, method, address, gateway, dns)
	case "systemd-networkd":
		return saveNetworkdProfile(ctx, iface, method, address, gateway, dns)
	case "ifupdown":
		return "", errors.New("ifupdown is active; persistent Home-AI profile editing is not supported yet")
	default:
		return "", errors.New("no supported persistent network backend detected")
	}
}

func detectNetworkBackend(ctx context.Context) string {
	if commandAvailable("nmcli") && systemServiceActive(ctx, "NetworkManager.service") {
		return "networkmanager"
	}
	if commandAvailable("networkctl") && systemServiceActive(ctx, "systemd-networkd.service") {
		return "systemd-networkd"
	}
	if _, err := os.Stat("/etc/network/interfaces"); err == nil {
		return "ifupdown"
	}
	return "unknown"
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func systemServiceActive(ctx context.Context, service string) bool {
	return exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", service).Run() == nil
}

func listNetworkInterfaceNames() []string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == "" || !validInterfaceName(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

func wireGuardConfigExists(name string) bool {
	_, err := os.Stat(wireGuardConfigPath(name))
	return err == nil
}

func validateDNSAddresses(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if net.ParseIP(value) == nil {
			return nil, fmt.Errorf("DNS server %q is not a valid IP address", value)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func inspectNetworkManagerProfile(ctx context.Context, iface string) updaterhelper.NetworkProfileStat {
	profile := updaterhelper.NetworkProfileStat{
		Interface: iface,
		Backend:   "networkmanager",
		Supported: true,
	}
	deviceType, err := hostCommandValue(ctx, "nmcli", "-g", "GENERAL.TYPE", "device", "show", iface)
	if err != nil {
		profile.Supported = false
		profile.Error = err.Error()
		return profile
	}
	if deviceType != "ethernet" {
		profile.Supported = false
		profile.Error = "only Ethernet profiles are managed in Network Management v2"
		return profile
	}

	connection, err := networkManagerConnection(ctx, iface)
	if err != nil {
		profile.Error = err.Error()
		return profile
	}
	if connection == "" {
		return profile
	}

	profile.Managed = true
	profile.Source = connection
	method, _ := hostCommandValue(ctx, "nmcli", "-g", "ipv4.method", "connection", "show", connection)
	switch method {
	case "auto":
		profile.Method = "dhcp"
	case "manual":
		profile.Method = "static"
	default:
		profile.Method = method
	}
	profile.Address, _ = hostCommandValue(ctx, "nmcli", "-g", "ipv4.addresses", "connection", "show", connection)
	profile.Address = firstListValue(profile.Address)
	profile.Gateway, _ = hostCommandValue(ctx, "nmcli", "-g", "ipv4.gateway", "connection", "show", connection)
	dnsRaw, _ := hostCommandValue(ctx, "nmcli", "-g", "ipv4.dns", "connection", "show", connection)
	profile.DNS = splitNetworkValues(dnsRaw)
	return profile
}

func saveNetworkManagerProfile(
	ctx context.Context,
	iface, method, address, gateway string,
	dns []string,
) (string, error) {
	deviceType, err := hostCommandValue(ctx, "nmcli", "-g", "GENERAL.TYPE", "device", "show", iface)
	if err != nil {
		return "", err
	}
	if deviceType != "ethernet" {
		return "", errors.New("only Ethernet NetworkManager profiles are supported")
	}

	connection, err := networkManagerConnection(ctx, iface)
	if err != nil {
		return "", err
	}
	if connection == "" {
		connection = "home-ai-" + iface
		if _, err := runHostCommand(
			ctx, "", "nmcli",
			"connection", "add",
			"type", "ethernet",
			"ifname", iface,
			"con-name", connection,
		); err != nil {
			return "", err
		}
	}

	args := []string{"connection", "modify", connection}
	if method == "dhcp" {
		args = append(args,
			"ipv4.method", "auto",
			"ipv4.addresses", "",
			"ipv4.gateway", "",
		)
	} else {
		args = append(args,
			"ipv4.method", "manual",
			"ipv4.addresses", address,
			"ipv4.gateway", gateway,
		)
	}
	if len(dns) == 0 {
		args = append(args, "ipv4.dns", "", "ipv4.ignore-auto-dns", "no")
	} else {
		args = append(args, "ipv4.dns", strings.Join(dns, ","), "ipv4.ignore-auto-dns", "yes")
	}
	if _, err := runHostCommand(ctx, "", "nmcli", args...); err != nil {
		return "", err
	}
	if _, err := runHostCommand(ctx, "", "nmcli", "connection", "up", connection); err != nil {
		return "", err
	}
	return fmt.Sprintf("persistent %s profile applied to %s using NetworkManager", method, iface), nil
}

func networkManagerConnection(ctx context.Context, iface string) (string, error) {
	value, err := hostCommandValue(ctx, "nmcli", "-g", "GENERAL.CONNECTION", "device", "show", iface)
	if err != nil {
		return "", err
	}
	if value == "--" || value == "" {
		return "", nil
	}
	return value, nil
}

func hostCommandValue(ctx context.Context, command string, args ...string) (string, error) {
	output, err := runHostCommand(ctx, "", command, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func inspectNetworkdProfile(iface string) updaterhelper.NetworkProfileStat {
	profile := updaterhelper.NetworkProfileStat{
		Interface: iface,
		Backend:   "systemd-networkd",
		Supported: true,
	}
	path := networkdProfilePath(iface)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return profile
	}
	if err != nil {
		profile.Error = err.Error()
		return profile
	}

	profile.Managed = true
	profile.Source = path
	profile.Method, profile.Address, profile.Gateway, profile.DNS = parseNetworkdProfile(string(data))
	return profile
}

func saveNetworkdProfile(
	ctx context.Context,
	iface, method, address, gateway string,
	dns []string,
) (string, error) {
	if err := os.MkdirAll(networkdProfileDir, 0o755); err != nil {
		return "", fmt.Errorf("create networkd config directory: %w", err)
	}
	path := networkdProfilePath(iface)
	content := renderNetworkdProfile(iface, method, address, gateway, dns)
	temp := path + ".tmp"
	if err := os.WriteFile(temp, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write networkd profile: %w", err)
	}
	if err := os.Chmod(temp, 0o644); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return "", fmt.Errorf("replace networkd profile: %w", err)
	}
	if _, err := runHostCommand(ctx, "", "networkctl", "reload"); err != nil {
		return "", err
	}
	if _, err := runHostCommand(ctx, "", "networkctl", "reconfigure", iface); err != nil {
		return "", err
	}
	return fmt.Sprintf("persistent %s profile applied to %s using systemd-networkd", method, iface), nil
}

func networkdProfilePath(iface string) string {
	return filepath.Join(networkdProfileDir, "05-home-ai-"+iface+".network")
}

func renderNetworkdProfile(
	iface, method, address, gateway string,
	dns []string,
) string {
	var b strings.Builder
	b.WriteString("[Match]\n")
	b.WriteString("Name=" + iface + "\n\n")
	b.WriteString("[Network]\n")
	if method == "dhcp" {
		b.WriteString("DHCP=ipv4\n")
		b.WriteString("IPv6AcceptRA=yes\n")
	} else {
		b.WriteString("Address=" + address + "\n")
		if gateway != "" {
			b.WriteString("Gateway=" + gateway + "\n")
		}
		b.WriteString("IPv6AcceptRA=yes\n")
	}
	for _, server := range dns {
		b.WriteString("DNS=" + server + "\n")
	}
	return b.String()
}

func parseNetworkdProfile(content string) (method, address, gateway string, dns []string) {
	inNetwork := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inNetwork = strings.EqualFold(line, "[Network]")
			continue
		}
		if !inNetwork {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "dhcp":
			if value == "yes" || value == "ipv4" {
				method = "dhcp"
			}
		case "address":
			if address == "" {
				address = value
			}
			if method == "" {
				method = "static"
			}
		case "gateway":
			gateway = value
		case "dns":
			if value != "" {
				dns = append(dns, value)
			}
		}
	}
	return method, address, gateway, dns
}

func firstListValue(value string) string {
	values := splitNetworkValues(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func splitNetworkValues(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if field := strings.TrimSpace(field); field != "" && field != "--" {
			result = append(result, field)
		}
	}
	return result
}
