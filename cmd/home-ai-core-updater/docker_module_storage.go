package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	_ "modernc.org/sqlite"
)

// NVR storage is authoritative only when its assigned filesystem UUID is
// mounted on a distinct device at an exact managed mountpoint.
var (
	nvrStorageDBPath = "/var/lib/home-ai-core/core.db"
	nvrStorageRoot   = "/mnt/home-ai-core"
	nvrUUIDPattern   = regexp.MustCompile(`(?i)^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
)

const nvrDiskDataName = ".home-ai-nvr-data"

func assignedNVRUUID(ctx context.Context) (string, error) {
	db, err := sql.Open("sqlite", "file:"+nvrStorageDBPath+"?mode=ro")
	if err != nil {
		return "", fmt.Errorf("open storage assignments: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT filesystem_uuid FROM storage_purposes WHERE purpose = 'nvr'")
	if err != nil {
		return "", fmt.Errorf("read NVR disk assignment: %w", err)
	}
	defer rows.Close()
	uuid := ""
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return "", err
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if !nvrUUIDPattern.MatchString(value) {
			return "", errors.New("invalid NVR filesystem UUID")
		}
		if uuid != "" {
			return "", errors.New("more than one NVR disk assigned; migration supports exactly one")
		}
		uuid = value
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if uuid == "" {
		return "", errors.New("no NVR disk assigned in HomeAiCore")
	}
	return uuid, nil
}

func diskFileID(info os.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

func assignedNVRDestination(ctx context.Context, uuid string) (string, error) {
	if !nvrUUIDPattern.MatchString(uuid) {
		return "", errors.New("invalid assigned NVR UUID")
	}
	root, err := os.Stat(nvrStorageRoot)
	if err != nil || !root.IsDir() {
		return "", errors.New("managed storage root unavailable")
	}
	rootID, ok := diskFileID(root)
	if !ok {
		return "", errors.New("managed storage identity unavailable")
	}
	entries, err := os.ReadDir(nvrStorageRoot)
	if err != nil {
		return "", err
	}
	selected := ""
	for _, e := range entries {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			continue
		}
		mp := filepath.Join(nvrStorageRoot, e.Name())
		info, err := os.Lstat(mp)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		// -M requires the exact mountpoint (unlike -T, which may return /).
		out, err := exec.CommandContext(ctx, "findmnt", "-rn", "-M", mp, "-o", "UUID").Output()
		if err != nil || !strings.EqualFold(strings.TrimSpace(string(out)), uuid) {
			continue
		}
		out, err = exec.CommandContext(ctx, "findmnt", "-rn", "-M", mp, "-o", "FSTYPE").Output()
		if err != nil || (strings.TrimSpace(string(out)) != "ext4" && strings.TrimSpace(string(out)) != "xfs") {
			return "", errors.New("assigned NVR filesystem must be ext4 or xfs")
		}
		id, ok := diskFileID(info)
		if !ok || id == rootID {
			return "", errors.New("NVR mount is on the system filesystem")
		}
		if selected != "" {
			return "", errors.New("NVR UUID has multiple managed mountpoints")
		}
		selected = filepath.Join(mp, nvrDiskDataName)
	}
	if selected == "" {
		return "", errors.New("assigned NVR disk is not mounted at /mnt/home-ai-core")
	}
	return selected, nil
}

func validateNVRDataLink(ctx context.Context, dataDir string) (string, error) {
	uuid, err := assignedNVRUUID(ctx)
	if err != nil {
		return "", err
	}
	target, err := assignedNVRDestination(ctx, uuid)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(dataDir)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", errors.New("NVR data remains on system disk; activate assigned storage in Installed modules")
	}
	link, err := os.Readlink(dataDir)
	if err != nil || link != target {
		return "", errors.New("NVR data link does not match assigned disk")
	}
	info, err = os.Lstat(target)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("NVR disk data directory is unavailable")
	}
	return target, nil
}

func prepareFreshNVRStorage(ctx context.Context, dataDir string, uid, gid int) error {
	if _, err := validateNVRDataLink(ctx, dataDir); err == nil {
		return nil
	}
	if _, err := os.Lstat(dataDir); err == nil {
		return errors.New("existing NVR data must be migrated before reinstalling")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	uuid, err := assignedNVRUUID(ctx)
	if err != nil {
		return err
	}
	target, err := assignedNVRDestination(ctx, uuid)
	if err != nil {
		return err
	}
	if err := os.Mkdir(target, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("NVR disk target is not a directory")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return errors.New("NVR disk target contains data; recovery required")
	}
	if err := os.Chown(target, uid, gid); err != nil {
		return err
	}
	if _, err := assignedNVRDestination(ctx, uuid); err != nil {
		return err
	}
	return os.Symlink(target, dataDir)
}

func verifyNVRContainerMount(ctx context.Context, dockerPath, name, dataDir string) error {
	out, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{json .Mounts}}", name).Output()
	if err != nil {
		return fmt.Errorf("inspect NVR data mount: %w", err)
	}
	var mounts []struct{ Type, Source, Destination string }
	if err := json.Unmarshal(out, &mounts); err != nil {
		return err
	}
	for _, m := range mounts {
		if m.Destination != "/data" {
			continue
		}
		if m.Type != "bind" {
			return errors.New("NVR /data is not a managed bind")
		}
		if m.Source == dataDir {
			return nil
		}
		if target, err := validateNVRDataLink(ctx, dataDir); err == nil && m.Source == target {
			return nil
		}
		return errors.New("NVR container has an unexpected /data source")
	}
	return errors.New("NVR container has no /data bind")
}

// All copied entries must match. Do not follow symlinks as root when checking.
func verifyNVRCopy(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		copyInfo, err := os.Lstat(filepath.Join(destination, rel))
		if err != nil {
			return err
		}
		if info.Mode().Type() != copyInfo.Mode().Type() {
			return fmt.Errorf("NVR copy type mismatch: %s", rel)
		}
		switch {
		case info.IsDir():
			return nil
		case info.Mode().IsRegular():
			if info.Size() != copyInfo.Size() {
				return fmt.Errorf("NVR copy size mismatch: %s", rel)
			}
			a, err := safeNVRHash(path)
			if err != nil {
				return err
			}
			b, err := safeNVRHash(filepath.Join(destination, rel))
			if err != nil {
				return err
			}
			if a != b {
				return fmt.Errorf("NVR copy checksum mismatch: %s", rel)
			}
		case info.Mode()&os.ModeSymlink != 0:
			a, err := os.Readlink(path)
			if err != nil {
				return err
			}
			b, err := os.Readlink(filepath.Join(destination, rel))
			if err != nil {
				return err
			}
			if a != b {
				return fmt.Errorf("NVR copy link mismatch: %s", rel)
			}
		default:
			return fmt.Errorf("unsupported NVR data entry: %s", rel)
		}
		return nil
	})
}

func safeNVRHash(path string) ([32]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return [32]byte{}, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return sum, nil
}

func inspectNVRDataDevice(ctx context.Context, dockerPath, name, dataDir string) error {
	target, err := validateNVRDataLink(ctx, dataDir)
	if err != nil {
		return err
	}
	fi, err := os.Stat(target)
	if err != nil {
		return err
	}
	want, ok := diskFileID(fi)
	if !ok {
		return errors.New("NVR disk device ID unavailable")
	}
	out, err := exec.CommandContext(ctx, dockerPath, "exec", name, "python3", "-c", "import os; print(os.stat('/data').st_dev)").Output()
	if err != nil {
		return fmt.Errorf("inspect actual container disk: %w", err)
	}
	var got uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &got); err != nil || got != want {
		return errors.New("NVR container still uses the wrong filesystem")
	}
	return nil
}

func waitNVRMigrationHealth(ctx context.Context, dockerPath, name, dataDir string) error {
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		out, err := exec.CommandContext(ctx, dockerPath, "inspect", "--format", "{{.State.Health.Status}}", name).Output()
		if err != nil {
			return err
		}
		status := strings.TrimSpace(string(out))
		if status == "healthy" {
			return inspectNVRDataDevice(ctx, dockerPath, name, dataDir)
		}
		if status == "unhealthy" {
			return errors.New("NVR unhealthy after storage switch")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("NVR did not become healthy after storage switch")
}

// activateNVRStorage migrates the existing image and its SQLite database
// without deleting original data or replacing the container image.
func activateNVRStorage(ctx context.Context, moduleID string, uid, gid int) (result moduleContainerStatus, message string, err error) {
	if moduleID != "nvr.module" {
		return result, "", errors.New("storage activation is restricted to NVR")
	}
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return result, "", err
	}
	name := moduleContainerName(moduleID)
	exists, managed, err := inspectManagedContainer(ctx, dockerPath, name, moduleID)
	if err != nil {
		return result, "", err
	}
	if !exists || !managed {
		return result, "", errors.New("managed NVR container not found")
	}
	dataDir := filepath.Join(moduleDataRoot, moduleID)
	if err := verifyNVRContainerMount(ctx, dockerPath, name, dataDir); err != nil {
		return result, "", err
	}
	uuid, err := assignedNVRUUID(ctx)
	if err != nil {
		return result, "", err
	}
	target, err := assignedNVRDestination(ctx, uuid)
	if err != nil {
		return result, "", err
	}
	if _, err := validateNVRDataLink(ctx, dataDir); err == nil {
		if err := inspectNVRDataDevice(ctx, dockerPath, name, dataDir); err != nil {
			return result, "", err
		}
		result, err := inspectModuleContainer(ctx, dockerPath, name)
		return result, "NVR already uses its assigned disk", err
	}
	fi, err := os.Lstat(dataDir)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return result, "", errors.New("NVR source data directory requires manual recovery")
	}
	if _, err := os.Lstat(target); err == nil {
		return result, "", errors.New("NVR disk directory already exists; refusing to overwrite recordings")
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, "", err
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		return result, "", err
	}
	if err := os.Chown(target, uid, gid); err != nil {
		return result, "", err
	}

	out, err := exec.CommandContext(ctx, dockerPath, "stop", "-t", "30", name).CombinedOutput()
	if err != nil {
		return result, "", fmt.Errorf("stop NVR: %s", commandError(out, err))
	}
	stopped := true
	backup := dataDir + ".pre-storage-" + fmt.Sprint(time.Now().UTC().UnixNano())
	renamed, linked := false, false
	defer func() {
		if err == nil || !stopped {
			return
		}
		restoreCtx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
		defer cancel()
		var restoreErr error
		if linked {
			_, _ = exec.CommandContext(restoreCtx, dockerPath, "stop", "-t", "15", name).CombinedOutput()
			if e := os.Remove(dataDir); e != nil {
				restoreErr = errors.Join(restoreErr, e)
			}
		}
		if renamed {
			if e := os.Rename(backup, dataDir); e != nil {
				restoreErr = errors.Join(restoreErr, e)
			}
		}
		if restoreErr == nil {
			if output, e := exec.CommandContext(restoreCtx, dockerPath, "start", name).CombinedOutput(); e != nil {
				restoreErr = fmt.Errorf("restart original NVR: %s", commandError(output, e))
			}
		}
		if restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("NVR rollback needs manual recovery: %w", restoreErr))
		}
	}()
	out, copyErr := exec.CommandContext(ctx, "cp", "-a", "--", dataDir+"/.", target).CombinedOutput()
	if copyErr != nil {
		return result, "", fmt.Errorf("copy NVR data: %s", commandError(out, copyErr))
	}
	if e := verifyNVRCopy(dataDir, target); e != nil {
		return result, "", fmt.Errorf("verify NVR backup: %w", e)
	}
	if actual, e := assignedNVRDestination(ctx, uuid); e != nil || actual != target {
		return result, "", errors.New("assigned NVR disk changed during migration")
	}
	if e := os.Rename(dataDir, backup); e != nil {
		return result, "", e
	}
	renamed = true
	if e := os.Symlink(target, dataDir); e != nil {
		return result, "", e
	}
	linked = true
	if _, e := validateNVRDataLink(ctx, dataDir); e != nil {
		return result, "", e
	}
	out, err = exec.CommandContext(ctx, dockerPath, "start", name).CombinedOutput()
	if err != nil {
		return result, "", fmt.Errorf("start migrated NVR: %s", commandError(out, err))
	}
	if err := waitNVRMigrationHealth(ctx, dockerPath, name, dataDir); err != nil {
		return result, "", err
	}
	result, err = inspectModuleContainer(ctx, dockerPath, name)
	if err != nil {
		return result, "", err
	}
	stopped = false
	return result, "NVR now uses assigned disk; original data retained in " + backup, nil
}
