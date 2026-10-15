package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// stageRollbackBackup prepares a complete snapshot without touching the
// previously committed rollback. Only the root-owned updater directory may
// be used as the parent of this snapshot.
func stageRollbackBackup(root, corePath, webPath, helperPath, currentVersion, nextVersion string, includeHelper bool) (string, error) {
	if !validVersion(currentVersion) || !validVersion(nextVersion) {
		return "", fmt.Errorf("cannot stage rollback for invalid version %q -> %q", currentVersion, nextVersion)
	}
	staged, err := os.MkdirTemp(root, ".backup-candidate-")
	if err != nil {
		return "", fmt.Errorf("create staged rollback: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(staged)
		}
	}()

	if err := os.MkdirAll(filepath.Join(staged, "bin"), 0o700); err != nil {
		return "", fmt.Errorf("prepare staged rollback binary directory: %w", err)
	}
	if err := copyFile(corePath, filepath.Join(staged, "bin", "home-ai-core"), 0o700); err != nil {
		return "", fmt.Errorf("backup core binary: %w", err)
	}
	if err := copyTree(webPath, filepath.Join(staged, "web")); err != nil {
		return "", fmt.Errorf("backup web ui: %w", err)
	}
	if includeHelper {
		if err := os.MkdirAll(filepath.Join(staged, "helper"), 0o700); err != nil {
			return "", fmt.Errorf("prepare staged rollback helper directory: %w", err)
		}
		if err := copyFile(helperPath, filepath.Join(staged, "helper", "home-ai-core-updater"), 0o700); err != nil {
			return "", fmt.Errorf("backup updater helper: %w", err)
		}
	}
	if err := writeBackupMetadata(staged, backupMetadata{
		Version:    currentVersion,
		ReplacedBy: nextVersion,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return "", fmt.Errorf("write staged rollback metadata: %w", err)
	}
	if err := syncRollbackDirectories(staged); err != nil {
		return "", fmt.Errorf("sync staged rollback: %w", err)
	}
	complete = true
	return staged, nil
}

// refuseInterruptedBackupStaging prevents an update from discarding evidence
// of a previously interrupted install. Recovery of a leftover snapshot must be
// explicit: it might be the only complete copy of the former installation.
func refuseInterruptedBackupStaging(root, rollbackWeb string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("inspect previous update state: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".backup-candidate-") {
			return fmt.Errorf("unresolved rollback snapshot %q: inspect or recover the interrupted update before retrying", filepath.Join(root, entry.Name()))
		}
	}
	if _, err := os.Lstat(rollbackWeb); err == nil {
		return fmt.Errorf("unresolved Web UI backup %q: inspect or recover the interrupted update before retrying", rollbackWeb)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect previous Web UI backup: %w", err)
	}
	return nil
}

// replaceDirectoryAtomically exchanges two directories within a single
// filesystem. The previous destination ends up at staged, so the caller may
// remove it only after the new active directory is known to be usable.
// Do not fall back to two non-atomic renames if RENAME_EXCHANGE is unavailable.
func replaceDirectoryAtomically(staged, active string) error {
	sourceInfo, err := os.Lstat(staged)
	if err != nil {
		return fmt.Errorf("inspect staged directory: %w", err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return errors.New("staged directory is not a real directory")
	}
	activeInfo, err := os.Lstat(active)
	if errors.Is(err, os.ErrNotExist) {
		return os.Rename(staged, active)
	}
	if err != nil {
		return fmt.Errorf("inspect active directory: %w", err)
	}
	if activeInfo.Mode()&os.ModeSymlink != 0 || !activeInfo.IsDir() {
		return errors.New("active directory is not a real directory")
	}
	if err := unix.Renameat2(unix.AT_FDCWD, staged, unix.AT_FDCWD, active, unix.RENAME_EXCHANGE); err != nil {
		return fmt.Errorf("atomic directory exchange: %w", err)
	}
	return nil
}

// On a power loss, a rename must never point to a tree whose file and
// directory entries were still only in memory.
func syncRollbackDirectories(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		dir, err := os.Open(directories[i])
		if err != nil {
			return err
		}
		syncErr := dir.Sync()
		closeErr := dir.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			return fmt.Errorf("sync backup directory %s: %w", directories[i], err)
		}
	}
	return nil
}
