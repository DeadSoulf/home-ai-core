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
	file, err := os.OpenFile(filepath.Join(backup, "metadata.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
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
	backup := filepath.Join(trustedUpdateRoot, "backup")
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

func performInstall(ctx context.Context, currentVersion, version, prepared string, uid, gid int) (helperUpdated bool, resultErr error) {
	rollbackWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-rollback")
	if err := refuseInterruptedBackupStaging(trustedUpdateRoot, rollbackWeb); err != nil {
		return false, err
	}
	// Prepare the replacement entirely before touching the running installation.
	newBinary := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.new")
	if err := copyFile(filepath.Join(prepared, "bin", "home-ai-core"), newBinary, 0o755); err != nil {
		return false, fmt.Errorf("stage new core binary: %w", err)
	}
	newWeb := filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-new")
	if err := os.RemoveAll(newWeb); err != nil {
		return false, fmt.Errorf("clear previous web staging: %w", err)
	}
	if err := copyTree(filepath.Join(prepared, "web"), newWeb); err != nil {
		return false, fmt.Errorf("stage new web ui: %w", err)
	}

	preparedHelper := filepath.Join(prepared, "helper", "home-ai-core-updater")
	newHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core-updater.new")
	includeHelper := false
	if info, err := os.Lstat(preparedHelper); err == nil {
		if !info.Mode().IsRegular() {
			return false, errors.New("staged updater helper must be a regular file")
		}
		if err := copyFile(preparedHelper, newHelper, 0o755); err != nil {
			return false, fmt.Errorf("stage updater helper: %w", err)
		}
		includeHelper = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect staged updater helper: %w", err)
	}

	// The committed rollback stays untouched until the new Core has started.
	// Any failure while copying the current Core leaves its old backup valid.
	stagedBackup, err := stageRollbackBackup(
		trustedUpdateRoot, liveBinary, liveWeb, liveHelper, currentVersion, version, includeHelper,
	)
	if err != nil {
		return false, err
	}

	rollbackNeeded := false
	preserveBackup := false
	defer func() {
		if rollbackNeeded {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			restoreErr := rollback(restoreCtx, stagedBackup, rollbackWeb)
			cancel()
			if restoreErr != nil {
				preserveBackup = true
				resultErr = errors.Join(resultErr,
					fmt.Errorf("automatic rollback failed; recovery snapshot retained at %s: %w", stagedBackup, restoreErr))
			}
		}
		if !preserveBackup {
			_ = os.RemoveAll(stagedBackup)
		}
	}()

	if err := systemctl(ctx, "stop", serviceName); err != nil {
		// A service manager error does not prove that the old service is still running.
		restartCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		restartErr := systemctl(restartCtx, "start", serviceName)
		cancel()
		return false, errors.Join(fmt.Errorf("stop Home-AI-Core: %w", err), restartErr)
	}

	rollbackNeeded = true
	if err := os.Rename(newBinary, liveBinary); err != nil {
		return false, fmt.Errorf("replace core binary: %w", err)
	}
	if err := os.Rename(liveWeb, rollbackWeb); err != nil {
		return false, fmt.Errorf("preserve current web ui: %w", err)
	}
	if err := os.Rename(newWeb, liveWeb); err != nil {
		return false, fmt.Errorf("replace web ui: %w", err)
	}
	if err := systemctl(ctx, "start", serviceName); err != nil {
		return false, fmt.Errorf("start updated Home-AI-Core: %w", err)
	}
	if err := waitActive(ctx, serviceName, 15*time.Second); err != nil {
		return false, err
	}

	if includeHelper {
		if err := os.Rename(newHelper, liveHelper); err != nil {
			return false, fmt.Errorf("replace updater helper: %w", err)
		}
		if err := os.Chmod(liveHelper, 0o755); err != nil {
			return false, fmt.Errorf("set updater helper permissions: %w", err)
		}
		helperUpdated = true
	}

	backup := filepath.Join(trustedUpdateRoot, "backup")
	if err := replaceDirectoryAtomically(stagedBackup, backup); err != nil {
		return false, fmt.Errorf("commit new rollback backup: %w", err)
	}
	// The commit above is the final fallible step. The previous backup now
	// resides at stagedBackup and can be cleaned up after returning.
	rollbackNeeded = false
	_ = os.RemoveAll(rollbackWeb)
	return helperUpdated, nil
}
func performManualRollback(ctx context.Context, backup string) (bool, error) {
	if _, ok := availableRollback(); !ok {
		return false, errors.New("rollback backup is incomplete")
	}

	current := filepath.Join(trustedUpdateRoot, "rollback-current")
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

func rollback(ctx context.Context, backup, rollbackWeb string) error {
	// Restore from the staged pre-update snapshot, not from the previous
	// committed rollback. Prepare everything before disturbing live files.
	stagedCore := filepath.Join(filepath.Dir(liveBinary), ".home-ai-core.rollback")
	if err := copyFile(filepath.Join(backup, "bin", "home-ai-core"), stagedCore, 0o755); err != nil {
		return errors.Join(fmt.Errorf("stage rollback binary: %w", err), systemctl(ctx, "start", serviceName))
	}

	webSource := rollbackWeb
	if info, err := os.Lstat(webSource); errors.Is(err, os.ErrNotExist) {
		webSource = filepath.Join(filepath.Dir(liveWeb), ".home-ai-core-web-restore")
		if err := os.RemoveAll(webSource); err != nil {
			return errors.Join(fmt.Errorf("clear rollback web staging: %w", err), systemctl(ctx, "start", serviceName))
		}
		if err := copyTree(filepath.Join(backup, "web"), webSource); err != nil {
			return errors.Join(fmt.Errorf("stage rollback web: %w", err), systemctl(ctx, "start", serviceName))
		}
	} else if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(fmt.Errorf("invalid prior web directory: %v", err), systemctl(ctx, "start", serviceName))
	}

	backupHelper := filepath.Join(backup, "helper", "home-ai-core-updater")
	stagedHelper := filepath.Join(filepath.Dir(liveHelper), ".home-ai-core-updater.rollback")
	helperStaged := false
	if info, err := os.Lstat(backupHelper); err == nil {
		if !info.Mode().IsRegular() {
			return errors.Join(errors.New("rollback helper is not a regular file"), systemctl(ctx, "start", serviceName))
		}
		if err := copyFile(backupHelper, stagedHelper, 0o755); err != nil {
			return errors.Join(fmt.Errorf("stage rollback helper: %w", err), systemctl(ctx, "start", serviceName))
		}
		helperStaged = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.Join(fmt.Errorf("inspect rollback helper: %w", err), systemctl(ctx, "start", serviceName))
	}

	if err := systemctl(ctx, "stop", serviceName); err != nil {
		return errors.Join(fmt.Errorf("stop Core for rollback: %w", err), systemctl(ctx, "start", serviceName))
	}
	var failures []error
	if err := os.Rename(stagedCore, liveBinary); err != nil {
		failures = append(failures, fmt.Errorf("restore Core binary: %w", err))
	}
	// Never delete a live Web tree before the replacement is ready. Exchange
	// existing directories atomically; webSource then holds the displaced tree.
	if err := replaceDirectoryAtomically(webSource, liveWeb); err != nil {
		failures = append(failures, fmt.Errorf("restore Web UI: %w", err))
	}
	if helperStaged {
		if err := os.Rename(stagedHelper, liveHelper); err != nil {
			failures = append(failures, fmt.Errorf("restore updater helper: %w", err))
		}
	}
	if err := systemctl(ctx, "start", serviceName); err != nil {
		failures = append(failures, fmt.Errorf("restart Core after rollback: %w", err))
	} else if err := waitActive(ctx, serviceName, 15*time.Second); err != nil {
		failures = append(failures, fmt.Errorf("confirm Core after rollback: %w", err))
	}
	if len(failures) == 0 {
		_ = os.RemoveAll(webSource)
	}
	return errors.Join(failures...)
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
	var syncErr error
	if copyErr == nil {
		syncErr = out.Sync()
	}
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
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
	return writeUntrustedResult(updateRoot, uid, gid, data)
}
