package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	maxRequest        = 256 << 10
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
	policyCtx, policyCancel := context.WithTimeout(ctx, 30*time.Second)
	if err := ensureNASServicePolicy(policyCtx); err != nil {
		logger.Error("reconcile NAS service policy", "error", err)
	}
	if _, err := performSMBOperation(policyCtx, updaterhelper.Request{Operation: "smb.suspend"}, uid, gid); err != nil {
		logger.Error("suspend stale SMB access", "error", err)
	}
	policyCancel()
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
	if request.Operation == "storage.nas.quota.inspect" {
		ctx, cancel := context.WithTimeout(parent, 40*time.Second)
		defer cancel()
		quota, err := inspectNASQuota(ctx, request)
		if err != nil {
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{OK: true, QuotaLimitBytes: quota.BlockHardLimit * 1024, QuotaUsedBytes: quota.CurrentSpace})
		return
	}
	if strings.HasPrefix(request.Operation, "storage.nas.") {
		_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
		ctx, cancel := context.WithTimeout(parent, 40*time.Second)
		defer cancel()
		message, err := performNASOperation(ctx, request, uid, gid)
		if err != nil {
			logger.Error("NAS operation failed", "operation", request.Operation, "root_path", request.RootPath, "relative_path", request.RelativePath, "error", err)
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		logger.Info("NAS operation completed", "operation", request.Operation, "root_path", request.RootPath, "relative_path", request.RelativePath)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{OK: true, Message: message})
		return
	}
	if strings.HasPrefix(request.Operation, "storage.") {
		_ = conn.SetDeadline(time.Now().Add(2*time.Minute + 15*time.Second))
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

	if request.Operation == "smb.inspect" {
		_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
		ctx, cancel := context.WithTimeout(parent, 40*time.Second)
		defer cancel()
		available, active, smbError, configuredUsers := inspectSMB(ctx, request.SMBUsers)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:                 true,
			Message:            "SMB inspected",
			HelperVersion:      updaterhelper.HelperVersion,
			ProtocolVersion:    updaterhelper.ProtocolVersion,
			SMBAvailable:       available,
			SMBActive:          active,
			SMBError:           smbError,
			SMBConfiguredUsers: configuredUsers,
		})
		return
	}
	if strings.HasPrefix(request.Operation, "smb.") {
		_ = conn.SetDeadline(time.Now().Add(12 * time.Minute))
		ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
		defer cancel()
		message, err := performSMBOperation(ctx, request, uid, gid)
		if err != nil {
			logger.Error("SMB operation failed", "operation", request.Operation, "error", err)
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		logger.Info("SMB operation completed", "operation", request.Operation)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{OK: true, Message: message})
		return
	}

	if request.Operation == "network.profile.inspect" {
		_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
		ctx, cancel := context.WithTimeout(parent, 40*time.Second)
		defer cancel()
		backend, profiles := inspectNetworkProfiles(ctx)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:              true,
			Message:         "network profiles inspected",
			HelperVersion:   updaterhelper.HelperVersion,
			ProtocolVersion: updaterhelper.ProtocolVersion,
			NetworkBackend:  backend,
			NetworkProfiles: profiles,
		})
		return
	}

	if request.Operation == "wireguard.inspect" {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		ctx, cancel := context.WithTimeout(parent, 25*time.Second)
		defer cancel()
		available, wireGuardError, tunnels := inspectWireGuard(ctx)
		_ = json.NewEncoder(conn).Encode(updaterhelper.Response{
			OK:                 true,
			Message:            "WireGuard inspected",
			HelperVersion:      updaterhelper.HelperVersion,
			ProtocolVersion:    updaterhelper.ProtocolVersion,
			WireGuardAvailable: available,
			WireGuardError:     wireGuardError,
			WireGuardTunnels:   tunnels,
		})
		return
	}
	if strings.HasPrefix(request.Operation, "network.") || strings.HasPrefix(request.Operation, "wireguard.") {
		_ = conn.SetDeadline(time.Now().Add(12 * time.Minute))
		ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
		defer cancel()
		message, err := performNetworkOperation(ctx, request)
		if err != nil {
			logger.Error("network operation failed", "operation", request.Operation, "interface", request.Interface, "tunnel", request.Tunnel, "error", err)
			_ = json.NewEncoder(conn).Encode(updaterhelper.Response{Error: err.Error()})
			return
		}
		logger.Info("network operation completed", "operation", request.Operation, "interface", request.Interface, "tunnel", request.Tunnel)
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
