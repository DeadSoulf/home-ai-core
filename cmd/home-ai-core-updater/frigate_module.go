package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/frigate"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"golang.org/x/sys/unix"
)

const frigateNetwork = "home-ai-frigate"
const frigateSnapshotLimit int64 = 8 << 30

var frigateHealthWindow = 330 * time.Second
var frigateHealthInterval = 2 * time.Second

func frigateRoot() string        { return filepath.Join(moduleDataRoot, frigate.ModuleID) }
func frigateJournalPath() string { return filepath.Join(frigateRoot(), "upgrade.json") }
func frigateBackupPath() string  { return filepath.Join(frigateRoot(), "upgrade-backup") }

type frigateTransaction struct {
	OldImage string           `json:"old_image"`
	NewImage string           `json:"new_image"`
	Phase    string           `json:"phase"`
	Snapshot frigate.Snapshot `json:"snapshot,omitempty"`
}

func validateFrigateRequest(request updaterhelper.Request) error {
	if request.ModuleID != frigate.ModuleID {
		return nil
	}
	switch request.Operation {
	case "docker.module.install":
		if !frigate.ValidImage(request.Image) {
			return errors.New("Frigate only accepts the official immutable GHCR image")
		}
		txn, err := readFrigateTransaction()
		if err != nil {
			return err
		}
		if txn != nil && (txn.Phase != "committed" || txn.NewImage != request.Image) {
			return errors.New("retained Frigate data or a pending update requires recovery before installation; do not start another image on migrated data")
		}
	case "docker.module.upgrade", "docker.module.upgrade.commit", "docker.module.upgrade.rollback":
		if !frigate.ValidImage(request.Image) || !frigate.ValidImage(request.PreviousImage) {
			return errors.New("Frigate upgrade requires two official immutable GHCR images")
		}
	case "docker.module.start", "docker.module.restart", "docker.module.remove":
		txn, err := readFrigateTransaction()
		if err != nil {
			return err
		}
		if txn != nil && txn.Phase != "committed" {
			return errors.New("Frigate upgrade needs recovery; retry the approved upgrade before lifecycle changes")
		}
	}
	return nil
}

func ensureFrigateDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Frigate path must be a real directory: %s", path)
	}
	return os.Chmod(path, 0700)
}
func prepareFrigateStorage() error {
	info, err := os.Lstat(moduleDataRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("untrusted module storage root")
	}
	for _, path := range []string{frigateRoot(), filepath.Join(frigateRoot(), "config"), filepath.Join(frigateRoot(), "media")} {
		if err := ensureFrigateDirectory(path); err != nil {
			return err
		}
	}
	// Never overwrite a user's existing configuration, including config.yaml.
	for _, name := range []string{"config.yml", "config.yaml"} {
		info, err := os.Lstat(filepath.Join(frigateRoot(), "config", name))
		if err == nil {
			if !info.Mode().IsRegular() {
				return errors.New("Frigate configuration must be a regular file")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	file, err := os.OpenFile(filepath.Join(frigateRoot(), "config", "config.yml"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString("mqtt:\n  enabled: false\ncameras: {}\ndetect:\n  enabled: false\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
func ensureFrigateNetwork(ctx context.Context, binary string) error {
	out, err := exec.CommandContext(ctx, binary, "network", "inspect", "--format", `{{index .Labels "home-ai.managed"}}|{{index .Labels "home-ai.frigate"}}`, frigateNetwork).CombinedOutput()
	if err == nil {
		if strings.TrimSpace(string(out)) != "true|true" {
			return errors.New("Frigate network name is owned by another workload")
		}
		return nil
	}
	message := strings.ToLower(string(out))
	if !strings.Contains(message, "no such") && !strings.Contains(message, "network "+frigateNetwork+" not found") {
		return fmt.Errorf("inspect Frigate network: %s", commandError(out, err))
	}
	return dockerAICommand(ctx, binary, "network", "create", "--driver", "bridge", "--label", "home-ai.managed=true", "--label", "home-ai.frigate=true", frigateNetwork)
}
func frigateCreateArgs(name, image string) []string {
	return []string{"create", "--name", name, "--label", "home-ai.managed=true", "--label", "home-ai.module.id=" + frigate.ModuleID,
		"--label", "home-ai.frigate.profile=1", "--network", frigateNetwork, "--restart", "unless-stopped",
		// The upstream s6 bootstrap requires root and a writable image layer. This
		// exception is fixed to frigate.module and does not weaken the generic SDK.
		"--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "FOWNER", "--cap-add", "SETUID", "--cap-add", "SETGID", "--cap-add", "DAC_OVERRIDE", "--cap-add", "KILL",
		"--security-opt", "no-new-privileges", "--pids-limit", "2048", "--shm-size", "256m", "--stop-timeout", "60",
		"--tmpfs", "/tmp/cache:rw,noexec,nosuid,size=256m",
		"--mount", "type=bind,src=" + filepath.Join(frigateRoot(), "config") + ",dst=/config",
		"--mount", "type=bind,src=" + filepath.Join(frigateRoot(), "media") + ",dst=/media/frigate", image}
}
func pullFrigateImage(ctx context.Context, binary, image string, report moduleProgressFunc) error {
	if !frigate.ValidImage(image) {
		return errors.New("untrusted Frigate image")
	}
	authDir, err := os.MkdirTemp("", "home-ai-frigate-pull-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(authDir)
	// Official public images never depend on or receive private catalog credentials.
	if err := pullDockerImageWithProgress(ctx, binary, append(os.Environ(), "DOCKER_CONFIG="+authDir), image, report); err != nil {
		return err
	}
	out, err := exec.CommandContext(ctx, binary, "image", "inspect", "--format", "{{.Os}}|{{.Architecture}}", image).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect Frigate image: %s", commandError(out, err))
	}
	if strings.TrimSpace(string(out)) != "linux|"+runtime.GOARCH {
		return errors.New("Frigate image architecture does not match this host")
	}
	return nil
}

type frigateContainer struct {
	ID     string `json:"Id"`
	Config struct {
		Image  string
		User   string
		Labels map[string]string
	}
	State struct {
		Running bool
		Status  string
		Health  *struct{ Status string }
	}
	HostConfig struct {
		Privileged     bool
		NetworkMode    string
		PortBindings   map[string]any
		Devices        []any
		DeviceRequests []any
	}
	Mounts []struct {
		Type, Source, Destination string
		RW                        bool
	}
}

func verifyFrigateContainer(ctx context.Context, binary, name, image string) (frigateContainer, error) {
	var info frigateContainer
	out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{json .}}", name).CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("inspect Frigate: %s", commandError(out, err))
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return info, err
	}
	if info.Config.Image != image || !frigate.ValidImage(info.Config.Image) || info.Config.Labels["home-ai.managed"] != "true" || info.Config.Labels["home-ai.module.id"] != frigate.ModuleID || info.Config.Labels["home-ai.frigate.profile"] != "1" {
		return info, errors.New("Frigate container identity or immutable image mismatch")
	}
	if info.HostConfig.Privileged || info.HostConfig.NetworkMode != frigateNetwork || len(info.HostConfig.PortBindings) != 0 || len(info.HostConfig.Devices) != 0 || len(info.HostConfig.DeviceRequests) != 0 {
		return info, errors.New("Frigate container has an unreviewed runtime profile")
	}
	expected := map[string]string{"/config": filepath.Join(frigateRoot(), "config"), "/media/frigate": filepath.Join(frigateRoot(), "media")}
	seen := map[string]bool{}
	for _, mount := range info.Mounts {
		source, ok := expected[mount.Destination]
		if !ok {
			if mount.Type != "tmpfs" {
				return info, errors.New("Frigate has an unexpected persistent mount")
			}
			continue
		}
		if mount.Type != "bind" || filepath.Clean(mount.Source) != source || !mount.RW {
			return info, errors.New("Frigate storage mount does not match managed storage")
		}
		seen[mount.Destination] = true
	}
	if len(seen) != len(expected) {
		return info, errors.New("Frigate managed configuration/media mount is missing")
	}
	return info, nil
}
func waitFrigateHealthy(ctx context.Context, binary, name string) error {
	deadline := time.NewTimer(frigateHealthWindow)
	defer deadline.Stop()
	for {
		out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name).CombinedOutput()
		value := strings.TrimSpace(string(out))
		if err == nil && value == "running|healthy" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect Frigate startup: %s", commandError(out, err))
		}
		if strings.Contains(value, "unhealthy") || strings.HasPrefix(value, "exited|") || strings.HasPrefix(value, "dead|") {
			return fmt.Errorf("Frigate candidate failed health check: %s", value)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Frigate health check timed out")
		case <-time.After(frigateHealthInterval):
		}
	}
}
func installFrigate(ctx context.Context, image string, report moduleProgressFunc) (moduleContainerStatus, string, error) {
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName(frigate.ModuleID)
	exists, managed, err := inspectManagedContainer(ctx, binary, name, frigate.ModuleID)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if exists {
		if !managed {
			return moduleContainerStatus{}, "", errors.New("Frigate container name is already in use")
		}
		if _, err := verifyFrigateContainer(ctx, binary, name, image); err != nil {
			return moduleContainerStatus{}, "", err
		}
		if err := dockerAICommand(ctx, binary, "start", name); err != nil {
			return moduleContainerStatus{}, "", err
		}
		if err := waitFrigateHealthy(ctx, binary, name); err != nil {
			return moduleContainerStatus{}, "", err
		}
		state, err := inspectModuleContainer(ctx, binary, name)
		return state, "same-image Frigate installation recovered", err
	}
	report("download", "")
	if err := pullFrigateImage(ctx, binary, image, report); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("storage", "")
	if err := prepareFrigateStorage(); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := ensureFrigateNetwork(ctx, binary); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("create", "")
	if err := dockerAICommand(ctx, binary, frigateCreateArgs(name, image)...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if _, err := verifyFrigateContainer(ctx, binary, name, image); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(binary, name, err)
	}
	report("start", "")
	if err := dockerAICommand(ctx, binary, "start", name); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(binary, name, err)
	}
	report("verify", "")
	if err := waitFrigateHealthy(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(binary, name, err)
	}
	state, err := inspectModuleContainer(ctx, binary, name)
	return state, "official Frigate installed and healthy; no host ports published", err
}

func readFrigateTransaction() (*frigateTransaction, error) {
	raw, err := os.ReadFile(frigateJournalPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 16<<20 {
		return nil, errors.New("Frigate upgrade journal is oversized")
	}
	var txn frigateTransaction
	if err := json.Unmarshal(raw, &txn); err != nil {
		return nil, err
	}
	if !frigate.ValidImage(txn.OldImage) || !frigate.ValidImage(txn.NewImage) || txn.OldImage == txn.NewImage {
		return nil, errors.New("invalid Frigate upgrade journal images")
	}
	switch txn.Phase {
	case "prepared", "snapshotted", "swapped", "committed":
	default:
		return nil, errors.New("invalid Frigate upgrade journal phase")
	}
	if txn.Phase != "prepared" && txn.Snapshot == nil {
		return nil, errors.New("Frigate upgrade journal is missing its verified backup")
	}
	return &txn, nil
}
func writeFrigateTransaction(txn *frigateTransaction) error {
	file, err := os.CreateTemp(frigateRoot(), ".upgrade-*.tmp")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	raw, marshalErr := json.Marshal(txn)
	if marshalErr != nil {
		_ = file.Close()
		return marshalErr
	}
	if len(raw) > 16<<20 {
		_ = file.Close()
		return errors.New("Frigate snapshot journal exceeds 16 MiB")
	}
	_, encodeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(encodeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(temp, frigateJournalPath()); err != nil {
		return err
	}
	return frigate.SyncDirectory(frigateRoot())
}
func clearFrigateTransaction() error {
	if err := os.RemoveAll(frigateBackupPath()); err != nil {
		return err
	}
	if err := os.Remove(frigateJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return frigate.SyncDirectory(frigateRoot())
}
func snapshotFrigateConfig() (frigate.Snapshot, error) {
	entries, err := frigate.ScanConfig(filepath.Join(frigateRoot(), "config"), frigateSnapshotLimit)
	if err != nil {
		return nil, err
	}
	var required uint64
	for _, entry := range entries {
		if !entry.Directory {
			required += uint64(entry.Size)
		}
	}
	var disk unix.Statfs_t
	if err := unix.Statfs(frigateRoot(), &disk); err != nil {
		return nil, err
	}
	if uint64(disk.Bavail)*uint64(disk.Bsize) < required+(2<<30) {
		return nil, errors.New("Frigate upgrade requires backup space plus 2 GiB free reserve")
	}
	return frigate.CopyConfig(filepath.Join(frigateRoot(), "config"), frigateBackupPath(), frigateSnapshotLimit)
}
func restoreFrigateConfig(txn *frigateTransaction) error {
	if err := frigate.VerifyConfig(frigateBackupPath(), txn.Snapshot, frigateSnapshotLimit); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(frigateRoot(), ".restore-*")
	if err != nil {
		return err
	}
	if err := os.Remove(temporary); err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if _, err := frigate.CopyConfig(frigateBackupPath(), temporary, frigateSnapshotLimit); err != nil {
		return err
	}
	config := filepath.Join(frigateRoot(), "config")
	failed := filepath.Join(frigateRoot(), fmt.Sprintf("failed-config-%d", time.Now().UnixNano()))
	if err := os.Rename(config, failed); err != nil {
		return err
	}
	if err := os.Rename(temporary, config); err != nil {
		return errors.Join(err, os.Rename(failed, config))
	}
	return frigate.SyncDirectory(frigateRoot())
}

// Rollback inspects every container before destructive actions. It works after
// interruption at either rename, and never restarts old code on migrated data.
func recoverFrigateTransaction(ctx context.Context, binary string, txn *frigateTransaction) (moduleContainerStatus, error) {
	name := moduleContainerName(frigate.ModuleID)
	candidate, previous := moduleUpgradeStageNames(frigate.ModuleID)
	found := map[string]frigateContainer{}
	for _, n := range []string{name, candidate, previous} {
		exists, managed, err := inspectManagedContainer(ctx, binary, n, frigate.ModuleID)
		if err != nil {
			return moduleContainerStatus{}, err
		}
		if !exists {
			continue
		}
		if !managed {
			return moduleContainerStatus{}, errors.New("refusing Frigate recovery over an unmanaged container")
		}
		out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Config.Image}}", n).Output()
		if err != nil {
			return moduleContainerStatus{}, err
		}
		image := strings.TrimSpace(string(out))
		if (n == candidate && image != txn.NewImage) || (n == previous && image != txn.OldImage) || (n == name && image != txn.OldImage && image != txn.NewImage) {
			return moduleContainerStatus{}, errors.New("Frigate recovery image mismatch")
		}
		info, err := verifyFrigateContainer(ctx, binary, n, image)
		if err != nil {
			return moduleContainerStatus{}, err
		}
		found[n] = info
	}
	oldName := ""
	for n, info := range found {
		if info.Config.Image == txn.OldImage {
			if oldName != "" {
				return moduleContainerStatus{}, errors.New("ambiguous Frigate rollback containers")
			}
			oldName = n
		}
	}
	if oldName == "" {
		return moduleContainerStatus{}, errors.New("previous Frigate container is missing; backup preserved for operator recovery")
	}
	if txn.Phase != "prepared" {
		if err := frigate.VerifyConfig(frigateBackupPath(), txn.Snapshot, frigateSnapshotLimit); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	for n, info := range found {
		if info.Config.Image == txn.NewImage {
			if err := dockerAICommand(ctx, binary, "rm", "-f", n); err != nil {
				return moduleContainerStatus{}, err
			}
		}
	}
	if found[oldName].State.Running {
		if err := dockerAICommand(ctx, binary, "stop", oldName); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	if txn.Phase != "prepared" {
		if err := restoreFrigateConfig(txn); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	if oldName != name {
		if err := dockerAICommand(ctx, binary, "rename", oldName, name); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	if err := dockerAICommand(ctx, binary, "start", name); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := waitFrigateHealthy(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, fmt.Errorf("previous Frigate failed recovery health check: %w", err)
	}
	if err := clearFrigateTransaction(); err != nil {
		return moduleContainerStatus{}, err
	}
	return inspectModuleContainer(ctx, binary, name)
}

func commitFrigateUpgrade(ctx context.Context, binary string, txn *frigateTransaction) (moduleContainerStatus, error) {
	name := moduleContainerName(frigate.ModuleID)
	_, previous := moduleUpgradeStageNames(frigate.ModuleID)
	if _, err := verifyFrigateContainer(ctx, binary, name, txn.NewImage); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := waitFrigateHealthy(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, err
	}
	exists, managed, err := inspectManagedContainer(ctx, binary, previous, frigate.ModuleID)
	if err != nil {
		return moduleContainerStatus{}, err
	}
	if exists {
		if !managed {
			return moduleContainerStatus{}, errors.New("unmanaged previous Frigate container")
		}
		if _, err := verifyFrigateContainer(ctx, binary, previous, txn.OldImage); err != nil {
			return moduleContainerStatus{}, err
		}
		if err := dockerAICommand(ctx, binary, "rm", previous); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	txn.Phase = "committed"
	if err := writeFrigateTransaction(txn); err != nil {
		return moduleContainerStatus{}, err
	}
	// Retain the verified config/database backup until the next approved update.
	return inspectModuleContainer(ctx, binary, name)
}

func upgradeFrigate(ctx context.Context, request updaterhelper.Request, report moduleProgressFunc) (moduleContainerStatus, string, error) {
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	txn, err := readFrigateTransaction()
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if request.Operation != "docker.module.upgrade" {
		if txn == nil || txn.NewImage != request.Image || txn.OldImage != request.PreviousImage {
			return moduleContainerStatus{}, "", errors.New("Frigate upgrade journal does not match the requested transaction")
		}
		if request.Operation == "docker.module.upgrade.commit" {
			state, err := commitFrigateUpgrade(ctx, binary, txn)
			return state, "Frigate registry commit finalized; verified backup retained", err
		}
		if txn.Phase == "committed" {
			return moduleContainerStatus{}, "", errors.New("committed Frigate rollback requires an explicit registry recovery")
		}
		state, err := recoverFrigateTransaction(ctx, binary, txn)
		return state, "previous Frigate image, configuration and database restored", err
	}
	if request.Image == request.PreviousImage {
		return moduleContainerStatus{}, "", errors.New("Frigate upgrade requires a different immutable image")
	}
	if txn != nil {
		if (txn.Phase == "committed" || txn.Phase == "swapped") && request.PreviousImage == txn.NewImage {
			if _, err := commitFrigateUpgrade(ctx, binary, txn); err != nil {
				return moduleContainerStatus{}, "", err
			}
			if err := clearFrigateTransaction(); err != nil {
				return moduleContainerStatus{}, "", err
			}
		} else if request.PreviousImage == txn.OldImage {
			recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Minute)
			defer cancel()
			_, err := recoverFrigateTransaction(recovery, binary, txn)
			if err != nil {
				return moduleContainerStatus{}, "", err
			}
			return moduleContainerStatus{}, "", errors.New("interrupted Frigate upgrade recovered; review and retry the update")
		} else {
			return moduleContainerStatus{}, "", errors.New("Frigate registry and pending transaction disagree; recovery required")
		}
	}
	name := moduleContainerName(frigate.ModuleID)
	candidate, previous := moduleUpgradeStageNames(frigate.ModuleID)
	info, err := verifyFrigateContainer(ctx, binary, name, request.PreviousImage)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if !info.State.Running {
		return moduleContainerStatus{}, "", errors.New("Frigate must be running before an upgrade")
	}
	if err := ensureModuleUpgradeNamesFree(ctx, binary, candidate, previous); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := verifyFrigateDatabaseLocation(ctx, binary, name); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("download", "")
	if err := pullFrigateImage(ctx, binary, request.Image, report); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("create", "")
	if err := dockerAICommand(ctx, binary, frigateCreateArgs(candidate, request.Image)...); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if _, err := verifyFrigateContainer(ctx, binary, candidate, request.Image); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(binary, candidate, err)
	}
	txn = &frigateTransaction{OldImage: request.PreviousImage, NewImage: request.Image, Phase: "prepared"}
	if err := writeFrigateTransaction(txn); err != nil {
		return moduleContainerStatus{}, "", cleanupFailedModuleStart(binary, candidate, err)
	}
	critical, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Minute)
	defer cancel()
	fail := func(cause error) (moduleContainerStatus, string, error) {
		recovery, done := context.WithTimeout(context.Background(), 8*time.Minute)
		defer done()
		report("rollback", "")
		_, rollbackErr := recoverFrigateTransaction(recovery, binary, txn)
		if rollbackErr != nil {
			return moduleContainerStatus{}, "", errors.Join(cause, fmt.Errorf("Frigate automatic rollback failed; backup and journal preserved: %w", rollbackErr))
		}
		return moduleContainerStatus{}, "", fmt.Errorf("Frigate update failed; previous image, configuration and database restored: %w", cause)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := dockerAICommand(critical, binary, "stop", name); err != nil {
		return fail(err)
	}
	snapshot, err := snapshotFrigateConfig()
	if err != nil {
		return fail(err)
	}
	txn.Snapshot = snapshot
	txn.Phase = "snapshotted"
	if err := writeFrigateTransaction(txn); err != nil {
		return fail(err)
	}
	report("snapshot", "configuration and SQLite backup verified")
	report("start", "")
	if err := dockerAICommand(critical, binary, "start", candidate); err != nil {
		return fail(err)
	}
	report("verify", "")
	if err := waitFrigateHealthy(critical, binary, candidate); err != nil {
		return fail(err)
	}
	if err := dockerAICommand(critical, binary, "rename", name, previous); err != nil {
		return fail(err)
	}
	if err := dockerAICommand(critical, binary, "rename", candidate, name); err != nil {
		return fail(err)
	}
	txn.Phase = "swapped"
	if err := writeFrigateTransaction(txn); err != nil {
		return fail(err)
	}
	state, err := inspectModuleContainer(critical, binary, name)
	if err != nil {
		return fail(err)
	}
	if state.State != "running" {
		return fail(errors.New("Frigate stopped after the healthy container swap"))
	}
	return state, "Frigate candidate healthy; old container and verified database backup retained until registry commit", nil
}
