package modules

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

func DiscoverHostCapabilities() []string {
	capabilities := map[string]struct{}{}

	if runtime.GOOS == "linux" {
		capabilities["host.linux"] = struct{}{}
	}
	switch runtime.GOARCH {
	case "amd64", "arm64":
		capabilities["host.arch."+runtime.GOARCH] = struct{}{}
	}

	if directoryExists("/run/systemd/system") {
		capabilities["host.systemd"] = struct{}{}
	}
	if fileExists("/dev/kvm") {
		capabilities["host.kvm"] = struct{}{}
	}
	if cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*"); len(cards) > 0 {
		capabilities["host.gpu"] = struct{}{}
	}

	out := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) Capabilities(ctx context.Context) ([]string, error) {
	items, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	capabilities := map[string]struct{}{}
	for _, capability := range DiscoverHostCapabilities() {
		capabilities[capability] = struct{}{}
	}
	for _, item := range items {
		if item.Status == "error" || item.Status == "disabled" {
			continue
		}
		for _, capability := range item.Manifest.Capabilities.Provides {
			capabilities[capability] = struct{}{}
		}
	}

	out := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out, nil
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
