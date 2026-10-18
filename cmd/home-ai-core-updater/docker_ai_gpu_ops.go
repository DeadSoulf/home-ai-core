package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	aiManagedID       = "ai.agent"
	aiGPUConfirm      = "ASSIGN_AI_GPU"
	aiGPUCandidate    = "home-ai-module-ai.agent-gpu-candidate"
	aiGPUPrevious     = "home-ai-module-ai.agent-gpu-previous"
	aiGPUHealthWindow = 95 * time.Second
)

var (
	aiGPUAddressPattern = regexp.MustCompile(`^[a-fA-F0-9]{4}:[a-fA-F0-9]{2}:[a-fA-F0-9]{2}\.[0-7]$`)
	aiGPUUUIDPattern    = regexp.MustCompile(`^GPU-[0-9a-fA-F-]{36}$`)
)

type aiGPUContainer struct {
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

func validateAIGPUPCI(values []string) ([]string, error) {
	if len(values) > 8 {
		return nil, errors.New("at most eight GPUs can be assigned")
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, original := range values {
		address := strings.ToLower(strings.TrimSpace(original))
		if !aiGPUAddressPattern.MatchString(address) || seen[address] {
			return nil, errors.New("invalid or duplicate GPU PCI address")
		}
		seen[address] = true
		out = append(out, address)
	}
	return out, nil
}

// nvidia-smi typically uses an 8-digit domain (00000000:01:00.0),
// whereas sysfs/Core uses 4 hex digits (0000:01:00.0).
func normalizeNvidiaBusID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 16 && value[8] == ':' {
		value = value[4:]
	}
	if !aiGPUAddressPattern.MatchString(value) {
		return ""
	}
	return value
}

func mapAIGPUsToUUIDs(ctx context.Context, selected []string) ([]string, error) {
	if len(selected) == 0 {
		return nil, nil // CPU fallback works even after GPU removal.
	}
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil, errors.New("NVIDIA driver/nvidia-smi not found on server")
	}
	output, err := exec.CommandContext(ctx, path, "--query-gpu=pci.bus_id,uuid", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return nil, errors.New("NVIDIA GPU enumeration failed; check driver")
	}
	mapping := map[string]string{}
	for _, line := range strings.Split(string(output), "\n") {
		pair := strings.SplitN(strings.TrimSpace(line), ",", 2)
		if len(pair) != 2 {
			continue
		}
		bus := normalizeNvidiaBusID(pair[0])
		uuid := strings.TrimSpace(pair[1])
		if bus != "" && aiGPUUUIDPattern.MatchString(uuid) {
			mapping[bus] = uuid
		}
	}
	ids := make([]string, 0, len(selected))
	for _, pci := range selected {
		uuid, ok := mapping[pci]
		if !ok {
			return nil, errors.New("selected GPU is not ready in NVIDIA driver")
		}
		ids = append(ids, uuid)
	}
	return ids, nil
}

func dockerAICommand(ctx context.Context, binary string, args ...string) error {
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("managed GPU container operation failed: %s", commandError(output, err))
	}
	return nil
}

