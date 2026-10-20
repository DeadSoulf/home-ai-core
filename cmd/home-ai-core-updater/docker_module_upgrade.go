package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

// Staging names are scoped to the module and never collide with the AI
// GPU-switch staging containers. Staging containers are never reused.
const moduleUpgradeHealthWindow = 100 * time.Second

func moduleUpgradeStageNames(moduleID string) (candidate, previous string) {
	name := moduleContainerName(moduleID)
	return name + "-upgrade-candidate", name + "-upgrade-previous"
}

func upgradeDockerModule(ctx context.Context, request updaterhelper.Request, uid, gid int, report moduleProgressFunc) (moduleContainerStatus, string, error) {
	if request.ModuleID == homeassistant.ModuleID {
		return upgradeHomeAssistant(ctx, request, report)
	}
	moduleID := request.ModuleID
	if moduleID == "frigate.module" {
		return upgradeFrigate(ctx, request, report)
	}
	if !moduleIDPattern.MatchString(moduleID) || len(moduleID) > 96 {
		return moduleContainerStatus{}, "", errors.New("invalid module id")
	}
	candidateName, previousName := moduleUpgradeStageNames(moduleID)
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", errors.New("Docker CLI unavailable")
	}
	name := moduleContainerName(moduleID)

	// Commit/rollback must verify the exact old and new immutable images, not
	// just container names. These operations never download or install images.
	if request.Operation == "docker.module.upgrade.commit" || request.Operation == "docker.module.upgrade.rollback" {
		if err := verifyUpgradePair(ctx, binary, moduleID, name, previousName, request.Image, request.PreviousImage); err != nil {
			return moduleContainerStatus{}, "", err
		}
		if request.Operation == "docker.module.upgrade.commit" {
			if err := dockerAICommand(ctx, binary, "rm", previousName); err != nil {
				return moduleContainerStatus{}, "", err
			}
			status, err := inspectModuleContainer(ctx, binary, name)
			return status, "previous module container removed after registry commit", err
		}
		result, err := rollbackModuleUpgrade(ctx, binary, name, previousName)
		return result, "previous module container restored", err
	}

	if request.Image == request.PreviousImage {
		return moduleContainerStatus{}, "", errors.New("candidate image is identical to the running image")
	}
	if err := verifyModuleUpgradeSource(ctx, binary, moduleID, name, request.PreviousImage); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := ensureModuleUpgradeNamesFree(ctx, binary, candidateName, previousName); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("download", "")
	if err := pullDockerModuleImage(ctx, binary, request.Image, report); err != nil {
		return moduleContainerStatus{}, "", fmt.Errorf("pull upgraded module image: %w", err)
	}
	// The candidate is created, not started, while the old AI process owns
	// the persistent /data directory. The old container remains untouched.
	dataDir := filepath.Clean(filepath.Join(moduleDataRoot, moduleID))
	args := []string{
		"create", "--name", candidateName,
		"--label", "home-ai.managed=true",
		"--label", "home-ai.module.id=" + moduleID,
		"--network", dockerNetworkName,
		"--user", fmt.Sprintf("%d:%d", uid, gid),
		"--restart", "unless-stopped",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--pids-limit", "512",
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src=" + dataDir + ",dst=/data",
		request.Image,
	}
	report("create", "")
	if err := dockerAICommand(ctx, binary, args...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	// Verify the candidate's /data source before stopping the running module.
	if err := verifyModuleUpgradeDataMount(ctx, binary, moduleID, candidateName); err != nil {
		cleanup, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		_ = dockerAICommand(cleanup, binary, "rm", "-f", candidateName)
		return moduleContainerStatus{}, "", err
	}
	// After Docker stop, compensation must complete even when a browser or
	// job subscriber disconnects halfway through the cut-over.
	critical, done := context.WithTimeout(context.WithoutCancel(ctx), 160*time.Second)
	defer done()
	if err := ctx.Err(); err != nil {
		_ = dockerAICommand(critical, binary, "rm", "-f", candidateName)
		return moduleContainerStatus{}, "", err
	}
	report("start", "")
	if err := dockerAICommand(critical, binary, "stop", name); err != nil {
		_ = dockerAICommand(critical, binary, "rm", "-f", candidateName)
		return moduleContainerStatus{}, "", err
	}
	recoverOld := func(cause error, renamed bool) error {
		recovery, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		var failures []error
		if err := dockerAICommand(recovery, binary, "rm", "-f", candidateName); err != nil {
			failures = append(failures, err)
		}
		if renamed {
			if err := dockerAICommand(recovery, binary, "rename", previousName, name); err != nil {
				failures = append(failures, err)
			}
		}
		if err := dockerAICommand(recovery, binary, "start", name); err != nil {
			failures = append(failures, err)
		}
		return errors.Join(append([]error{cause}, failures...)...)
	}
	if err := dockerAICommand(critical, binary, "start", candidateName); err != nil {
		return moduleContainerStatus{}, "", recoverOld(err, false)
	}
	report("verify", "")
	if err := waitModuleUpgradeHealthy(critical, binary, candidateName); err != nil {
		return moduleContainerStatus{}, "", recoverOld(err, false)
	}
	if err := dockerAICommand(critical, binary, "rename", name, previousName); err != nil {
		return moduleContainerStatus{}, "", recoverOld(err, false)
	}
	if err := dockerAICommand(critical, binary, "rename", candidateName, name); err != nil {
		return moduleContainerStatus{}, "", recoverOld(err, true)
	}
	// The stopped previous container is intentionally retained until the Core
	// SQLite manifest write succeeds. Commit and rollback are separate calls.
	status, err := inspectModuleContainer(critical, binary, name)
	if err != nil || status.State != "running" {
		if err == nil {
			err = errors.New("upgraded module container is not running after the swap")
		}
		recovery, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		_, rollbackErr := rollbackModuleUpgrade(recovery, binary, name, previousName)
		return moduleContainerStatus{}, "", errors.Join(err, rollbackErr)
	}
	return status, "module candidate healthy; previous container retained pending registry commit", nil
}

