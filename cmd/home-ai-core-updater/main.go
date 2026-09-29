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