// inspectAIContainer is strict about the managed identity and /data mount,
// before the helper is allowed to recreate the container with another device set.
func inspectAIContainer(ctx context.Context, binary string) (aiGPUContainer, error) {
	// Reuse Core's trusted endpoint proof: running managed container, exact
	// module label, private IP, and matching managed Docker network identity.
	if _, err := inspectManagedModuleAddress(ctx, aiManagedID); err != nil {
		return aiGPUContainer{}, fmt.Errorf("verify managed AI network: %w", err)
	}
	name := moduleContainerName(aiManagedID)
	exists, managed, err := inspectManagedContainer(ctx, binary, name, aiManagedID)
	if err != nil {
		return aiGPUContainer{}, err
	}
	if !exists || !managed {
		return aiGPUContainer{}, errors.New("managed AI Agent is not installed")
	}
	output, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return aiGPUContainer{}, fmt.Errorf("inspect AI container: %s", commandError(output, err))
	}
	var state aiGPUContainer
	if err := json.Unmarshal(output, &state); err != nil {
		return aiGPUContainer{}, fmt.Errorf("decode AI container: %w", err)
	}
	if !state.State.Running || state.Config.Labels["home-ai.managed"] != "true" ||
		state.Config.Labels["home-ai.module.id"] != aiManagedID {
		return aiGPUContainer{}, errors.New("AI container is stopped or not managed")
	}
	if !imageDigestPattern.MatchString(state.Config.Image) {
		return aiGPUContainer{}, errors.New("AI container image must be an immutable digest")
	}
	expectedData := filepath.Clean(filepath.Join(moduleDataRoot, aiManagedID))
	hasData := false
	for _, mount := range state.Mounts {
		if mount.Destination == "/data" && mount.Type == "bind" && filepath.Clean(mount.Source) == expectedData {
			hasData = true
		}
	}
	if !hasData {
		return aiGPUContainer{}, errors.New("AI persistent data mount does not match managed storage")
	}
	return state, nil
}

func aiGPUStatus(ctx context.Context) ([]string, error) {
	binary, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.New("Docker unavailable")
	}
	state, err := inspectAIContainer(ctx, binary)
	if err != nil {
		return nil, err
	}
	values := strings.TrimSpace(state.Config.Labels["home-ai.ai.gpu.pci"])
	if values == "" {
		return []string{}, nil
	}
	return validateAIGPUPCI(strings.Split(values, ","))
}

func ensureAICandidateNamesFree(ctx context.Context, dockerPath string) error {
	for _, name := range []string{aiGPUCandidate, aiGPUPrevious} {
		output, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{.Id}}", name).CombinedOutput()
		if err == nil {
			return fmt.Errorf("GPU reconfiguration requires recovery of existing container %q", name)
		}
		if !strings.Contains(strings.ToLower(string(output)), "no such") {
			return errors.New("unable to verify free GPU candidate container names")
		}
	}
	return nil
}

