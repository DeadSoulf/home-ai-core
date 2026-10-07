package modules

import (
	"fmt"
	"strings"
)

func CompareVersions(left, right string) (int, error) {
	a, err := parseVersion(left)
	if err != nil {
		return 0, err
	}
	b, err := parseVersion(right)
	if err != nil {
		return 0, err
	}
	return compareVersion(a, b), nil
}

func CheckCompatibility(manifest Manifest, coreVersion, architecture string, capabilities []string) error {
	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	current := strings.TrimPrefix(strings.TrimSpace(coreVersion), "v")
	if idx := strings.IndexAny(current, "+-"); idx >= 0 {
		current = current[:idx]
	}
	if current != "" {
		if _, err := parseVersion(current); err == nil {
			ok, err := satisfies(current, manifest.Core)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("module %q is incompatible with Core %s", manifest.ID, coreVersion)
			}
		}
	}

	if len(manifest.Host.Architectures) > 0 {
		allowed := false
		for _, candidate := range manifest.Host.Architectures {
			if candidate == architecture {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("module %q does not support architecture %s", manifest.ID, architecture)
		}
	}

	available := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		available[capability] = struct{}{}
	}
	for _, required := range manifest.Capabilities.Requires {
		if required == "host.docker" {
			continue
		}
		if _, ok := available[required]; !ok {
			return fmt.Errorf("module %q requires unavailable capability %s", manifest.ID, required)
		}
	}
	return nil
}
