package modules

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const ManifestSchemaVersion = 1

var (
	moduleIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	namePattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)+$`)
)

var lifecycleOps = map[string]struct{}{
	"install": {},
	"upgrade": {},
	"remove":  {},
	"backup":  {},
	"restore": {},
}

func DecodeManifest(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 256<<10))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, fmt.Errorf("manifest must contain one JSON object")
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(m Manifest) error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema version %d", m.SchemaVersion)
	}
	if !moduleIDPattern.MatchString(m.ID) || len(m.ID) > 96 {
		return fmt.Errorf("invalid module id")
	}
	if strings.TrimSpace(m.Name) == "" || len([]rune(m.Name)) > 128 {
		return fmt.Errorf("invalid module name")
	}
	if len([]rune(m.Description)) > 1024 {
		return fmt.Errorf("module description is too long")
	}
	if _, err := parseVersion(m.Version); err != nil {
		return fmt.Errorf("invalid module version: %w", err)
	}
	if strings.TrimSpace(m.Core) == "" {
		return fmt.Errorf("core version constraint is required")
	}
	if _, err := satisfies("0.0.0", m.Core); err != nil {
		return fmt.Errorf("invalid core version constraint: %w", err)
	}
	if len(m.Lifecycle) == 0 {
		return fmt.Errorf("at least one lifecycle operation is required")
	}

	if err := uniqueStrings("conflict", m.Conflicts, moduleIDPattern); err != nil {
		return err
	}
	if err := uniqueStrings("permission", m.Permissions, permissionPattern); err != nil {
		return err
	}
	if err := uniqueStrings("required capability", m.Capabilities.Requires, permissionPattern); err != nil {
		return err
	}
	if err := uniqueStrings("provided capability", m.Capabilities.Provides, permissionPattern); err != nil {
		return err
	}
	if err := validatePublishedEvents(m.ID, m.Events.Publishes); err != nil {
		return err
	}
	if err := validateSubscribedEvents(m.Events.Subscribes); err != nil {
		return err
	}

	seenDeps := map[string]struct{}{}
	for _, dep := range m.Dependencies {
		if dep.ID == m.ID || !moduleIDPattern.MatchString(dep.ID) {
			return fmt.Errorf("invalid dependency %q", dep.ID)
		}
		if _, exists := seenDeps[dep.ID]; exists {
			return fmt.Errorf("duplicate dependency %q", dep.ID)
		}
		seenDeps[dep.ID] = struct{}{}
		if strings.TrimSpace(dep.Version) == "" {
			return fmt.Errorf("dependency %q requires a version constraint", dep.ID)
		}
		if _, err := satisfies("0.0.0", dep.Version); err != nil {
			return fmt.Errorf("dependency %q has invalid version constraint: %w", dep.ID, err)
		}
	}
	for _, conflict := range m.Conflicts {
		if conflict == m.ID {
			return fmt.Errorf("module cannot conflict with itself")
		}
	}

	seenLifecycle := map[string]struct{}{}
	for _, op := range m.Lifecycle {
		if _, ok := lifecycleOps[op]; !ok {
			return fmt.Errorf("unsupported lifecycle operation %q", op)
		}
		if _, exists := seenLifecycle[op]; exists {
			return fmt.Errorf("duplicate lifecycle operation %q", op)
		}
		seenLifecycle[op] = struct{}{}
	}

	if err := uniqueAllowedArchitectures(m.Host.Architectures); err != nil {
		return err
	}
	if err := uniqueStrings("package", m.Host.Packages, namePattern); err != nil {
		return err
	}

	if m.API.Namespace != "" {
		if !moduleIDPattern.MatchString(m.API.Namespace) {
			return fmt.Errorf("invalid api namespace")
		}
		if strings.HasPrefix(m.API.Namespace, "core") {
			return fmt.Errorf("core api namespace is reserved")
		}
	}

	seenNav := map[string]struct{}{}
	for _, item := range m.UI.Navigation {
		if !moduleIDPattern.MatchString(item.ID) {
			return fmt.Errorf("invalid navigation id %q", item.ID)
		}
		if _, exists := seenNav[item.ID]; exists {
			return fmt.Errorf("duplicate navigation id %q", item.ID)
		}
		seenNav[item.ID] = struct{}{}
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("navigation title is required")
		}
		expectedPrefix := "/modules/" + m.ID
		if item.Route != expectedPrefix && !strings.HasPrefix(item.Route, expectedPrefix+"/") {
			return fmt.Errorf("navigation route %q must be under %s", item.Route, expectedPrefix)
		}
	}

	return nil
}

func validatePublishedEvents(moduleID string, values []string) error {
	seen := map[string]struct{}{}
	prefix := moduleID + "."
	for _, value := range values {
		if !permissionPattern.MatchString(value) || !strings.HasPrefix(value, prefix) {
			return fmt.Errorf("published event %q must use module namespace %s*", value, prefix)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate event publish %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateSubscribedEvents(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if err := validateEventTopic(value); err != nil {
			return fmt.Errorf("invalid event subscription %q: %w", value, err)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate event subscription %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateEventTopic(value string) error {
	if value == "*" {
		return nil
	}
	if strings.HasSuffix(value, ".*") {
		base := strings.TrimSuffix(value, ".*")
		if permissionPattern.MatchString(base + ".event") {
			return nil
		}
		return fmt.Errorf("invalid wildcard namespace")
	}
	if !permissionPattern.MatchString(value) {
		return fmt.Errorf("invalid event name")
	}
	return nil
}

func uniqueAllowedArchitectures(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value != "amd64" && value != "arm64" {
			return fmt.Errorf("unsupported architecture %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate architecture %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func uniqueStrings(label string, values []string, pattern *regexp.Regexp) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !pattern.MatchString(value) {
			return fmt.Errorf("invalid %s %q", label, value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
