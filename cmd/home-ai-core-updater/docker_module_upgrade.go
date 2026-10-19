package main

import (
    "context"
    "errors"
    "fmt"
    "os/exec"
    "path/filepath"
    "strings"
    "time"

    "github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

// These names deliberately differ from the AI GPU-switch staging containers.
// Neither operation may replace a container owned by the other.
const (
    aiUpgradeCandidate = "home-ai-module-ai.agent-upgrade-candidate"
    aiUpgradePrevious = "home-ai-module-ai.agent-upgrade-previous"
    aiUpgradeHealthWindow = 100 * time.Second
)

func upgradeDockerAI(ctx context.Context, request updaterhelper.Request, uid, gid int, report moduleProgressFunc) (moduleContainerStatus, string, error) {
    if request.ModuleID != aiManagedID {
        return moduleContainerStatus{}, "", errors.New("in-place upgrades are currently supported only for ai.agent")
    }
    binary, err := exec.LookPath("docker")
    if err != nil { return moduleContainerStatus{}, "", errors.New("Docker CLI unavailable") }
    name := moduleContainerName(aiManagedID)

    // Commit/rollback must verify the exact old and new immutable images, not
    // just container names. These operations never download or install images.
    if request.Operation == "docker.module.upgrade.commit" || request.Operation == "docker.module.upgrade.rollback" {
        if err := verifyUpgradePair(ctx, binary, name, request.Image, request.PreviousImage); err != nil {
            return moduleContainerStatus{}, "", err
        }
        if request.Operation == "docker.module.upgrade.commit" {
            if err := dockerAICommand(ctx, binary, "rm", aiUpgradePrevious); err != nil { return moduleContainerStatus{}, "", err }
            status, err := inspectModuleContainer(ctx, binary, name)
            return status, "previous AI container removed after registry commit", err
        }
        result, err := rollbackAIUpgrade(ctx, binary, name)
        return result, "previous AI container restored", err
    }

    if request.Image == request.PreviousImage {
        return moduleContainerStatus{}, "", errors.New("candidate image is identical to the running image")
    }
    state, err := inspectAIContainer(ctx, binary)
    if err != nil { return moduleContainerStatus{}, "", err }
    if state.Config.Image != request.PreviousImage {
        return moduleContainerStatus{}, "", errors.New("working AI image does not match its registered immutable digest")
    }
    if strings.TrimSpace(state.Config.Labels["home-ai.ai.gpu.pci"]) != "" {
        return moduleContainerStatus{}, "", errors.New("AI GPU allocation must be disabled before a CPU image upgrade")
    }
    // Custom GPU device requests cannot be silently dropped on replacement.
    deviceRequests, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .HostConfig.DeviceRequests}}", name).Output()
    if err != nil { return moduleContainerStatus{}, "", err }
    if value := strings.TrimSpace(string(deviceRequests)); value != "" && value != "null" && value != "[]" {
        return moduleContainerStatus{}, "", errors.New("AI container has custom device requests; upgrade refused")
    }
    if err := ensureAIUpgradeNamesFree(ctx, binary); err != nil { return moduleContainerStatus{}, "", err }
    report("download", "")
    if err := pullDockerModuleImage(ctx, binary, request.Image, report); err != nil {
        return moduleContainerStatus{}, "", fmt.Errorf("pull upgraded AI image: %w", err)
    }
    // The candidate is created, not started, while the old AI process owns
    // the persistent /data directory. The old container remains untouched.
    dataDir := filepath.Clean(filepath.Join(moduleDataRoot, aiManagedID))
    args := []string{
        "create", "--name", aiUpgradeCandidate,
        "--label", "home-ai.managed=true",
        "--label", "home-ai.module.id="+aiManagedID,
        "--network", dockerNetworkName,
        "--user", fmt.Sprintf("%d:%d", uid, gid),
        "--restart", "unless-stopped",
        "--cap-drop", "ALL",
        "--security-opt", "no-new-privileges",
        "--pids-limit", "512",
        "--read-only",
        "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
        "--mount", "type=bind,src="+dataDir+",dst=/data",
        request.Image,
    }
    report("create", "")
    if err := dockerAICommand(ctx, binary, args...); err != nil {
        return moduleContainerStatus{}, "", err
    }
    // After Docker stop, compensation must complete even when a browser or
    // job subscriber disconnects halfway through the cut-over.
    critical, done := context.WithTimeout(context.WithoutCancel(ctx), 160*time.Second)
    defer done()
    if err := ctx.Err(); err != nil {
        _ = dockerAICommand(critical, binary, "rm", "-f", aiUpgradeCandidate)
        return moduleContainerStatus{}, "", err
    }
    report("start", "")
    if err := dockerAICommand(critical, binary, "stop", name); err != nil {
        _ = dockerAICommand(critical, binary, "rm", "-f", aiUpgradeCandidate)
        return moduleContainerStatus{}, "", err
    }
    recoverOld := func(cause error, renamed bool) error {
        recovery, cancel := context.WithTimeout(context.Background(), 45*time.Second)
        defer cancel()
        var failures []error
        if err := dockerAICommand(recovery, binary, "rm", "-f", aiUpgradeCandidate); err != nil { failures = append(failures, err) }
        if renamed {
            if err := dockerAICommand(recovery, binary, "rename", aiUpgradePrevious, name); err != nil { failures = append(failures, err) }
        }
        if err := dockerAICommand(recovery, binary, "start", name); err != nil { failures = append(failures, err) }
        return errors.Join(append([]error{cause}, failures...)...)
    }
    if err := dockerAICommand(critical, binary, "start", aiUpgradeCandidate); err != nil {
        return moduleContainerStatus{}, "", recoverOld(err, false)
    }
    report("verify", "")
    if err := waitAIUpgradeHealthy(critical, binary, aiUpgradeCandidate); err != nil {
        return moduleContainerStatus{}, "", recoverOld(err, false)
    }
    if err := dockerAICommand(critical, binary, "rename", name, aiUpgradePrevious); err != nil {
        return moduleContainerStatus{}, "", recoverOld(err, false)
    }
    if err := dockerAICommand(critical, binary, "rename", aiUpgradeCandidate, name); err != nil {
        return moduleContainerStatus{}, "", recoverOld(err, true)
    }
    // The stopped previous container is intentionally retained until the Core
    // SQLite manifest write succeeds. Commit and rollback are separate calls.
    status, err := inspectModuleContainer(critical, binary, name)
    if err != nil { return moduleContainerStatus{}, "", fmt.Errorf("new AI active; previous container retained for recovery: %w", err) }
    return status, "AI candidate healthy; previous container retained pending registry commit", nil
}

