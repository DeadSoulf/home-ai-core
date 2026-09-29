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
	socketPath  = "/run/home-ai-core-updater.sock"
	updateRoot  = "/var/lib/home-ai-core/update"
	liveBinary  = "/usr/bin/home-ai-core"
	liveWeb     = "/usr/share/home-ai-core/web"
	serviceName = "home-ai-core.service"
	serviceUser = "home-ai-core"
	maxRequest  = 16 << 10
)

var installMu sync.Mutex

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

	logger.Info("bundle updater helper ready", "socket", socketPath)
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
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: "invalid updater request"})
		return
	}
	if strings.HasPrefix(request.Operation, "storage.") {
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
		if err := performInstall(ctx, request.Version, prepared, uid, gid); err != nil {
			logger.Error("bundle update failed", "version", request.Version, "error", err)
			_ = writeResult(uid, gid, updaterhelper.Result{
				Status:    "failed",
				Version:   request.Version,
				Error:     err.Error(),
				UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
			return
		}
		logger.Info("bundle update installed", "version", request.Version)
		_ = writeResult(uid, gid, updaterhelper.Result{
			Status:    "succeeded",
			Version:   request.Version,
			Message:   "update installed",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	}()
}

func performInstall(ctx context.Context, version, prepared string, uid, gid int) error {
	backup := filepath.Join(updateRoot, "backup")
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(backup, "bin"), 0o700); err != nil {
		return err
	}
	if err := copyFile(liveBinary, filepath.Join(backup, "bin", "home-ai-core"), 0o700); err != nil {
		return fmt.Errorf("backup core binary: %w", err)
	}
	if err := copyTree(liveWeb, filepath.Join(backup, "web")); err != nil {
		return fmt.Errorf("backup web ui: %w", err)
	}

	newBinary := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.new")
	if err := copyFile(filepath.Join(prepared, "bin", "home-ai-core"), newBinary, 0o755); err != nil {
		return fmt.Errorf("stage new core binary: %w", err)
	}
	newWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-new")
	rollbackWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-rollback")
	_ = os.RemoveAll(newWeb)
	_ = os.RemoveAll(rollbackWeb)
	if err := copyTree(filepath.Join(prepared, "web"), newWeb); err != nil {
		return fmt.Errorf("stage new web ui: %w", err)
	}

	if err := systemctl(ctx, "stop", serviceName); err != nil {
		return fmt.Errorf("stop Home-AI-Core: %w", err)
	}

	rollbackNeeded := true
	defer func() {
		if rollbackNeeded {
			rollback(context.Background(), backup, rollbackWeb)
		}
	}()

	if err := os.Rename(newBinary, liveBinary); err != nil {
		return fmt.Errorf("replace core binary: %w", err)
	}
	if err := os.Rename(liveWeb, rollbackWeb); err != nil {
		return fmt.Errorf("preserve current web ui: %w", err)
	}
	if err := os.Rename(newWeb, liveWeb); err != nil {
		_ = os.Rename(rollbackWeb, liveWeb)
		return fmt.Errorf("replace web ui: %w", err)
	}

	if err := systemctl(ctx, "start", serviceName); err != nil {
		return fmt.Errorf("start updated Home-AI-Core: %w", err)
	}
	if err := waitActive(ctx, serviceName, 15*time.Second); err != nil {
		return err
	}

	rollbackNeeded = false
	_ = os.RemoveAll(rollbackWeb)
	_ = os.Chown(filepath.Join(updateRoot, "backup"), uid, gid)
	return nil
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
		if request.SizeMiB > 0 && request.SizeMiB > 16*1024*1024 {
			return "", errors.New("partition size is too large")
		}
		if err := createPartition(ctx, device, request.SizeMiB); err != nil {
			return "", err
		}
		return "partition created", nil

	case "storage.partition.delete":
		if request.Confirm != "DELETE "+device {
			return "", errors.New("partition deletion confirmation does not match device")
		}
		if err := requirePartitionType(ctx, device); err != nil {
			return "", err
		}
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("partition must be unmounted before deletion")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to delete a partition on the system disk")
		}
		if err := deletePartition(ctx, device); err != nil {
			return "", err
		}
		return "partition deleted", nil

	case "storage.format":
		if request.Confirm != "FORMAT "+device {
			return "", errors.New("format confirmation does not match device")
		}
		targets, err := mountedTargets(ctx, device)
		if err != nil {
			return "", err
		}
		if len(targets) != 0 {
			return "", errors.New("device must be unmounted before formatting")
		}
		protected, err := samePhysicalDiskAsRoot(ctx, device)
		if err != nil {
			return "", err
		}
		if protected {
			return "", errors.New("refusing to format a device on the system disk")
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

func createPartition(ctx context.Context, disk string, sizeMiB uint64) error {
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
	_ = exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", disk).Run()
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
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
	_ = exec.CommandContext(ctx, "/usr/sbin/blockdev", "--rereadpt", parentDevice).Run()
	_ = exec.CommandContext(ctx, "/usr/bin/udevadm", "settle").Run()
	return nil
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
