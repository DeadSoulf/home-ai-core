package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const (
	ifupdownMainPath      = "/etc/network/interfaces"
	ifupdownDir           = "/etc/network/interfaces.d"
	ifupdownHomeAIPrefix  = "50-home-ai-"
	ifupdownManagedMarker = "# Managed by Home-AI-Core"
	ifupdownIncludeBegin  = "# BEGIN Home-AI-Core managed include"
	ifupdownIncludeLine   = "source /etc/network/interfaces.d/*"
	ifupdownIncludeEnd    = "# END Home-AI-Core managed include"
)

type ifupdownStanza struct {
	Interface string
	Family    string
	Method    string
	Address   string
	Netmask   string
	Gateway   string
	DNS       []string
	Source    string
	Owned     bool
}

type ifupdownInventory struct {
	Stanzas     map[string][]ifupdownStanza
	IncludedDir bool
}

func inspectIfupdownProfiles(names []string) map[string]updaterhelper.NetworkProfileStat {
	inventory, err := loadIfupdownInventory()
	result := make(map[string]updaterhelper.NetworkProfileStat, len(names))
	for _, name := range names {
		profile := updaterhelper.NetworkProfileStat{
			Interface: name,
			Backend:   "ifupdown",
			Supported: true,
			Ownership: "none",
		}
		if err != nil {
			profile.Supported = false
			profile.Error = err.Error()
			result[name] = profile
			continue
		}

		stanzas := inventory.Stanzas[name]
		if len(stanzas) == 0 {
			result[name] = profile
			continue
		}

		profile.Managed = true
		if len(stanzas) > 1 {
			profile.Supported = false
			profile.Ownership = "conflict"
			profile.Source = joinIfupdownSources(stanzas)
			profile.Error = "multiple active ifupdown IPv4 stanzas exist for this interface; Home-AI will not modify them"
			fillNetworkProfileFromIfupdown(&profile, stanzas[0])
			result[name] = profile
			continue
		}

		stanza := stanzas[0]
		fillNetworkProfileFromIfupdown(&profile, stanza)
		if stanza.Owned {
			profile.Ownership = "home-ai"
		} else {
			profile.Ownership = "external"
			profile.Supported = false
			profile.Error = "profile is externally managed; Home-AI will not overwrite it"
		}
		result[name] = profile
	}
	return result
}

func saveIfupdownProfile(
	ctx context.Context,
	iface, method, address, gateway string,
	dns []string,
) (string, error) {
	inventory, err := loadIfupdownInventory()
	if err != nil {
		return "", err
	}
	stanzas := inventory.Stanzas[iface]
	if len(stanzas) > 1 {
		return "", errors.New("multiple active ifupdown IPv4 stanzas exist for this interface; refusing to modify configuration")
	}
	if len(stanzas) == 1 && !stanzas[0].Owned {
		return "", fmt.Errorf(
			"interface %s is externally managed in %s; Home-AI will not overwrite it",
			iface,
			stanzas[0].Source,
		)
	}

	if err := ensureIfupdownInclude(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(ifupdownDir, 0o755); err != nil {
		return "", fmt.Errorf("create ifupdown include directory: %w", err)
	}

	path := ifupdownOwnedPath(iface)
	content := renderIfupdownProfile(iface, method, address, gateway, dns)
	if err := atomicWriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write ifupdown profile: %w", err)
	}

	if err := applyIfupdownInterface(ctx, iface); err != nil {
		return "", err
	}
	return fmt.Sprintf("persistent %s profile applied to %s using ifupdown", method, iface), nil
}

func loadIfupdownInventory() (ifupdownInventory, error) {
	inventory := ifupdownInventory{Stanzas: map[string][]ifupdownStanza{}}

	mainData, err := os.ReadFile(ifupdownMainPath)
	if err != nil {
		return inventory, fmt.Errorf("read %s: %w", ifupdownMainPath, err)
	}
	mainText := string(mainData)
	for _, stanza := range parseIfupdownFile(ifupdownMainPath, mainText, false) {
		if stanza.Family == "inet" {
			inventory.Stanzas[stanza.Interface] = append(inventory.Stanzas[stanza.Interface], stanza)
		}
	}

	inventory.IncludedDir = ifupdownIncludesDir(mainText)
	if !inventory.IncludedDir {
		return inventory, nil
	}

	entries, err := os.ReadDir(ifupdownDir)
	if errors.Is(err, os.ErrNotExist) {
		return inventory, nil
	}
	if err != nil {
		return inventory, fmt.Errorf("read %s: %w", ifupdownDir, err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !validIfupdownIncludeName(entry.Name()) {
			continue
		}
		path := filepath.Join(ifupdownDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return inventory, fmt.Errorf("read %s: %w", path, err)
		}
		owned := strings.HasPrefix(entry.Name(), ifupdownHomeAIPrefix) &&
			strings.Contains(string(data), ifupdownManagedMarker)
		for _, stanza := range parseIfupdownFile(path, string(data), owned) {
			if stanza.Family == "inet" {
				inventory.Stanzas[stanza.Interface] = append(inventory.Stanzas[stanza.Interface], stanza)
			}
		}
	}
	return inventory, nil
}

func parseIfupdownFile(source, content string, owned bool) []ifupdownStanza {
	var result []ifupdownStanza
	var current *ifupdownStanza

	flush := func() {
		if current != nil {
			result = append(result, *current)
			current = nil
		}
	}

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "iface":
			flush()
			if len(fields) >= 4 {
				current = &ifupdownStanza{
					Interface: fields[1],
					Family:    fields[2],
					Method:    fields[3],
					Source:    source,
					Owned:     owned,
				}
			}
			continue
		case "auto", "allow-auto", "allow-hotplug", "source", "source-directory", "mapping":
			continue
		}
		if current == nil {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
		switch strings.ToLower(fields[0]) {
		case "address":
			if current.Address == "" {
				current.Address = value
			}
		case "netmask":
			current.Netmask = value
		case "gateway":
			current.Gateway = value
		case "dns-nameservers":
			current.DNS = append(current.DNS, strings.Fields(value)...)
		}
	}
	flush()
	return result
}

