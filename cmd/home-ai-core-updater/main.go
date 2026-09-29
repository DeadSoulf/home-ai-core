package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updater"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const (
	socketPath        = "/run/home-ai-core-updater.sock"
	updateRoot        = "/var/lib/home-ai-core/update"
	liveBinary        = "/usr/bin/home-ai-core"
	liveWeb           = "/usr/share/home-ai-core/web"
	liveHelper        = "/usr/libexec/home-ai-core/home-ai-core-updater"
	serviceName       = "home-ai-core.service"
	helperServiceName = "home-ai-core-updater.service"
	serviceUser       = "home-ai-core"
	maxRequest        = 16 << 10
)

var installMu sync.Mutex

type backupMetadata struct {
	Version    string `json:"version"`
	ReplacedBy string `json:"replaced_by,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if os.Geteuid() != 0 {
		logger.Error("updater helper must run as root")
		os.Exit(1)
	}

	uid, gid, err := serviceIdentity()
	if err != nil {
		logger.Error("resolve service identity", "error", err)
		os.Exit(1)
	}
	if err := prepareSocket(socketPath); err != nil {
		logger.Error("prepare updater socket", "error", err)
		os.Exit(1)
	}

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		logger.Error("listen updater socket", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	if err := os.Chown(socketPath, 0, gid); err != nil {
		logger.Error("chown updater socket", "error", err)
		os.Exit(1)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		logger.Error("chmod updater socket", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	logger.Info(
		"bundle updater helper ready",
		"socket", socketPath,
		"version", updaterhelper.HelperVersion,
		"protocol", updaterhelper.ProtocolVersion,
	)
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("accept updater connection", "error", err)
			continue
		}
		go handleConnection(ctx, logger, conn, uint32(uid), uid, gid)
	}
}

func handleConnection(parent context.Context, logger *slog.Logger, conn *net.UnixConn, expectedUID uint32, uid, gid int) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	peer, err := peerUID(conn)
	if err != nil || peer != expectedUID {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "unauthorized updater client"})
		return
	}

	var request updaterhelper.Request
	decoder := json.NewDecoder(io.LimitReader(bufio.NewReader(conn), maxRequest+1))
	// Keep the privileged helper forward-compatible with newer Core/Web clients.
	// Unknown optional request fields are ignored; every operation still validates
	// its own required arguments and the peer UID before execution.
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "invalid updater request"})
		return
	}
	if request.Operation == "info" {
		metadata, rollbackAvailable := availableRollback()
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:                true,
			Message:           "helper ready",
			HelperVersion:     updaterhelper.HelperVersion,
			ProtocolVersion:   updaterhelper.ProtocolVersion,
			RollbackAvailable: rollbackAvailable,
			RollbackVersion:   metadata.Version,
		})
		return
	}
	if request.ProtocolVersion > updaterhelper.ProtocolVersion {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			Error: fmt.Sprintf(
				"unsupported helper protocol %d; helper supports %d",
				request.ProtocolVersion,
				updaterhelper.ProtocolVersion,
			),
			HelperVersion:   updaterhelper.HelperVersion,
			ProtocolVersion: updaterhelper.ProtocolVersion,
		})
		return
	}
	if request.Operation == "storage.inspect" {
		_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
		ctx, cancel := context.WithTimeout(parent, 40*time.Second)
		defer cancel()
		stats, err := inspectFilesystemStats(ctx)
		if err != nil {
			logger.Error("storage inspection failed", "error", err)
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		health := inspectDiskHealth(ctx)
		lvm := inspectLVM(ctx)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:              true,
			Message:         "storage inspected",
			HelperVersion:   updaterhelper.HelperVersion,
			ProtocolVersion: updaterhelper.ProtocolVersion,
			FilesystemStats: stats,
			DiskHealth:      health,
			LVM:             lvm,
		})
		return
	}
	if strings.HasPrefix(request.Operation, "storage.") {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		message, err := performStorageOperation(ctx, request)
		if err != nil {
			logger.Error("storage operation failed", "operation", request.Operation, "device", request.Device, "error", err)
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		logger.Info("storage operation completed", "operation", request.Operation, "device", request.Device)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{OK: true, Message: message})
		return
	}

	if request.Operation == "rollback" {
		metadata, rollbackAvailable := availableRollback()
		if !rollbackAvailable || !validVersion(metadata.Version) {
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "no valid rollback backup is available"})
			return
		}
		if !installMu.TryLock() {
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "update installation already in progress"})
			return
		}
		if err := json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:                true,
			Message:           "rollback accepted",
			RollbackAvailable: true,
			RollbackVersion:   metadata.Version,
		}); err != nil {
			installMu.Unlock()
			return
		}
		go func() {
			defer installMu.Unlock()
			time.Sleep(500 * time.Millisecond)
			ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
			defer cancel()
			helperUpdated, err := performManualRollback(ctx, filepath.Join(updateRoot, "backup"))
			if err != nil {
				logger.Error("update rollback failed", "version", metadata.Version, "error", err)
				_ = writeResult(uid, gid, updaterhelper.Result{
					Status:    "failed",
					Version:   metadata.Version,
					Error:     err.Error(),
					UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				})
				return
			}
			logger.Info("update rollback completed", "version", metadata.Version, "helper_updated", helperUpdated)
			_ = writeResult(uid, gid, updaterhelper.Result{
				Status:    "succeeded",
				Version:   metadata.Version,
				Message:   "rollback completed",
				UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
			if helperUpdated {
				_ = exec.Command("/usr/bin/systemctl", "restart", "--no-block", helperServiceName).Run()
			}
		}()
		return
	}

	if request.Operation != "install" || !validVersion(request.Version) {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "invalid updater operation or version"})
		return
	}
	if !installMu.TryLock() {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "update installation already in progress"})
		return
	}

	prepared := filepath.Join(updateRoot, "prepared-"+request.Version)
	if _, err := updater.VerifyPreparedBundle(prepared, request.Version); err != nil {
		installMu.Unlock()
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
		return
	}

	if err := json.NewEncoder(conn).Encode(updaterhelper.Response{OK: true, Message: "installation accepted"}); err != nil {
		installMu.Unlock()
		return
	}

	go func() {
		defer installMu.Unlock()
		time.Sleep(750 * time.Millisecond)
		ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
		defer cancel()
		helperUpdated, err := performInstall(ctx, request.CurrentVersion, request.Version, prepared, uid, gid)
		if err != nil {
			logger.Error("bundle update failed", "version", request.Version, "error", err)
			_ = writeResult(uid, gid, updaterhelper.Result{
				Status:    "failed",
				Version:   request.Version,
				Error:     err.Error(),
				UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
			return
		}
		logger.Info("bundle update installed", "version", request.Version, "helper_updated", helperUpdated)
		_ = writeResult(uid, gid, updaterhelper.Result{
			Status:    "succeeded",
			Version:   request.Version,
			Message:   "update installed",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if helperUpdated {
			logger.Info("restarting updater helper to activate new version")
			_ = exec.Command("/usr/bin/systemctl", "restart", "--no-block", helperServiceName).Run()
		}
	}()
}

func writeBackupMetadata(backup string, metadata backupMetadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(backup, "metadata.json"), data, 0o600)
}

func readBackupMetadata(backup string) (backupMetadata, error) {
	data, err := os.ReadFile(filepath.Join(backup, "metadata.json"))
	if err != nil {
		return backupMetadata{}, err
	}
	var metadata backupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return backupMetadata{}, err
	}
	return metadata, nil
}

func availableRollback() (backupMetadata, bool) {
	backup := filepath.Join(updateRoot, "backup")
	metadata, err := readBackupMetadata(backup)
	if err != nil || !validVersion(metadata.Version) {
		return backupMetadata{}, false
	}
	coreInfo, err := os.Stat(filepath.Join(backup, "bin", "home-ai-core"))
	if err != nil || !coreInfo.Mode().IsRegular() {
		return backupMetadata{}, false
	}
	webInfo, err := os.Stat(filepath.Join(backup, "web"))
	if err != nil || !webInfo.IsDir() {
		return backupMetadata{}, false
	}
	return metadata, true
}

func performInstall(ctx context.Context, currentVersion, version, prepared string, uid, gid int) (bool, error) {
	backup := filepath.Join(updateRoot, "backup")
	if err := os.RemoveAll(backup); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Join(backup, "bin"), 0o700); err != nil {
		return false, err
	}
	if err := copyFile(liveBinary, filepath.Join(backup, "bin", "home-ai-core"), 0o700); err != nil {
		return false, fmt.Errorf("backup core binary: %w", err)
	}
	if err := copyTree(liveWeb, filepath.Join(backup, "web")); err != nil {
		return false, fmt.Errorf("backup web ui: %w", err)
	}
	if validVersion(currentVersion) {
		if err := writeBackupMetadata(backup, backupMetadata{
			Version:    currentVersion,
			ReplacedBy: version,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return false, fmt.Errorf("write rollback metadata: %w", err)
		}
	}

	newBinary := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.new")
	if err := copyFile(filepath.Join(prepared, "bin", "home-ai-core"), newBinary, 0o755); err != nil {
		return false, fmt.Errorf("stage new core binary: %w", err)
	}
	newWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-new")
	rollbackWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-rollback")
	_ = os.RemoveAll(newWeb)
	_ = os.RemoveAll(rollbackWeb)
	if err := copyTree(filepath.Join(prepared, "web"), newWeb); err != nil {
		return false, fmt.Errorf("stage new web ui: %w", err)
	}

	helperUpdated := false
	preparedHelper := filepath.Join(prepared, "helper", "home-ai-core-updater")
	newHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core-updater.new")
	if helperInfo, err := os.Stat(preparedHelper); err == nil && helperInfo.Mode().IsRegular() {
		if err := os.MkdirAll(filepath.Join(backup, "helper"), 0o700); err != nil {
			return false, fmt.Errorf("prepare helper backup: %w", err)
		}
		if err := copyFile(liveHelper, filepath.Join(backup, "helper", "home-ai-core-updater"), 0o700); err != nil {
			return false, fmt.Errorf("backup updater helper: %w", err)
		}
		if err := copyFile(preparedHelper, newHelper, 0o755); err != nil {
			return false, fmt.Errorf("stage updater helper: %w", err)
		}
	}

	if err := systemctl(ctx, "stop", serviceName); err != nil {
		return false, fmt.Errorf("stop Home-AI-Core: %w", err)
	}

	rollbackNeeded := true
	defer func() {
		if rollbackNeeded {
			rollback(context.Background(), backup, rollbackWeb)
		}
	}()

	if err := os.Rename(newBinary, liveBinary); err != nil {
		return false, fmt.Errorf("replace core binary: %w", err)
	}
	if err := os.Rename(liveWeb, rollbackWeb); err != nil {
		return false, fmt.Errorf("preserve current web ui: %w", err)
	}
	if err := os.Rename(newWeb, liveWeb); err != nil {
		_ = os.Rename(rollbackWeb, liveWeb)
		return false, fmt.Errorf("replace web ui: %w", err)
	}

	if err := systemctl(ctx, "start", serviceName); err != nil {
		return false, fmt.Errorf("start updated Home-AI-Core: %w", err)
	}
	if err := waitActive(ctx, serviceName, 15*time.Second); err != nil {
		return false, err
	}

	if _, err := os.Stat(newHelper); err == nil {
		if err := os.Rename(newHelper, liveHelper); err != nil {
			return false, fmt.Errorf("replace updater helper: %w", err)
		}
		if err := os.Chmod(liveHelper, 0o755); err != nil {
			return false, fmt.Errorf("set updater helper permissions: %w", err)
		}
		helperUpdated = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect staged updater helper: %w", err)
	}

	rollbackNeeded = false
	_ = os.RemoveAll(rollbackWeb)
	_ = os.Chown(filepath.Join(updateRoot, "backup"), uid, gid)
	return helperUpdated, nil
}

func performManualRollback(ctx context.Context, backup string) (bool, error) {
	if _, ok := availableRollback(); !ok {
		return false, errors.New("rollback backup is incomplete")
	}

	current := filepath.Join(updateRoot, "rollback-current")
	if err := os.RemoveAll(current); err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Join(current, "bin"), 0o700); err != nil {
		return false, err
	}
	if err := copyFile(liveBinary, filepath.Join(current, "bin", "home-ai-core"), 0o700); err != nil {
		return false, fmt.Errorf("backup current core before rollback: %w", err)
	}
	if err := copyTree(liveWeb, filepath.Join(current, "web")); err != nil {
		return false, fmt.Errorf("backup current web before rollback: %w", err)
	}
	if info, err := os.Stat(liveHelper); err == nil && info.Mode().IsRegular() {
		if err := os.MkdirAll(filepath.Join(current, "helper"), 0o700); err != nil {
			return false, err
		}
		if err := copyFile(liveHelper, filepath.Join(current, "helper", "home-ai-core-updater"), 0o700); err != nil {
			return false, fmt.Errorf("backup current helper before rollback: %w", err)
		}
	}

	targetBinary := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.rollback-target")
	targetWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-rollback-target")
	preRollbackWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-pre-rollback")
	targetHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core-updater.rollback-target")
	_ = os.Remove(targetBinary)
	_ = os.RemoveAll(targetWeb)
	_ = os.RemoveAll(preRollbackWeb)
	_ = os.Remove(targetHelper)

	if err := copyFile(filepath.Join(backup, "bin", "home-ai-core"), targetBinary, 0o755); err != nil {
		return false, fmt.Errorf("stage rollback core: %w", err)
	}
	if err := copyTree(filepath.Join(backup, "web"), targetWeb); err != nil {
		return false, fmt.Errorf("stage rollback web: %w", err)
	}
	helperWillChange := false
	backupHelper := filepath.Join(backup, "helper", "home-ai-core-updater")
	if info, err := os.Stat(backupHelper); err == nil && info.Mode().IsRegular() {
		if err := copyFile(backupHelper, targetHelper, 0o755); err != nil {
			return false, fmt.Errorf("stage rollback helper: %w", err)
		}
		helperWillChange = true
	}

	restoreCurrent := func() {
		_ = systemctl(context.Background(), "stop", serviceName)
		tmp := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.restore-current")
		if err := copyFile(filepath.Join(current, "bin", "home-ai-core"), tmp, 0o755); err == nil {
			_ = os.Rename(tmp, liveBinary)
		}
		_ = os.RemoveAll(liveWeb)
		if _, err := os.Stat(preRollbackWeb); err == nil {
			_ = os.Rename(preRollbackWeb, liveWeb)
		} else {
			_ = copyTree(filepath.Join(current, "web"), liveWeb)
		}
		currentHelper := filepath.Join(current, "helper", "home-ai-core-updater")
		if info, err := os.Stat(currentHelper); err == nil && info.Mode().IsRegular() {
			tmpHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core.restore-current-helper")
			if err := copyFile(currentHelper, tmpHelper, 0o755); err == nil {
				_ = os.Rename(tmpHelper, liveHelper)
			}
		}
		_ = systemctl(context.Background(), "start", serviceName)
	}

	if err := systemctl(ctx, "stop", serviceName); err != nil {
		return false, fmt.Errorf("stop Home-AI-Core for rollback: %w", err)
	}
	if err := os.Rename(targetBinary, liveBinary); err != nil {
		_ = systemctl(context.Background(), "start", serviceName)
		return false, fmt.Errorf("replace core during rollback: %w", err)
	}
	if err := os.Rename(liveWeb, preRollbackWeb); err != nil {
		restoreCurrent()
		return false, fmt.Errorf("preserve current web during rollback: %w", err)
	}
	if err := os.Rename(targetWeb, liveWeb); err != nil {
		restoreCurrent()
		return false, fmt.Errorf("replace web during rollback: %w", err)
	}
	if err := systemctl(ctx, "start", serviceName); err != nil {
		restoreCurrent()
		return false, fmt.Errorf("start rolled-back Home-AI-Core: %w", err)
	}
	if err := waitActive(ctx, serviceName, 15*time.Second); err != nil {
		restoreCurrent()
		return false, err
	}

	helperUpdated := false
	if helperWillChange {
		if err := os.Rename(targetHelper, liveHelper); err != nil {
			restoreCurrent()
			return false, fmt.Errorf("restore updater helper: %w", err)
		}
		helperUpdated = true
	}

	_ = os.RemoveAll(preRollbackWeb)
	_ = os.RemoveAll(current)
	return helperUpdated, nil
}

func rollback(ctx context.Context, backup, rollbackWeb string) {
	_ = systemctl(ctx, "stop", serviceName)

	tmp := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.rollback")
	if err := copyFile(filepath.Join(backup, "bin", "home-ai-core"), tmp, 0o755); err == nil {
		_ = os.Rename(tmp, liveBinary)
	}

	_ = os.RemoveAll(liveWeb)
	if _, err := os.Stat(rollbackWeb); err == nil {
		_ = os.Rename(rollbackWeb, liveWeb)
	} else {
		_ = copyTree(filepath.Join(backup, "web"), liveWeb)
	}

	backupHelper := filepath.Join(backup, "helper", "home-ai-core-updater")
	if info, err := os.Stat(backupHelper); err == nil && info.Mode().IsRegular() {
		tmpHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core-updater.rollback")
		if err := copyFile(backupHelper, tmpHelper, 0o755); err == nil {
			_ = os.Rename(tmpHelper, liveHelper)
		}
	}
	_ = systemctl(ctx, "start", serviceName)
}

func waitActive(ctx context.Context, service string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", service).Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("updated Home-AI-Core did not become active")
}

func systemctl(ctx context.Context, action, service string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", action, service).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(target, mode); err != nil {
		return err
	}
	return nil
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			return os.Chmod(dest, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular file %s", path)
		}
		return copyFile(path, dest, 0o644)
	})
}

func writeResult(uid, gid int, result updaterhelper.Result) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(updateRoot, "install-result.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chown(tmp, uid, gid); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type inspectLsblkOutput struct {
	BlockDevices []inspectLsblkNode `json:"blockdevices"`
}

type inspectLsblkNode struct {
	Path        string             `json:"path"`
	Type        string             `json:"type"`
	Filesystem  string             `json:"fstype"`
	FreeBytes   *uint64            `json:"fsavail"`
	Mountpoints []*string          `json:"mountpoints"`
	Children    []inspectLsblkNode `json:"children"`
}

func inspectFilesystemStats(ctx context.Context) ([]updaterhelper.FilesystemStat, error) {
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/lsblk",
		"--json",
		"--bytes",
		"--output", "PATH,TYPE,FSTYPE,FSAVAIL,MOUNTPOINTS",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect filesystems: %s", strings.TrimSpace(string(output)))
	}
	var decoded inspectLsblkOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("decode filesystem inventory: %w", err)
	}

	stats := make([]updaterhelper.FilesystemStat, 0)
	var visit func(inspectLsblkNode)
	visit = func(node inspectLsblkNode) {
		filesystem := strings.ToLower(strings.TrimSpace(node.Filesystem))
		if node.Path != "" && filesystem != "" && filesystem != "swap" && filesystem != "lvm2_member" {
			stat := updaterhelper.FilesystemStat{
				Device:     node.Path,
				Filesystem: filesystem,
			}
			if node.FreeBytes != nil {
				stat.FreeBytes = *node.FreeBytes
				stat.FreeKnown = true
			} else if free, ok := offlineFilesystemFreeBytesPrivileged(ctx, node.Path, filesystem); ok {
				stat.FreeBytes = free
				stat.FreeKnown = true
			}
			stats = append(stats, stat)
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	for _, node := range decoded.BlockDevices {
		visit(node)
	}
	return stats, nil
}

func offlineFilesystemFreeBytesPrivileged(ctx context.Context, device, filesystem string) (uint64, bool) {
	switch filesystem {
	case "ext2", "ext3", "ext4":
		output, err := exec.CommandContext(ctx, "/usr/sbin/dumpe2fs", "-h", device).CombinedOutput()
		if err != nil {
			return 0, false
		}
		var freeBlocks, blockSize uint64
		for _, line := range strings.Split(string(output), "\n") {
			key, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil {
				continue
			}
			switch strings.TrimSpace(key) {
			case "Free blocks":
				freeBlocks = number
			case "Block size":
				blockSize = number
			}
		}
		if blockSize == 0 {
			return 0, false
		}
		return freeBlocks * blockSize, true
	case "xfs":
		output, err := exec.CommandContext(
			ctx,
			"/usr/sbin/xfs_db",
			"-r",
			"-c", "sb 0",
			"-c", "p blocksize fdblocks",
			device,
		).CombinedOutput()
		if err != nil {
			return 0, false
		}
		var freeBlocks, blockSize uint64
		for _, line := range strings.Split(string(output), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			number, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil {
				continue
			}
			switch strings.TrimSpace(key) {
			case "fdblocks":
				freeBlocks = number
			case "blocksize":
				blockSize = number
			}
		}
		if blockSize == 0 {
			return 0, false
		}
		return freeBlocks * blockSize, true
	case "vfat", "fat", "fat32":
		output, err := exec.CommandContext(ctx, "/usr/sbin/fsck.fat", "-n", "-v", device).CombinedOutput()
		if err != nil {
			return 0, false
		}
		var clusterSize, usedClusters, totalClusters uint64
		for _, line := range strings.Split(string(output), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.Contains(trimmed, "bytes per cluster") {
				fields := strings.Fields(trimmed)
				if len(fields) > 0 {
					clusterSize, _ = strconv.ParseUint(fields[0], 10, 64)
				}
			}
			if !strings.Contains(trimmed, "clusters") || !strings.Contains(trimmed, "/") {
				continue
			}
			for _, field := range strings.Fields(trimmed) {
				if !strings.Contains(field, "/") {
					continue
				}
				parts := strings.SplitN(strings.Trim(field, ",;()"), "/", 2)
				if len(parts) != 2 {
					continue
				}
				used, errUsed := strconv.ParseUint(parts[0], 10, 64)
				total, errTotal := strconv.ParseUint(parts[1], 10, 64)
				if errUsed == nil && errTotal == nil && total >= used {
					usedClusters = used
					totalClusters = total
				}
			}
		}
		if clusterSize == 0 || totalClusters == 0 || totalClusters < usedClusters {
			return 0, false
		}
		return (totalClusters - usedClusters) * clusterSize, true
	default:
		return 0, false
	}
}

type smartctlJSON struct {
	SmartStatus *struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature *struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime *struct {
		Hours uint64 `json:"hours"`
	} `json:"power_on_time"`
	NVMe *struct {
		PercentageUsed int `json:"percentage_used"`
		Temperature    int `json:"temperature"`
	} `json:"nvme_smart_health_information_log"`
	ATAAttributes *struct {
		Table []struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
			Raw   struct {
				Value int64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

type healthLsblkOutput struct {
	BlockDevices []struct {
		Path      string `json:"path"`
		Type      string `json:"type"`
		Transport string `json:"tran"`
	} `json:"blockdevices"`
}

func inspectDiskHealth(ctx context.Context) []updaterhelper.DiskHealthStat {
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/lsblk",
		"--json",
		"--output", "PATH,TYPE,TRAN",
	).CombinedOutput()
	if err != nil {
		return nil
	}
	var devices healthLsblkOutput
	if json.Unmarshal(output, &devices) != nil {
		return nil
	}

	_, smartErr := os.Stat("/usr/sbin/smartctl")
	result := make([]updaterhelper.DiskHealthStat, 0, len(devices.BlockDevices))
	for _, item := range devices.BlockDevices {
		if item.Type != "disk" || item.Path == "" {
			continue
		}
		stat := updaterhelper.DiskHealthStat{
			Device:    item.Path,
			Transport: strings.ToLower(strings.TrimSpace(item.Transport)),
			Health:    "unknown",
		}
		if smartErr != nil {
			stat.SmartError = "smartctl is not installed"
			result = append(result, stat)
			continue
		}

		smartOutput, commandErr := exec.CommandContext(
			ctx,
			"/usr/sbin/smartctl",
			"-j", "-H", "-A",
			item.Path,
		).CombinedOutput()
		var decoded smartctlJSON
		if err := json.Unmarshal(smartOutput, &decoded); err != nil {
			message := strings.TrimSpace(string(smartOutput))
			if message == "" && commandErr != nil {
				message = commandErr.Error()
			}
			if len(message) > 240 {
				message = message[:240]
			}
			stat.SmartError = message
			result = append(result, stat)
			continue
		}

		stat.SmartAvailable =
			decoded.SmartStatus != nil ||
			decoded.Temperature != nil ||
			decoded.PowerOnTime != nil ||
			decoded.NVMe != nil ||
			decoded.ATAAttributes != nil
		if decoded.SmartStatus != nil {
			if decoded.SmartStatus.Passed {
				stat.Health = "ok"
			} else {
				stat.Health = "failed"
			}
		}
		if decoded.Temperature != nil && decoded.Temperature.Current > -100 && decoded.Temperature.Current < 200 {
			value := decoded.Temperature.Current
			stat.TemperatureC = &value
		}
		if decoded.PowerOnTime != nil {
			value := decoded.PowerOnTime.Hours
			stat.PowerOnHours = &value
		}
		if decoded.NVMe != nil {
			if stat.TemperatureC == nil && decoded.NVMe.Temperature > -100 && decoded.NVMe.Temperature < 200 {
				value := decoded.NVMe.Temperature
				stat.TemperatureC = &value
			}
			remaining := 100 - decoded.NVMe.PercentageUsed
			if remaining < 0 {
				remaining = 0
			}
			if remaining > 100 {
				remaining = 100
			}
			stat.LifeRemainingPct = &remaining
		}
		if stat.LifeRemainingPct == nil && decoded.ATAAttributes != nil {
			for _, attribute := range decoded.ATAAttributes.Table {
				name := strings.ToLower(strings.ReplaceAll(attribute.Name, "-", "_"))
				switch name {
				case "ssd_life_left", "percent_lifetime_remain", "media_wearout_indicator":
					remaining := attribute.Value
					if remaining < 0 {
						remaining = 0
					}
					if remaining > 100 {
						remaining = 100
					}
					stat.LifeRemainingPct = &remaining
				}
				if stat.LifeRemainingPct != nil {
					break
				}
			}
		}
		if stat.Health == "ok" {
			hot := stat.TemperatureC != nil && *stat.TemperatureC >= 60
			worn := stat.LifeRemainingPct != nil && *stat.LifeRemainingPct <= 10
			if hot || worn {
				stat.Health = "warning"
			}
		}
		if commandErr != nil && stat.Health == "unknown" {
			stat.SmartError = "smartctl could not read SMART data for this device"
			stat.SmartAvailable = false
		}
		result = append(result, stat)
	}
	return result
}

type lvsJSON struct {
	Report []struct {
		LV []struct {
			Path            string `json:"lv_path"`
			LVName          string `json:"lv_name"`
			VGName          string `json:"vg_name"`
			Size            string `json:"lv_size"`
			VGSize          string `json:"vg_size"`
			VGFree          string `json:"vg_free"`
			Attr            string `json:"lv_attr"`
			DataPercent     string `json:"data_percent"`
			MetadataPercent string `json:"metadata_percent"`
		} `json:"lv"`
	} `json:"report"`
}

func inspectLVM(ctx context.Context) []updaterhelper.LVMStat {
	if _, err := os.Stat("/usr/sbin/lvs"); err != nil {
		return nil
	}
	output, err := exec.CommandContext(
		ctx,
		"/usr/sbin/lvs",
		"--reportformat", "json",
		"--units", "b",
		"--nosuffix",
		"-o", "lv_path,lv_name,vg_name,lv_size,vg_size,vg_free,lv_attr,data_percent,metadata_percent",
	).CombinedOutput()
	if err != nil {
		return nil
	}
	var decoded lvsJSON
	if json.Unmarshal(output, &decoded) != nil {
		return nil
	}
	var result []updaterhelper.LVMStat
	for _, report := range decoded.Report {
		for _, item := range report.LV {
			size, _ := strconv.ParseFloat(strings.TrimSpace(item.Size), 64)
			vgSize, _ := strconv.ParseFloat(strings.TrimSpace(item.VGSize), 64)
			vgFree, _ := strconv.ParseFloat(strings.TrimSpace(item.VGFree), 64)
			stat := updaterhelper.LVMStat{
				Device:    strings.TrimSpace(item.Path),
				Name:      lvmMapperName(strings.TrimSpace(item.VGName), strings.TrimSpace(item.LVName)),
				VGName:    strings.TrimSpace(item.VGName),
				LVName:    strings.TrimSpace(item.LVName),
				SizeBytes:   uint64(size),
				VGSizeBytes: uint64(vgSize),
				VGFreeBytes: uint64(vgFree),
				Active:      len(item.Attr) > 4 && item.Attr[4] == 'a',
			}
			if value, err := strconv.ParseFloat(strings.TrimSpace(item.DataPercent), 64); err == nil {
				stat.DataPercent = &value
			}
			if value, err := strconv.ParseFloat(strings.TrimSpace(item.MetadataPercent), 64); err == nil {
				stat.MetadataPercent = &value
			}
			result = append(result, stat)
		}
	}
	return result
}

func lvmMapperName(vg, lv string) string {
	escape := func(value string) string { return strings.ReplaceAll(value, "-", "--") }
	if vg == "" || lv == "" {
		return ""
	}
	return escape(vg) + "-" + escape(lv)
}

func performStorageOperation(ctx context.Context, request updaterhelper.Request) (string, error) {
	device, err := validateBlockDevice(request.Device)
	if err != nil {
		return "", err
	}

	switch request.Operation {
	case "storage.mount":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("device is already mounted")
		}
		target := strings.TrimSpace(request.Mountpoint)
		if target == "" {
			target = filepath.Join("/mnt/home-ai-core", filepath.Base(device))
		}
		target = filepath.Clean(target)
		if target != "/mnt/home-ai-core" && !strings.HasPrefix(target, "/mnt/home-ai-core/") {
			return "", errors.New("mount point must be under /mnt/home-ai-core")
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return "", fmt.Errorf("create mount point: %w", err)
		}
		if err := os.Chmod(target, 0o755); err != nil {
			return "", fmt.Errorf("set mount point permissions: %w", err)
		}
		if output, err := exec.CommandContext(ctx, "/usr/bin/mount", "--", device, target).CombinedOutput(); err != nil {
			return "", fmt.Errorf("mount device: %s", strings.TrimSpace(string(output)))
		}
		targets, err = mountedTargets(ctx, device)
		if err != nil {
			return "", fmt.Errorf("verify mount: %w", err)
		}
		mounted := false
		for _, mountedTarget := range targets {
			if mountedTarget == target {
				mounted = true
				break
			}
		}
		if !mounted {
			return "", errors.New("mount completed but verification did not find the requested mount")
		}
		return "device mounted", nil

	case "storage.unmount":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		for _, target := range targets {
			if target == "/" {
				return "", errors.New("refusing to unmount the root filesystem")
			}
		}
		if len(targets) == 0 {
			return "", errors.New("device is not mounted")
		}
		if output, err := exec.CommandContext(ctx, "/usr/bin/umount", "--", device).CombinedOutput(); err != nil {
			return "", fmt.Errorf("unmount device: %s", strings.TrimSpace(string(output)))
		}
		remaining, err := mountedTargets(ctx, device)
		if err != nil {
			return "", fmt.Errorf("verify unmount: %w", err)
		}
		if len(remaining) != 0 {
			return "", fmt.Errorf("unmount completed but device is still mounted at: %s", strings.Join(remaining, ", "))
		}
		return "device unmounted", nil

	case "storage.partition.create":
		if request.Confirm != "CREATE "+device {
			return "", errors.New("partition creation confirmation does not match disk")
		}
		if err := requireDiskType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to change the partition table on the system disk")
		}
		if mounted, err := diskHasMountedDescendants(ctx, device); err != nil {
			return "", err
		} else if mounted {
			return "", errors.New("all filesystems on the disk must be unmounted before changing partitions")
		}
		if request.SizeMiB > 0 && request.SizeMiB < 16 {
			return "", errors.New("partition size must be at least 16 MiB")
		}
		if request.SizeMiB > 0 && request.SizeMiB > 1024*1024*1024 {
			return "", errors.New("partition size is too large")
		}
		if request.SizeMiB > 0 {
			remaining, err := remainingDiskBytes(ctx, device)
			if err != nil {
				return "", err
			}
			const alignmentReserve = uint64(4 * 1024 * 1024)
			requested := request.SizeMiB * 1024 * 1024
			if remaining <= alignmentReserve || requested > remaining-alignmentReserve {
				return "", errors.New("requested partition size exceeds available unallocated space")
			}
		}
		if err := createPartition(ctx, device, request.SizeMiB); err != nil {
			return "", err
		}
		return "partition created", nil

	case "storage.partition.delete":
		if request.Confirm != filepath.Base(device) {
			return "", errors.New("partition deletion confirmation does not match device")
		}
		if err := requirePartitionType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to delete a partition on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		if err := deletePartition(ctx, device); err != nil {
			return "", err
		}
		return "partition deleted", nil

	case "storage.partition.delete_all":
		if request.Confirm != filepath.Base(device) {
			return "", errors.New("delete-all confirmation does not match disk")
		}
		if err := requireDiskType(ctx, device); err != nil {
			return "", err
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to delete partitions on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		if err := deleteAllPartitions(ctx, device); err != nil {
			return "", err
		}
		return "all partitions deleted", nil

	case "storage.label.rename":
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("device must be unmounted before renaming")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to rename a filesystem on the system disk")
		}
		label := strings.TrimSpace(request.Label)
		if label == "" {
			return "", errors.New("filesystem label cannot be empty")
		}
		fstypeOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "FSTYPE", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("inspect filesystem type: %s", strings.TrimSpace(string(fstypeOutput)))
		}
		fstype := strings.ToLower(strings.TrimSpace(string(fstypeOutput)))
		if !validFilesystemLabelForType(label, fstype) {
			return "", errors.New("invalid filesystem label for filesystem type")
		}
		var command string
		var args []string
		switch fstype {
		case "ext4":
			command = "/usr/sbin/e2label"
			args = []string{device, label}
		case "xfs":
			command = "/usr/sbin/xfs_admin"
			args = []string{"-L", label, device}
		case "vfat", "fat", "fat32":
			command = "/usr/sbin/fatlabel"
			args = []string{device, label}
		default:
			return "", errors.New("renaming is supported for ext4, xfs and vfat filesystems")
		}
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("filesystem label tool is unavailable: %s", command)
		}
		if output, err := exec.CommandContext(ctx, command, args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("rename filesystem: %s", strings.TrimSpace(string(output)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		labelOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "LABEL", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("verify filesystem label: %s", strings.TrimSpace(string(labelOutput)))
		}
		if strings.TrimSpace(string(labelOutput)) != label {
			return "", fmt.Errorf("filesystem label verification failed")
		}
		return "filesystem renamed", nil

	case "storage.format":
		if request.Confirm != "FORMAT "+device {
			return "", errors.New("format confirmation does not match device")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to format a device on the system disk")
		}
		if err := prepareDestructiveChange(ctx, device); err != nil {
			return "", err
		}
		label := strings.TrimSpace(request.Label)
		if !validFilesystemLabel(label) {
			return "", errors.New("invalid filesystem label")
		}

		var command string
		var args []string
		switch strings.ToLower(strings.TrimSpace(request.Filesystem)) {
		case "ext4":
			command = "/usr/sbin/mkfs.ext4"
			args = []string{"-F"}
			if label != "" {
				args = append(args, "-L", label)
			}
		case "xfs":
			command = "/usr/sbin/mkfs.xfs"
			args = []string{"-f"}
			if label != "" {
				args = append(args, "-L", label)
			}
		case "vfat":
			command = "/usr/sbin/mkfs.vfat"
			if label != "" {
				args = append(args, "-n", label)
			}
		default:
			return "", errors.New("unsupported filesystem; use ext4, xfs or vfat")
		}
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("filesystem tool is unavailable: %s", command)
		}
		args = append(args, device)
		if output, err := exec.CommandContext(ctx, command, args...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("format device: %s", strings.TrimSpace(string(output)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		actualOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "FSTYPE", device).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("verify filesystem: %s", strings.TrimSpace(string(actualOutput)))
		}
		actual := strings.ToLower(strings.TrimSpace(string(actualOutput)))
		expected := strings.ToLower(strings.TrimSpace(request.Filesystem))
		if expected == "vfat" && (actual == "fat" || actual == "fat32") {
			actual = "vfat"
		}
		if actual != expected {
			return "", fmt.Errorf("filesystem verification failed: expected %s, got %s", expected, actual)
		}
		if label != "" {
			labelOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "LABEL", device).CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("verify filesystem label: %s", strings.TrimSpace(string(labelOutput)))
			}
			if strings.TrimSpace(string(labelOutput)) != label {
				return "", errors.New("filesystem label verification failed")
			}
		}
		return "device formatted", nil
	default:
		return "", errors.New("unsupported storage operation")
	}
}

func requireDiskType(ctx context.Context, device string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "TYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect disk type: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "disk" {
		return errors.New("partition creation is allowed only on physical disks")
	}
	return nil
}

func requirePartitionType(ctx context.Context, device string) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "TYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect partition type: %s", strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(string(output)) != "part" {
		return errors.New("partition deletion is allowed only for partitions")
	}
	return nil
}

func diskHasMountedDescendants(ctx context.Context, device string) (bool, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "MOUNTPOINTS", device).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect disk mounts: %s", strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) != "" {
			return true, nil
		}
	}
	return false, nil
}

func remainingDiskBytes(ctx context.Context, disk string) (uint64, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-bnro", "TYPE,SIZE", disk).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("inspect disk capacity: %s", strings.TrimSpace(string(output)))
	}
	var total uint64
	var allocated uint64
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		if fields[0] == "disk" && total == 0 {
			total = value
		}
		if fields[0] == "part" {
			allocated += value
		}
	}
	if total == 0 {
		return 0, errors.New("cannot determine disk capacity")
	}
	if allocated >= total {
		return 0, nil
	}
	return total - allocated, nil
}

func createPartition(ctx context.Context, disk string, sizeMiB uint64) error {
	before, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}

	pttypeOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PTTYPE", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect partition table: %s", strings.TrimSpace(string(pttypeOutput)))
	}
	pttype := strings.TrimSpace(string(pttypeOutput))

	var args []string
	if pttype == "" {
		args = []string{"--label", "gpt", disk}
	} else {
		args = []string{"--append", disk}
	}

	line := ",\n"
	if sizeMiB > 0 {
		line = fmt.Sprintf(",%dMiB\n", sizeMiB)
	}
	command := exec.CommandContext(ctx, "/usr/sbin/sfdisk", args...)
	command.Stdin = strings.NewReader(line)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create partition: %s", strings.TrimSpace(string(output)))
	}

	reloadOutput, err := exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"partition was written but kernel could not reload the table: %s",
			strings.TrimSpace(string(reloadOutput)),
		)
	}
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()

	after, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}
	if len(after) <= len(before) {
		return errors.New("partition table was changed but the new partition is not visible to the kernel")
	}
	return nil
}

func prepareDestructiveChange(ctx context.Context, device string) error {
	activeSwaps, err := activeSwapDevices()
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "NAME,FSTYPE", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect device usage: %s", strings.TrimSpace(string(output)))
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		fstype := ""
		if len(fields) > 1 {
			fstype = fields[1]
		}
		if fstype == "swap" {
			if !devicePathInSet(name, activeSwaps) {
				continue
			}
			if out, err := exec.CommandContext(ctx, "/usr/sbin/swapoff", "--", name).CombinedOutput(); err != nil {
				message := strings.TrimSpace(string(out))
				if message == "" {
					message = err.Error()
				}
				return fmt.Errorf("disable active swap %s: %s", name, message)
			}
			continue
		}
		targets, err := mountedTargets(ctx, name)
		if err != nil {
			return err
		}
		if len(targets) != 0 {
			if out, err := exec.CommandContext(ctx, "/usr/bin/umount", "--", name).CombinedOutput(); err != nil {
				return fmt.Errorf("unmount %s: %s", name, strings.TrimSpace(string(out)))
			}
		}
	}
	if err := deactivateLVMOnDisk(ctx, device); err != nil {
		return err
	}
	return nil
}

func activeSwapDevices() (map[string]bool, error) {
	data, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return nil, fmt.Errorf("inspect active swap devices: %w", err)
	}
	result := map[string]bool{}
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		if index == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		path := fields[0]
		result[path] = true
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			result[resolved] = true
		}
	}
	return result, nil
}

func devicePathInSet(device string, paths map[string]bool) bool {
	if paths[device] {
		return true
	}
	resolved, err := filepath.EvalSymlinks(device)
	if err != nil {
		return false
	}
	return paths[resolved]
}

func deactivateLVMOnDisk(ctx context.Context, device string) error {
	if _, err := os.Stat("/usr/sbin/pvs"); err != nil {
		return nil
	}
	if _, err := os.Stat("/usr/sbin/vgchange"); err != nil {
		return nil
	}

	targetDisk, err := topPhysicalDisk(ctx, device)
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(
		ctx,
		"/usr/sbin/pvs",
		"--noheadings",
		"--separator", "|",
		"-o", "pv_name,vg_name",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect LVM physical volumes: %s", strings.TrimSpace(string(output)))
	}

	type pvVG struct {
		pv string
		vg string
	}
	var mappings []pvVG
	targetVGs := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		pv := strings.TrimSpace(parts[0])
		vg := strings.TrimSpace(parts[1])
		if pv == "" || vg == "" || !strings.HasPrefix(pv, "/dev/") {
			continue
		}
		mappings = append(mappings, pvVG{pv: pv, vg: vg})
		disk, err := topPhysicalDisk(ctx, pv)
		if err != nil {
			continue
		}
		if disk == targetDisk {
			targetVGs[vg] = true
		}
	}

	for vg := range targetVGs {
		for _, mapping := range mappings {
			if mapping.vg != vg {
				continue
			}
			disk, err := topPhysicalDisk(ctx, mapping.pv)
			if err != nil {
				return fmt.Errorf("resolve LVM physical volume %s: %w", mapping.pv, err)
			}
			if disk != targetDisk {
				return fmt.Errorf("refusing to deactivate LVM volume group %s because it also uses another physical disk", vg)
			}
		}
		out, err := exec.CommandContext(ctx, "/usr/sbin/vgchange", "-an", "--", vg).CombinedOutput()
		if err != nil {
			return fmt.Errorf("deactivate LVM volume group %s: %s", vg, strings.TrimSpace(string(out)))
		}
		_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
		attrOutput, err := exec.CommandContext(
			ctx,
			"/usr/sbin/lvs",
			"--noheadings",
			"-o", "lv_attr",
			"--select", "vg_name="+vg,
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf("verify LVM volume group %s: %s", vg, strings.TrimSpace(string(attrOutput)))
		}
		for _, line := range strings.Split(string(attrOutput), "\n") {
			attr := strings.TrimSpace(line)
			if len(attr) > 4 && attr[4] == 'a' {
				return fmt.Errorf("LVM volume group %s is still active after deactivation", vg)
			}
		}
	}
	return nil
}

func deleteAllPartitions(ctx context.Context, disk string) error {
	before, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, "/usr/sbin/sfdisk", "--delete", disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete all partitions: %s", strings.TrimSpace(string(output)))
	}

	if err := rereadPartitionTable(ctx, disk); err != nil {
		return err
	}

	after, err := partitionNames(ctx, disk)
	if err != nil {
		return err
	}
	if len(after) != 0 {
		return fmt.Errorf("kernel still reports partitions after deletion: %s", strings.Join(after, ", "))
	}
	if len(before) == 0 {
		return errors.New("disk has no partitions to delete")
	}
	return nil
}

func deletePartition(ctx context.Context, partition string) error {
	parentOutput, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PKNAME", partition).CombinedOutput()
	if err != nil {
		return fmt.Errorf("resolve parent disk: %s", strings.TrimSpace(string(parentOutput)))
	}
	parent := strings.TrimSpace(string(parentOutput))
	if parent == "" {
		return errors.New("cannot resolve parent disk")
	}
	parentDevice := filepath.Join("/dev", parent)
	if err := requireDiskType(ctx, parentDevice); err != nil {
		return err
	}

	partNumberData, err := os.ReadFile(filepath.Join("/sys/class/block", filepath.Base(partition), "partition"))
	if err != nil {
		return fmt.Errorf("resolve partition number: %w", err)
	}
	partNumber := strings.TrimSpace(string(partNumberData))
	if partNumber == "" {
		return errors.New("cannot resolve partition number")
	}

	output, err := exec.CommandContext(ctx, "/usr/sbin/sfdisk", "--delete", parentDevice, partNumber).CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete partition: %s", strings.TrimSpace(string(output)))
	}
	if err := rereadPartitionTable(ctx, parentDevice); err != nil {
		return err
	}
	if _, err := os.Stat(partition); err == nil {
		return fmt.Errorf("kernel still reports partition %s after deletion", partition)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verify deleted partition: %w", err)
	}
	return nil
}

func rereadPartitionTable(ctx context.Context, disk string) error {
	output, err := exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	if err != nil {
		// partx can remove stale kernel partition mappings after the on-disk table
		// has changed. It still refuses entries that are genuinely busy.
		_, _ = exec.CommandContext(ctx, "/usr/bin/partx", "--delete", disk).CombinedOutput()
		output, err = exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).CombinedOutput()
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("partition table changed on disk but kernel could not reload it; close/deactivate volumes that still use this disk: %s", message)
	}
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
	return nil
}

func partitionNames(ctx context.Context, disk string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-nrpo", "NAME,TYPE", disk).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect disk partitions: %s", strings.TrimSpace(string(output)))
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "part" {
			names = append(names, fields[0])
		}
	}
	return names, nil
}

func validateBlockDevice(value string) (string, error) {
	device := filepath.Clean(strings.TrimSpace(value))
	if device == "." || !strings.HasPrefix(device, "/dev/") {
		return "", errors.New("invalid block device path")
	}
	info, err := os.Stat(device)
	if err != nil {
		return "", fmt.Errorf("stat block device: %w", err)
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return "", errors.New("requested path is not a block device")
	}
	return device, nil
}

func mountedTargets(ctx context.Context, device string) ([]string, error) {
	output, err := exec.CommandContext(ctx, "/usr/bin/findmnt", "-rn", "-S", device, "-o", "TARGET").CombinedOutput()
	if err != nil {
		// findmnt exits non-zero when the source has no mounts.
		if len(strings.TrimSpace(string(output))) == 0 {
			return []string{}, nil
		}
		return nil, fmt.Errorf("inspect mounts: %s", strings.TrimSpace(string(output)))
	}
	var targets []string
	for _, line := range strings.Split(string(output), "\n") {
		if target := strings.TrimSpace(line); target != "" {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func samePhysicalDiskAsRoot(ctx context.Context, device string) (bool, error) {
	rootOutput, err := exec.CommandContext(ctx, "/usr/bin/findmnt", "-rn", "-o", "SOURCE", "/").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("resolve root filesystem: %s", strings.TrimSpace(string(rootOutput)))
	}
	rootSource := strings.TrimSpace(string(rootOutput))
	if index := strings.Index(rootSource, "["); index >= 0 {
		rootSource = rootSource[:index]
	}
	if !strings.HasPrefix(rootSource, "/dev/") {
		return false, errors.New("cannot safely resolve the system disk")
	}
	rootDisk, err := topPhysicalDisk(ctx, rootSource)
	if err != nil {
		return false, err
	}
	targetDisk, err := topPhysicalDisk(ctx, device)
	if err != nil {
		return false, err
	}
	return rootDisk == targetDisk, nil
}

func topPhysicalDisk(ctx context.Context, device string) (string, error) {
	current := device
	for i := 0; i < 16; i++ {
		output, err := exec.CommandContext(ctx, "/usr/bin/lsblk", "-ndo", "PKNAME", current).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("resolve parent disk: %s", strings.TrimSpace(string(output)))
		}
		parent := strings.TrimSpace(string(output))
		if parent == "" {
			return filepath.Base(current), nil
		}
		current = filepath.Join("/dev", parent)
	}
	return "", errors.New("block device parent chain is too deep")
}

func validFilesystemLabelForType(value, filesystem string) bool {
	limit := 32
	switch filesystem {
	case "ext4":
		limit = 16
	case "xfs":
		limit = 12
	case "vfat", "fat", "fat32":
		limit = 11
	}
	if len(value) == 0 || len(value) > limit {
		return false
	}
	return validFilesystemLabel(value)
}

func validFilesystemLabel(value string) bool {
	if len(value) > 32 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == ' ' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validVersion(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func serviceIdentity() (int, int, error) {
	account, err := user.Lookup(serviceUser)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, err
	}
	group, err := user.LookupGroup(serviceUser)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var (
		uid uint32
		credErr error
	)
	if err := raw.Control(func(fd uintptr) {
		cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			credErr = err
			return
		}
		uid = cred.Uid
	}); err != nil {
		return 0, err
	}
	return uid, credErr
}

func prepareSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("refusing to replace non-socket updater path")
	}
	return os.Remove(path)
}