func ensureModuleUpgradeNamesFree(ctx context.Context, binary, candidateName, previousName string) error {
	for _, name := range []string{candidateName, previousName} {
		output, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Id}}", name).CombinedOutput()
		if err == nil {
			return fmt.Errorf("module upgrade requires recovery of existing staging container %q", name)
		}
		if !strings.Contains(strings.ToLower(string(output)), "no such") {
			return fmt.Errorf("unable to verify upgrade staging name %q is free", name)
		}
	}
	return nil
}

func verifyUpgradePair(ctx context.Context, binary, moduleID, name, previousName, newImage, oldImage string) error {
	for _, pair := range []struct{ name, image string }{{name, newImage}, {previousName, oldImage}} {
		exists, managed, err := inspectManagedContainer(ctx, binary, pair.name, moduleID)
		if err != nil {
			return err
		}
		if !exists || !managed {
			return fmt.Errorf("expected managed module container %q not found", pair.name)
		}
		out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Config.Image}}", pair.name).Output()
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(out)) != pair.image {
			return fmt.Errorf("container %q image does not match the upgrade transaction", pair.name)
		}
		if err := verifyModuleUpgradeDataMount(ctx, binary, moduleID, pair.name); err != nil {
			return err
		}
	}
	return nil
}

func waitModuleUpgradeHealthy(ctx context.Context, binary, name string) error {
	deadline := time.Now().Add(moduleUpgradeHealthWindow)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name).CombinedOutput()
		status := strings.TrimSpace(string(out))
		if err == nil && status == "running|healthy" {
			return nil
		}
		if err == nil && strings.Contains(status, "|unhealthy") {
			return errors.New("new module container reported unhealthy")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("new module container did not become healthy")
}

