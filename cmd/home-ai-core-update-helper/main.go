package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updatehelper"
)

const (
	defaultSocket = "/run/home-ai-core-update.sock"
	serviceUser   = "home-ai-core"
	maxRequest    = 32 << 10
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if os.Geteuid() != 0 {
		logger.Error("update helper must run as root")
		os.Exit(1)
	}

	uid, gid, err := serviceIdentity()
	if err != nil {
		logger.Error("failed to resolve service identity", "error", err)
		os.Exit(1)
	}

	socketPath := defaultSocket
	if len(os.Args) == 3 && os.Args[1] == "--socket" {
		socketPath = os.Args[2]
	} else if len(os.Args) != 1 {
		logger.Error("usage: home-ai-core-update-helper [--socket PATH]")
		os.Exit(2)
	}

	if err := prepareSocketPath(socketPath); err != nil {
		logger.Error("failed to prepare update helper socket", "error", err)
		os.Exit(1)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		logger.Error("failed to listen on update helper socket", "error", err)
		os.Exit(1)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	if err := os.Chown(socketPath, 0, gid); err != nil {
		logger.Error("failed to set update helper socket owner", "error", err)
		os.Exit(1)
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		logger.Error("failed to set update helper socket mode", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	logger.Info("update helper ready", "socket", socketPath)
	conn, err := listener.AcceptUnix()
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		logger.Error("accept failed", "error", err)
		os.Exit(1)
	}
	// Handle one privileged request and exit. systemd restarts the service,
	// which also guarantees that an upgraded helper binary is picked up
	// immediately after a successful self-update.
	handleConnection(ctx, logger, conn, uint32(uid))
}

func handleConnection(parent context.Context, logger *slog.Logger, conn *net.UnixConn, expectedUID uint32) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Minute))

	uid, err := peerUID(conn)
	if err != nil || uid != expectedUID {
		logger.Warn("rejected update helper client", "uid", uid, "error", err)
		_ = json.NewEncoder(conn).Encode(updatehelper.Response{Error: "unauthorized update helper client"})
		return
	}

	reader := io.LimitReader(bufio.NewReader(conn), maxRequest+1)
	var request updatehelper.Request
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(updatehelper.Response{Error: "invalid helper request"})
		return
	}

	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
	defer cancel()
	message, err := installUpdate(ctx, request)
	if err != nil {
		logger.Error("core update failed", "version", request.Version, "error", err)
		_ = json.NewEncoder(conn).Encode(updatehelper.Response{Error: err.Error()})
		return
	}
	logger.Info("core update accepted", "version", request.Version, "message", message)
	_ = json.NewEncoder(conn).Encode(updatehelper.Response{OK: true, Message: message})
}

func installUpdate(ctx context.Context, request updatehelper.Request) (string, error) {
	if request.Operation != "install-core-update" {
		return "", errors.New("unsupported helper operation")
	}
	if request.Architecture != runtime.GOARCH {
		return "", errors.New("package architecture does not match host")
	}
	path, err := validateStagedPackage(request.PackagePath, request.SHA256)
	if err != nil {
		return "", err
	}

	fields, err := dpkgFields(ctx, path)
	if err != nil {
		return "", err
	}
	if fields["Package"] != "home-ai-core" {
		return "", errors.New("helper only installs the home-ai-core package")
	}
	expectedArch := runtime.GOARCH
	if expectedArch == "amd64" || expectedArch == "arm64" {
		if fields["Architecture"] != expectedArch {
			return "", fmt.Errorf("Debian package architecture %q does not match %q", fields["Architecture"], expectedArch)
		}
	}
	if fields["Version"] != request.DebianVersion {
		return "", fmt.Errorf("Debian package version %q does not match expected %q", fields["Version"], request.DebianVersion)
	}

	installed, err := installedVersion(ctx)
	if err != nil {
		return "", err
	}
	if installed == request.DebianVersion {
		return "requested version is already installed", nil
	}
	if !dpkgVersionGreater(ctx, request.DebianVersion, installed) {
		return "", fmt.Errorf("refusing non-upgrade from %q to %q", installed, request.DebianVersion)
	}

	cmd := exec.CommandContext(
		ctx,
		"/usr/bin/apt-get",
		"-y",
		"-o", "Dpkg::Options::=--force-confold",
		"--no-install-recommends",
		"install",
		path,
	)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 4000 {
			message = message[len(message)-4000:]
		}
		return "", fmt.Errorf("apt-get failed: %s", message)
	}
	return "package installed", nil
}

func validateStagedPackage(path, expectedHash string) (string, error) {
	clean := filepath.Clean(path)
	prefix := "/var/lib/home-ai-core/updates" + string(os.PathSeparator)
	if !strings.HasPrefix(clean, prefix) || filepath.Ext(clean) != ".deb" {
		return "", errors.New("package path is outside the update staging directory")
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", fmt.Errorf("stat package: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("staged package must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "", errors.New("staged package must not be group/world writable")
	}

	file, err := os.Open(clean)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	if hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(expectedHash) {
		return "", errors.New("staged package SHA-256 mismatch")
	}
	return clean, nil
}

func dpkgFields(ctx context.Context, path string) (map[string]string, error) {
	fields := make(map[string]string, 3)
	for _, field := range []string{"Package", "Version", "Architecture"} {
		cmd := exec.CommandContext(ctx, "/usr/bin/dpkg-deb", "-f", path, field)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("inspect Debian package field %s: %w", field, err)
		}
		value := strings.TrimSpace(string(output))
		if value == "" {
			return nil, fmt.Errorf("Debian package field %s is empty", field)
		}
		fields[field] = value
	}
	return fields, nil
}

func installedVersion(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/dpkg-query", "-W", "-f=${Version}", "home-ai-core")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read installed Home-AI-Core version: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func dpkgVersionGreater(ctx context.Context, left, right string) bool {
	return exec.CommandContext(ctx, "/usr/bin/dpkg", "--compare-versions", left, "gt", right).Run() == nil
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
		uid     uint32
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

func prepareSocketPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("refusing to replace non-socket helper path")
	}
	return os.Remove(path)
}
