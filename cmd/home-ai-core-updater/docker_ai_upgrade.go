package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	aiUpgradeCandidate    = "home-ai-module-ai.agent-upgrade-candidate"
	aiUpgradePrevious     = "home-ai-module-ai.agent-upgrade-previous"
	aiUpgradeJournalName  = ".ai-agent-upgrade-journal.json"
	aiUpgradeBackupName   = ".ai-agent-upgrade-data-backup"
	aiUpgradeCompleteName = ".ai-agent-upgrade-snapshot-complete"
)

type aiUpgradeJournal struct {
	PreviousImage    string `json:"previous_image"`
	NextImage        string `json:"next_image"`
	PreviousManifest string `json:"previous_manifest"`
}

func aiUpgradePath(filename string) string { return filepath.Join(moduleDataRoot, filename) }

// The journal stays OUTSIDE /data, so restoring the snapshot cannot erase
// recovery metadata. Never replace a previous backup or pending journal.
func readAIUpgradeJournal() (aiUpgradeJournal, bool, error) {
	path := aiUpgradePath(aiUpgradeJournalName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return aiUpgradeJournal{}, false, nil
	}
	if err != nil {
		return aiUpgradeJournal{}, false, err
	}
	if len(data) > 32*1024 {
		return aiUpgradeJournal{}, false, errors.New("AI upgrade journal too large")
	}
	var state aiUpgradeJournal
	if err := json.Unmarshal(data, &state); err != nil {
		return aiUpgradeJournal{}, false, err
	}
	if !imageDigestPattern.MatchString(state.PreviousImage) || !imageDigestPattern.MatchString(state.NextImage) ||
		len(state.PreviousManifest) > 16*1024 || state.PreviousManifest == "" {
		return aiUpgradeJournal{}, false, errors.New("invalid AI upgrade recovery journal")
	}
	return state, true, nil
}

func writeAIUpgradeJournal(state aiUpgradeJournal) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(aiUpgradePath(aiUpgradeJournalName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("upgrade journal already exists or cannot be opened: %w", err)
	}
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func aiDockerExists(ctx context.Context, docker, name string) (bool, error) {
	output, err := exec.CommandContext(ctx, docker, "inspect", "--format", "{{.Id}}", name).CombinedOutput()
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no such") {
		return false, nil
	}
	return false, fmt.Errorf("inspect module recovery container: %s", commandError(output, err))
}

func verifyAIUpgradeContainer(ctx context.Context, docker, name, expectedImage string) error {
	exists, managed, err := inspectManagedContainer(ctx, docker, name, aiManagedID)
	if err != nil {
		return err
	}
	if !exists || !managed {
		return errors.New("unmanaged or missing AI upgrade container")
	}
	out, err := exec.CommandContext(ctx, docker, "inspect", "--format", "{{.Config.Image}}", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("verify AI upgrade image: %s", commandError(out, err))
	}
	if strings.TrimSpace(string(out)) != expectedImage {
		return errors.New("AI upgrade container image does not match the recovery journal")
	}
	return nil
}