func rollbackModuleUpgrade(ctx context.Context, binary, name, previousName string) (moduleContainerStatus, error) {
	// The caller verified both managed images before destructive actions.
	if err := dockerAICommand(ctx, binary, "stop", name); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := dockerAICommand(ctx, binary, "rm", name); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := dockerAICommand(ctx, binary, "rename", previousName, name); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := dockerAICommand(ctx, binary, "start", name); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := waitModuleUpgradeHealthy(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, err
	}
	return inspectModuleContainer(ctx, binary, name)
}

// verifyModuleUpgradeSource prevents a privileged switch away from an
// unrecognized container, local-source fallback, modified data mount or GPU.
func verifyModuleUpgradeSource(ctx context.Context, binary, moduleID, name, registeredImage string) error {
	exists, managed, err := inspectManagedContainer(ctx, binary, name, moduleID)
	if err != nil {
		return err
	}
	if !exists || !managed {
		return fmt.Errorf("module %q has no managed running container", moduleID)
	}
	if moduleID == aiManagedID || moduleID == "nvr.module" {
		if _, err := inspectManagedModuleAddress(ctx, moduleID); err != nil {
			return fmt.Errorf("verify managed module network: %w", err)
		}
	}
	output, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect module for upgrade: %s", commandError(output, err))
	}
	var state aiGPUContainer
	if err := json.Unmarshal(output, &state); err != nil {
		return fmt.Errorf("decode module container: %w", err)
	}
	if !state.State.Running || state.Config.Labels["home-ai.managed"] != "true" ||
		state.Config.Labels["home-ai.module.id"] != moduleID {
		return errors.New("module container is stopped or unmanaged")
	}
	if state.Config.Image != registeredImage {
		return errors.New("running module image differs from registered immutable digest; upgrade refused")
	}
	if moduleID == aiManagedID && strings.TrimSpace(state.Config.Labels["home-ai.ai.gpu.pci"]) != "" {
		return errors.New("AI GPU allocation must be disabled before a CPU image upgrade")
	}
	if err := verifyModuleUpgradeDataMount(ctx, binary, moduleID, name); err != nil {
		return err
	}
	requests, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .HostConfig.DeviceRequests}}", name).Output()
	if err != nil {
		return fmt.Errorf("inspect module device assignments: %w", err)
	}
	if value := strings.TrimSpace(string(requests)); value != "" && value != "null" && value != "[]" {
		return errors.New("module has custom device requests; upgrade refused")
	}
	return nil
}

// Both source and candidate must use the SAME managed bind mount at /data.
// NVR has an assigned-disk symlink; revalidate its physical mount instead of
// assuming the symlink is still attached to the user's recording disk.
func verifyModuleUpgradeDataMount(ctx context.Context, binary, moduleID, name string) error {
	dataDir := filepath.Clean(filepath.Join(moduleDataRoot, moduleID))
	if moduleID == "nvr.module" {
		if _, err := validateNVRDataLink(ctx, dataDir); err != nil {
			return fmt.Errorf("verify assigned NVR disk before upgrade: %w", err)
		}
		return verifyNVRContainerMount(ctx, binary, name, dataDir)
	}
	out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .Mounts}}", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect module data mount: %s", commandError(out, err))
	}
	var mounts []struct {
		Type, Source, Destination string
	}
	if err := json.Unmarshal(out, &mounts); err != nil {
		return fmt.Errorf("decode module mounts: %w", err)
	}
	found := false
	for _, mount := range mounts {
		if mount.Destination != "/data" {
			continue
		}
		if mount.Type != "bind" || filepath.Clean(mount.Source) != dataDir {
			return errors.New("module /data is not the expected managed persistent bind mount")
		}
		found = true
	}
	if !found {
		return errors.New("module /data managed persistent bind mount is missing")
	}
	return nil
}