func fillNetworkProfileFromIfupdown(profile *updaterhelper.NetworkProfileStat, stanza ifupdownStanza) {
	profile.Method = stanza.Method
	profile.Address = normalizeIfupdownAddress(stanza.Address, stanza.Netmask)
	profile.Gateway = stanza.Gateway
	profile.DNS = append([]string(nil), stanza.DNS...)
	profile.Source = stanza.Source
}

func normalizeIfupdownAddress(address, netmask string) string {
	address = strings.TrimSpace(address)
	netmask = strings.TrimSpace(netmask)
	if address == "" || strings.Contains(address, "/") || netmask == "" {
		return address
	}
	ip := net.ParseIP(address)
	if ip == nil || ip.To4() == nil {
		return address
	}
	prefix, ok := ifupdownNetmaskPrefix(netmask)
	if !ok {
		return address
	}
	return address + "/" + strconv.Itoa(prefix)
}

func ifupdownNetmaskPrefix(value string) (int, bool) {
	if n, err := strconv.Atoi(value); err == nil && n >= 0 && n <= 32 {
		return n, true
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return 0, false
	}
	mask := net.IPMask(ip.To4())
	ones, bits := mask.Size()
	if bits != 32 || ones < 0 {
		return 0, false
	}
	return ones, true
}

func renderIfupdownProfile(
	iface, method, address, gateway string,
	dns []string,
) string {
	var b strings.Builder
	b.WriteString(ifupdownManagedMarker + "\n")
	b.WriteString("# This file is owned by Home-AI-Core.\n")
	b.WriteString("auto " + iface + "\n")
	b.WriteString("iface " + iface + " inet " + method + "\n")
	if method == "static" {
		b.WriteString("    address " + address + "\n")
		if gateway != "" {
			b.WriteString("    gateway " + gateway + "\n")
		}
	}
	if len(dns) > 0 {
		b.WriteString("    dns-nameservers " + strings.Join(dns, " ") + "\n")
	}
	return b.String()
}

func ifupdownIncludesDir(content string) bool {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "source":
			for _, pattern := range fields[1:] {
				if pattern == "/etc/network/interfaces.d/*" ||
					strings.HasPrefix(pattern, "/etc/network/interfaces.d/") {
					return true
				}
			}
		case "source-directory":
			for _, dir := range fields[1:] {
				if strings.TrimRight(dir, "/") == "/etc/network/interfaces.d" {
					return true
				}
			}
		}
	}
	return false
}

func ensureIfupdownIncludeContent(content string) (string, bool) {
	if ifupdownIncludesDir(content) {
		return content, false
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed != "" {
		trimmed += "\n\n"
	}
	trimmed += ifupdownIncludeBegin + "\n" +
		ifupdownIncludeLine + "\n" +
		ifupdownIncludeEnd + "\n"
	return trimmed, true
}

func ensureIfupdownInclude() error {
	data, err := os.ReadFile(ifupdownMainPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", ifupdownMainPath, err)
	}
	updated, changed := ensureIfupdownIncludeContent(string(data))
	if !changed {
		return nil
	}

	info, err := os.Stat(ifupdownMainPath)
	if err != nil {
		return fmt.Errorf("stat %s: %w", ifupdownMainPath, err)
	}
	backup := ifupdownMainPath + ".home-ai.bak." + time.Now().UTC().Format("20060102T150405Z")
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("backup %s: %w", ifupdownMainPath, err)
	}
	if err := atomicWriteFile(ifupdownMainPath, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Errorf("add Home-AI include to %s: %w", ifupdownMainPath, err)
	}
	return nil
}

func atomicWriteFile(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".home-ai-network-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func applyIfupdownInterface(ctx context.Context, iface string) error {
	if !commandAvailable("ifup") {
		return errors.New("ifup is unavailable")
	}

	if commandAvailable("ifquery") {
		if _, err := runHostCommand(ctx, "", "ifquery", iface); err != nil {
			return fmt.Errorf("validate ifupdown profile for %s: %w", iface, err)
		}
	}

	if commandAvailable("ifdown") {
		// A newly managed interface may not currently be in ifupdown state.
		// A failed down is therefore non-fatal; the following ifup is decisive.
		_, _ = runHostCommand(ctx, "", "ifdown", "--force", iface)
	}
	if _, err := runHostCommand(ctx, "", "ifup", iface); err != nil {
		return fmt.Errorf("apply ifupdown profile for %s: %w", iface, err)
	}
	return nil
}

func ifupdownOwnedPath(iface string) string {
	return filepath.Join(ifupdownDir, ifupdownHomeAIPrefix+iface)
}

func validIfupdownIncludeName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") ||
		strings.HasSuffix(name, "~") ||
		strings.Contains(name, ".bak") ||
		strings.Contains(name, ".tmp") ||
		strings.Contains(name, ".dpkg-") {
		return false
	}
	return true
}

func joinIfupdownSources(stanzas []ifupdownStanza) string {
	seen := map[string]struct{}{}
	var sources []string
	for _, stanza := range stanzas {
		if _, ok := seen[stanza.Source]; ok {
			continue
		}
		seen[stanza.Source] = struct{}{}
		sources = append(sources, stanza.Source)
	}
	sort.Strings(sources)
	return strings.Join(sources, ", ")
}