// A staged CPU/GPU mode switch preserves the exact image and mounted /data.
// Before stopping the working AI, create and validate a candidate container;
// on failure, restart the original and preserve persistent data. No
// privileged socket is exposed to the AI container or the browser.
func applyAIGPUSelection(ctx context.Context, requested []string, serviceUID, serviceGID int) ([]string, error) {
	if !moduleOperationMu.TryLock() {
		return nil, errors.New("another module operation is in progress")
	}
	defer moduleOperationMu.Unlock()
	selected, err := validateAIGPUPCI(requested)
	if err != nil {
		return nil, err
	}
	ids, err := mapAIGPUsToUUIDs(ctx, selected)
	if err != nil {
		return nil, err
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.New("Docker unavailable")
	}
	state, err := inspectAIContainer(ctx, dockerPath)
	if err != nil {
		return nil, err
	}
	if err := ensureAICandidateNamesFree(ctx, dockerPath); err != nil {
		return nil, err
	}
	if current := strings.TrimSpace(state.Config.Labels["home-ai.ai.gpu.pci"]); current == strings.Join(selected, ",") {
		return selected, nil
	}
	targetImage, err := aiImageForDeviceSelection(state.Config.Image, len(ids))
	if err != nil {
		return nil, err
	}
	// In 0.3.0 the expensive GPU image is downloaded separately on explicit
	// approval. Fail BEFORE touching the working container if it is missing.
	if targetImage != state.Config.Image {
		cached, imageErr := aiPinnedImageCached(ctx, dockerPath, targetImage)
		if imageErr != nil {
			return nil, imageErr
		}
		if !cached {
			return nil, errors.New("AI runtime image is not downloaded; prepare optional GPU components before applying")
		}
	}
	name := moduleContainerName(aiManagedID)
	data := filepath.Clean(filepath.Join(moduleDataRoot, aiManagedID))
	args := []string{
		"create", "--name", aiGPUCandidate,
		"--label", "home-ai.managed=true",
		"--label", "home-ai.module.id=" + aiManagedID,
		"--label", "home-ai.ai.gpu.pci=" + strings.Join(selected, ","),
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", serviceUID, serviceGID),
		"--restart", "unless-stopped",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src=" + data + ",dst=/data",
	}
	if len(ids) > 0 {
		// Docker CLI's CSV parser requires the quoted device=... value when
		// a comma-separated list of UUIDs is supplied.
		args = append(args,
			"--gpus", "\"device="+strings.Join(ids, ",")+"\"",
			"--env", "CUDA_VISIBLE_DEVICES="+strings.Join(ids, ","),
			"--env", "NVIDIA_DRIVER_CAPABILITIES=compute,utility",
		)
	}
	args = append(args, targetImage)
	if err := dockerAICommand(ctx, dockerPath, args...); err != nil {
		return nil, err // Existing AI has not been stopped.
	}
	// From this point, always attempt compensation even if the HTTP client
	// disconnects. Cancelling halfway through the switch can strand an AI.
	critical, cancel := context.WithTimeout(context.WithoutCancel(ctx), 125*time.Second)
	defer cancel()
	rollback := func(cause error, oldRenamed bool) error {
		recovery, done := context.WithTimeout(context.Background(), 40*time.Second)
		defer done()
		var errs []error
		if err := dockerAICommand(recovery, dockerPath, "rm", "-f", aiGPUCandidate); err != nil {
			errs = append(errs, err)
		}
		if oldRenamed {
			if err := dockerAICommand(recovery, dockerPath, "rename", aiGPUPrevious, name); err != nil {
				errs = append(errs, err)
			}
		}
		if err := dockerAICommand(recovery, dockerPath, "start", name); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(append([]error{cause}, errs...)...)
	}
	if err := dockerAICommand(critical, dockerPath, "stop", name); err != nil {
		_ = dockerAICommand(critical, dockerPath, "rm", "-f", aiGPUCandidate)
		return nil, err
	}
	if err := dockerAICommand(critical, dockerPath, "start", aiGPUCandidate); err != nil {
		return nil, rollback(err, false)
	}
	healthy := false
	deadline := time.Now().Add(aiGPUHealthWindow)
	for time.Now().Before(deadline) && critical.Err() == nil {
		output, err := exec.CommandContext(critical, dockerPath, "inspect", "--format",
			"{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", aiGPUCandidate).CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) == "running|healthy" {
			healthy = true
			break
		}
		if err == nil && strings.Contains(string(output), "|unhealthy") {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !healthy {
		return nil, rollback(errors.New("candidate AI container did not become healthy"), false)
	}
	if len(ids) > 0 {
		output, err := exec.CommandContext(critical, dockerPath, "exec", aiGPUCandidate,
			"nvidia-smi", "--query-gpu=uuid", "--format=csv,noheader").CombinedOutput()
		if err != nil {
			return nil, rollback(fmt.Errorf("GPU not accessible inside AI container: %s", commandError(output, err)), false)
		}
		for _, id := range ids {
			if !strings.Contains(string(output), id) {
				return nil, rollback(errors.New("selected GPU not visible in candidate container"), false)
			}
		}
	}
	if err := dockerAICommand(critical, dockerPath, "rename", name, aiGPUPrevious); err != nil {
		return nil, rollback(err, false)
	}
	if err := dockerAICommand(critical, dockerPath, "rename", aiGPUCandidate, name); err != nil {
		return nil, rollback(err, true)
	}
	// A failed backup cleanup is not a reason to discard a healthy newly
	// enabled AI container. The next reconfigure refuses the leftover backup.
	cleanup, done := context.WithTimeout(context.Background(), 20*time.Second)
	defer done()
	_ = dockerAICommand(cleanup, dockerPath, "rm", aiGPUPrevious)
	return selected, nil
}