func ensureAIUpgradeNamesFree(ctx context.Context, binary string) error {
    for _, name := range []string{aiUpgradeCandidate, aiUpgradePrevious} {
        output, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Id}}", name).CombinedOutput()
        if err == nil { return fmt.Errorf("AI upgrade requires recovery of existing staging container %q", name) }
        if !strings.Contains(strings.ToLower(string(output)), "no such") {
            return fmt.Errorf("unable to verify upgrade staging name %q is free", name)
        }
    }
    return nil
}

func verifyUpgradePair(ctx context.Context, binary, name, newImage, oldImage string) error {
    for _, pair := range []struct{name, image string}{{name, newImage}, {aiUpgradePrevious, oldImage}} {
        exists, managed, err := inspectManagedContainer(ctx, binary, pair.name, aiManagedID)
        if err != nil { return err }
        if !exists || !managed { return fmt.Errorf("expected managed AI container %q not found", pair.name) }
        out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Config.Image}}", pair.name).Output()
        if err != nil { return err }
        if strings.TrimSpace(string(out)) != pair.image {
            return fmt.Errorf("container %q image does not match the upgrade transaction", pair.name)
        }
    }
    return nil
}

func waitAIUpgradeHealthy(ctx context.Context, binary, name string) error {
    deadline := time.Now().Add(aiUpgradeHealthWindow)
    for time.Now().Before(deadline) {
        if err := ctx.Err(); err != nil { return err }
        out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name).CombinedOutput()
        status := strings.TrimSpace(string(out))
        if err == nil && status == "running|healthy" { return nil }
        if err == nil && strings.Contains(status, "|unhealthy") { return errors.New("new AI container reported unhealthy") }
        select { case <-ctx.Done(): return ctx.Err(); case <-time.After(2*time.Second): }
    }
    return errors.New("new AI container did not become healthy")
}

func rollbackAIUpgrade(ctx context.Context, binary, name string) (moduleContainerStatus, error) {
    // The caller verified both managed images before destructive actions.
    if err := dockerAICommand(ctx, binary, "stop", name); err != nil { return moduleContainerStatus{}, err }
    if err := dockerAICommand(ctx, binary, "rm", name); err != nil { return moduleContainerStatus{}, err }
    if err := dockerAICommand(ctx, binary, "rename", aiUpgradePrevious, name); err != nil { return moduleContainerStatus{}, err }
    if err := dockerAICommand(ctx, binary, "start", name); err != nil { return moduleContainerStatus{}, err }
    if err := waitAIUpgradeHealthy(ctx, binary, name); err != nil { return moduleContainerStatus{}, err }
    return inspectModuleContainer(ctx, binary, name)
}
