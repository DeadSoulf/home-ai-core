package modules

import (
	"fmt"
	"runtime"
	"sort"
)

type PlanInput struct {
	CoreVersion  string
	Target       string
	Available    map[string]Manifest
	Installed    map[string]string
	Capabilities []string
	Architecture string
}

type Plan struct {
	Order []Manifest `json:"order"`
}

func PlanInstall(input PlanInput) (Plan, error) {
	if input.Architecture == "" {
		input.Architecture = runtime.GOARCH
	}
	target, ok := input.Available[input.Target]
	if !ok {
		return Plan{}, fmt.Errorf("target module %q is unavailable", input.Target)
	}

	capabilities := map[string]struct{}{}
	for _, value := range input.Capabilities {
		capabilities[value] = struct{}{}
	}
	for id, installedVersion := range input.Installed {
		if manifest, exists := input.Available[id]; exists && manifest.Version == installedVersion {
			for _, capability := range manifest.Capabilities.Provides {
				capabilities[capability] = struct{}{}
			}
		}
	}

	state := map[string]int{}
	selected := map[string]Manifest{}
	order := []Manifest{}
	var visit func(Manifest) error
	visit = func(m Manifest) error {
		switch state[m.ID] {
		case 1:
			return fmt.Errorf("dependency cycle includes %q", m.ID)
		case 2:
			return nil
		}
		if err := ValidateManifest(m); err != nil {
			return fmt.Errorf("module %q: %w", m.ID, err)
		}
		coreOK, err := satisfies(input.CoreVersion, m.Core)
		if err != nil || !coreOK {
			return fmt.Errorf("module %q is incompatible with Core %s", m.ID, input.CoreVersion)
		}
		if len(m.Host.Architectures) > 0 && !contains(m.Host.Architectures, input.Architecture) {
			return fmt.Errorf("module %q does not support architecture %s", m.ID, input.Architecture)
		}

		for _, conflict := range m.Conflicts {
			if _, installed := input.Installed[conflict]; installed {
				return fmt.Errorf("module %q conflicts with installed module %q", m.ID, conflict)
			}
			if state[conflict] != 0 {
				return fmt.Errorf("module %q conflicts with planned module %q", m.ID, conflict)
			}
		}

		state[m.ID] = 1
		deps := append([]Dependency(nil), m.Dependencies...)
		sort.Slice(deps, func(i, j int) bool { return deps[i].ID < deps[j].ID })
		for _, dep := range deps {
			if installedVersion, installed := input.Installed[dep.ID]; installed {
				ok, err := satisfies(installedVersion, dep.Version)
				if err != nil || !ok {
					return fmt.Errorf("installed dependency %q version %s does not satisfy %s", dep.ID, installedVersion, dep.Version)
				}
				continue
			}
			depManifest, exists := input.Available[dep.ID]
			if !exists {
				return fmt.Errorf("dependency %q required by %q is unavailable", dep.ID, m.ID)
			}
			ok, err := satisfies(depManifest.Version, dep.Version)
			if err != nil || !ok {
				return fmt.Errorf("dependency %q version %s does not satisfy %s", dep.ID, depManifest.Version, dep.Version)
			}
			if err := visit(depManifest); err != nil {
				return err
			}
		}

		for _, required := range m.Capabilities.Requires {
			if _, ok := capabilities[required]; !ok {
				return fmt.Errorf("module %q requires unavailable capability %q", m.ID, required)
			}
		}
		for _, provided := range m.Capabilities.Provides {
			capabilities[provided] = struct{}{}
		}
		state[m.ID] = 2
		selected[m.ID] = m
		if _, installed := input.Installed[m.ID]; !installed {
			order = append(order, m)
		}
		return nil
	}

	if err := visit(target); err != nil {
		return Plan{}, err
	}
	if err := validateSelectedConflicts(selected, input.Installed, input.Available); err != nil {
		return Plan{}, err
	}
	return Plan{Order: order}, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validateSelectedConflicts(
	selected map[string]Manifest,
	installed map[string]string,
	available map[string]Manifest,
) error {
	for id, manifest := range selected {
		for _, conflict := range manifest.Conflicts {
			if _, planned := selected[conflict]; planned {
				return fmt.Errorf("planned modules %q and %q conflict", id, conflict)
			}
			if _, present := installed[conflict]; present {
				return fmt.Errorf("module %q conflicts with installed module %q", id, conflict)
			}
		}
		for installedID := range installed {
			installedManifest, ok := available[installedID]
			if !ok {
				continue
			}
			if contains(installedManifest.Conflicts, id) {
				return fmt.Errorf("installed module %q conflicts with module %q", installedID, id)
			}
		}
	}
	return nil
}
