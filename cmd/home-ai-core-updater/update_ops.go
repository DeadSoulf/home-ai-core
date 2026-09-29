package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

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