func aiUpgradeDockerHealth(ctx context.Context, docker, name string, limit time.Duration) error {
	checkCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	for checkCtx.Err() == nil {
		output, err := exec.CommandContext(checkCtx, docker, "inspect", "--format",
			"{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}missing{{end}}", name).CombinedOutput()
		if err == nil {
			state := strings.TrimSpace(string(output))
			if state == "running|healthy" {
				return nil
			}
			if strings.HasPrefix(state, "exited|") || strings.HasSuffix(state, "|unhealthy") || strings.HasSuffix(state, "|missing") {
				return fmt.Errorf("AI replacement failed Docker health: %s", state)
			}
		}
		select {
		case <-checkCtx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("AI replacement health check timed out")
}

func aiUpgradeSnapshot(ctx context.Context) error {
	dataDir := filepath.Join(moduleDataRoot, "ai.agent")
	info, err := os.Lstat(dataDir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("AI data mount is not a physical directory")
	}
	backup := aiUpgradePath(aiUpgradeBackupName)
	for _, path := range []string{backup, aiUpgradePath(aiUpgradeCompleteName)} {
		if _, err := os.Lstat(path); err == nil {
			return errors.New("previous AI upgrade backup requires recovery")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Mkdir(backup, 0o700); err != nil {
		return err
	}
	// Run only after the old AI process is stopped to obtain a consistent
	// SQLite snapshot. Reflinks speed up copies on supporting filesystems.
	// Never hardlink writable database or Ollama model files into backups.
	cmd := exec.CommandContext(ctx, "cp", "-a", "--reflink=auto", dataDir+"/.", backup)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("backup persistent AI data: %s", commandError(output, err))
	}
	marker, err := os.OpenFile(aiUpgradePath(aiUpgradeCompleteName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = marker.WriteString("snapshot complete\n")
	if err == nil {
		err = marker.Sync()
	}
	return errors.Join(err, marker.Close())
}

func aiUpgradeSnapshotReady() (bool, error) {
	_, err := os.Lstat(aiUpgradePath(aiUpgradeCompleteName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(aiUpgradePath(aiUpgradeBackupName))
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("unsafe AI recovery backup")
	}
	return true, nil
}

// Rollback is idempotent. It never destroys the previous managed container,
// and never considers a partially copied snapshot to be recoverable.
func rollbackAIUpgrade(ctx context.Context, docker string, state aiUpgradeJournal) error {
	name := moduleContainerName(aiManagedID)
	previous, err := aiDockerExists(ctx, docker, aiUpgradePrevious)
	if err != nil {
		return err
	}
	current, err := aiDockerExists(ctx, docker, name)
	if err != nil {
		return err
	}
	if previous {
		if err := verifyAIUpgradeContainer(ctx, docker, aiUpgradePrevious, state.PreviousImage); err != nil {
			return err
		}
		if current {
			if err := verifyAIUpgradeContainer(ctx, docker, name, state.NextImage); err != nil {
				return err
			}
			if err := dockerAICommand(ctx, docker, "rm", "-f", name); err != nil {
				return err
			}
		}
		if err := dockerAICommand(ctx, docker, "rename", aiUpgradePrevious, name); err != nil {
			return err
		}
	} else if !current {
		return errors.New("old AI container missing; refusing to discard recovery snapshot")
	} else if err := verifyAIUpgradeContainer(ctx, docker, name, state.PreviousImage); err != nil {
		return err
	}
	candidate, err := aiDockerExists(ctx, docker, aiUpgradeCandidate)
	if err != nil {
		return err
	}
	if candidate {
		if err := verifyAIUpgradeContainer(ctx, docker, aiUpgradeCandidate, state.NextImage); err != nil {
			return err
		}
		if err := dockerAICommand(ctx, docker, "rm", "-f", aiUpgradeCandidate); err != nil {
			return err
		}
	}
	ready, err := aiUpgradeSnapshotReady()
	if err != nil {
		return err
	}
	if ready {
		dataDir := filepath.Join(moduleDataRoot, aiManagedID)
		if err := os.RemoveAll(dataDir); err != nil {
			return err
		}
		if err := os.Rename(aiUpgradePath(aiUpgradeBackupName), dataDir); err != nil {
			return err
		}
		if err := os.Remove(aiUpgradePath(aiUpgradeCompleteName)); err != nil {
			return err
		}
	} else {
		// No complete snapshot means the candidate could not have started.
		// Any partial backup is retained only until the old container starts.
	}
	if err := dockerAICommand(ctx, docker, "start", name); err != nil {
		// start on an already-running old container is harmless in Docker.
		return err
	}
	if err := aiUpgradeDockerHealth(ctx, docker, name, 95*time.Second); err != nil {
		return err
	}
	if err := os.RemoveAll(aiUpgradePath(aiUpgradeBackupName)); err != nil {
		return err
	}
	return os.Remove(aiUpgradePath(aiUpgradeJournalName))
}

func finalizeAIUpgrade(ctx context.Context, docker string, state aiUpgradeJournal) error {
	exists, managed, err := inspectManagedContainer(ctx, docker, moduleContainerName(aiManagedID), aiManagedID)
	if err != nil {
		return err
	}
	if !exists || !managed {
		return errors.New("AI replacement is not a managed container")
	}
	output, err := exec.CommandContext(ctx, docker, "inspect", "--format", "{{.Config.Image}}", moduleContainerName(aiManagedID)).CombinedOutput()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != state.NextImage {
		return errors.New("AI upgrade cannot finalize unexpected image")
	}
	if err := aiUpgradeDockerHealth(ctx, docker, moduleContainerName(aiManagedID), 15*time.Second); err != nil {
		return err
	}
	previous, err := aiDockerExists(ctx, docker, aiUpgradePrevious)
	if err != nil {
		return err
	}
	if previous {
		if err := verifyAIUpgradeContainer(ctx, docker, aiUpgradePrevious, state.PreviousImage); err != nil {
			return err
		}
		if err := dockerAICommand(ctx, docker, "rm", "-f", aiUpgradePrevious); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(aiUpgradePath(aiUpgradeBackupName)); err != nil {
		return err
	}
	if err := os.Remove(aiUpgradePath(aiUpgradeCompleteName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Remove(aiUpgradePath(aiUpgradeJournalName))
}

// The registry's persisted image is authoritative after an unexpected restart.
// Old version in DB => restore old container and data. New image in DB =>
// finalize only a healthy replacement, otherwise restore and tell Core to
// persist the previous manifest from the write-ahead journal.
func reconcileAIUpgrade(ctx context.Context, registryImage string) (moduleContainerStatus, error) {
	state, exists, err := readAIUpgradeJournal()
	if err != nil {
		return moduleContainerStatus{}, err
	}
	if !exists {
		return moduleContainerStatus{State: "none"}, nil
	}
	if registryImage != "" && registryImage != state.PreviousImage && registryImage != state.NextImage {
		return moduleContainerStatus{}, errors.New("registry image differs from both journal versions; recovery refused")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, err
	}
	if registryImage == state.NextImage {
		if err := finalizeAIUpgrade(ctx, docker, state); err == nil {
			return moduleContainerStatus{State: "committed"}, nil
		}
	}
	if err := rollbackAIUpgrade(ctx, docker, state); err != nil {
		return moduleContainerStatus{}, err
	}
	return moduleContainerStatus{State: "rolled_back", PreviousManifest: state.PreviousManifest}, nil
}

func upgradeDockerAI(ctx context.Context, newImage, previousImage, previousManifest string, uid, gid int, report moduleProgressFunc) (status moduleContainerStatus, message string, resultErr error) {
	if !imageDigestPattern.MatchString(newImage) || !imageDigestPattern.MatchString(previousImage) || newImage == previousImage {
		return moduleContainerStatus{}, "", errors.New("AI upgrade must change immutable OCI image")
	}
	if len(previousManifest) == 0 || len(previousManifest) > 16*1024 || !json.Valid([]byte(previousManifest)) {
		return moduleContainerStatus{}, "", errors.New("previous module manifest required for recovery")
	}
	if _, exists, err := readAIUpgradeJournal(); err != nil {
		return moduleContainerStatus{}, "", err
	} else if exists {
		return moduleContainerStatus{}, "", errors.New("interrupted upgrade must be reconciled before another update")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	old, err := inspectAIContainer(ctx, docker)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if old.Config.Image != previousImage {
		return moduleContainerStatus{}, "", errors.New("actual AI image does not match Core registry; refusing upgrade")
	}
	if old.Config.Labels["home-ai.ai.gpu.pci"] != "" {
		return moduleContainerStatus{}, "", errors.New("switch AI to CPU before updating its image")
	}
	for _, name := range []string{aiUpgradeCandidate, aiUpgradePrevious} {
		found, err := aiDockerExists(ctx, docker, name)
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		if found {
			return moduleContainerStatus{}, "", errors.New("previous AI upgrade container requires recovery")
		}
	}
	report("download", "")
	if err := pullDockerModuleImage(ctx, docker, newImage, report); err != nil {
		return moduleContainerStatus{}, "", err
	}
	args := []string{
		"create", "--name", aiUpgradeCandidate,
		"--label", "home-ai.managed=true", "--label", "home-ai.module.id=" + aiManagedID,
		"--network", dockerNetworkName, "--user", fmt.Sprintf("%d:%d", uid, gid),
		"--restart", "no", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "512", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", "type=bind,src=" + filepath.Join(moduleDataRoot, aiManagedID) + ",dst=/data", newImage,
	}
	report("create", "")
	if err := dockerAICommand(ctx, docker, args...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	journal := aiUpgradeJournal{PreviousImage: previousImage, NextImage: newImage, PreviousManifest: previousManifest}
	if err := writeAIUpgradeJournal(journal); err != nil {
		_ = dockerAICommand(context.Background(), docker, "rm", "-f", aiUpgradeCandidate)
		return moduleContainerStatus{}, "", err
	}
	// Failures and cancellations after journal creation always attempt
	// rollback independently of the disconnected HTTP client.
	success := false
	defer func() {
		if success {
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		if err := rollbackAIUpgrade(recovery, docker, journal); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("AI rollback needs operator recovery: %w", err))
		}
	}()
	report("backup", "")
	if err := dockerAICommand(ctx, docker, "stop", moduleContainerName(aiManagedID)); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := aiUpgradeSnapshot(ctx); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("start", "")
	if err := dockerAICommand(ctx, docker, "start", aiUpgradeCandidate); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("verify", "")
	if err := aiUpgradeDockerHealth(ctx, docker, aiUpgradeCandidate, 180*time.Second); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := dockerAICommand(ctx, docker, "stop", aiUpgradeCandidate); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := dockerAICommand(ctx, docker, "rename", moduleContainerName(aiManagedID), aiUpgradePrevious); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := dockerAICommand(ctx, docker, "rename", aiUpgradeCandidate, moduleContainerName(aiManagedID)); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := dockerAICommand(ctx, docker, "update", "--restart", "unless-stopped", moduleContainerName(aiManagedID)); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := dockerAICommand(ctx, docker, "start", moduleContainerName(aiManagedID)); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := aiUpgradeDockerHealth(ctx, docker, moduleContainerName(aiManagedID), 180*time.Second); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("verify", "")
	success = true
	return moduleContainerStatus{State: "running"}, "AI replacement healthy; awaiting registry commit", nil
}
