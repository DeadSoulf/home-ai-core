package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/homeassistant"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

type haContextReader struct {
	ctx context.Context
	io.Reader
}

func (r haContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// Called only after Docker has stopped the old container. The candidate receives
// a COPY, never the old /config. Rollback therefore restores the old database as
// well as the old image, even after an irreversible candidate schema migration.
func haCopyConfig(ctx context.Context, source string) (string, error) {
	if err := haValidateConfig(source); err != nil {
		return "", err
	}
	var size uint64
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("Home Assistant backup refuses symlinks and special files")
		}
		if info.Size() < 0 || uint64(info.Size()) > ^uint64(0)-size {
			return errors.New("Home Assistant configuration size overflow")
		}
		size += uint64(info.Size())
		return nil
	})
	if err != nil {
		return "", err
	}
	var disk syscall.Statfs_t
	if err := syscall.Statfs(haRoot(), &disk); err != nil {
		return "", err
	}
	free := disk.Bavail * uint64(disk.Bsize)
	const reserve = uint64(256 << 20)
	if free < reserve || size > free-reserve {
		return "", errors.New("insufficient free space for a full Home Assistant configuration copy plus 256 MiB reserve")
	}
	destination, err := os.MkdirTemp(haRoot(), "config-")
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return errors.New("Home Assistant configuration changed to an unsafe file type")
		}
		input, err := root.Open(rel)
		if err != nil {
			return err
		}
		defer input.Close()
		opened, err := input.Stat()
		if err != nil {
			return err
		}
		if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			return errors.New("Home Assistant configuration changed while copying")
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600|info.Mode().Perm()&0o100)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, haContextReader{ctx, input})
		if copyErr == nil {
			copyErr = output.Sync()
		}
		closeErr := output.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		return os.Chtimes(target, info.ModTime(), info.ModTime())
	})
	if err != nil {
		return "", fmt.Errorf("configuration copy incomplete at %s; original preserved: %w", destination, err)
	}
	// Persist directory entries before a candidate is allowed to start.
	err = filepath.WalkDir(destination, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	})
	if err != nil {
		return "", err
	}
	return destination, nil
}

// Container IDs survive rename and command timeout ambiguity. Never restart the
// old instance until the candidate has definitely stopped and been removed.
func haRestorePrevious(binary string, old haContainer, candidateID string) (moduleContainerStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	if candidateID != "" {
		if _, err := haInspect(ctx, binary, candidateID); err != nil {
			return moduleContainerStatus{}, fmt.Errorf("cannot verify candidate before rollback: %w", err)
		}
		if err := dockerAICommand(ctx, binary, "stop", "--time", "60", candidateID); err != nil {
			return moduleContainerStatus{}, err
		}
		if err := dockerAICommand(ctx, binary, "rm", candidateID); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	info, err := haInspect(ctx, binary, old.ID)
	if err != nil {
		return moduleContainerStatus{}, err
	}
	if info.Config.Image != old.Config.Image || info.configDir != old.configDir {
		return moduleContainerStatus{}, errors.New("previous Home Assistant image or config changed; rollback refused")
	}
	name := moduleContainerName(homeassistant.ModuleID)
	out, err := exec.CommandContext(ctx, binary, "inspect", "--format", "{{.Name}}", old.ID).CombinedOutput()
	if err != nil {
		return moduleContainerStatus{}, fmt.Errorf("inspect previous Home Assistant name: %s", commandError(out, err))
	}
	if strings.TrimPrefix(strings.TrimSpace(string(out)), "/") != name {
		if err := dockerAICommand(ctx, binary, "rename", old.ID, name); err != nil {
			return moduleContainerStatus{}, err
		}
	}
	if err := dockerAICommand(ctx, binary, "start", old.ID); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := haWaitHealthy(ctx, binary, old.ID); err != nil {
		return moduleContainerStatus{}, err
	}
	if err := haWriteState("active.json", haState{Image: old.Config.Image, Directory: old.configDir}); err != nil {
		return moduleContainerStatus{}, err
	}
	return inspectModuleContainer(ctx, binary, old.ID)
}

func upgradeHomeAssistant(ctx context.Context, request updaterhelper.Request, report moduleProgressFunc) (moduleContainerStatus, string, error) {
	if request.ModuleID != homeassistant.ModuleID {
		return moduleContainerStatus{}, "", errors.New("invalid Home Assistant module id")
	}
	if err := homeassistant.ValidateImage(request.Image); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := homeassistant.ValidateImage(request.PreviousImage); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if request.Image == request.PreviousImage {
		return moduleContainerStatus{}, "", errors.New("Home Assistant candidate is identical to the previous image")
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	name := moduleContainerName(homeassistant.ModuleID)
	candidateName, previousName := moduleUpgradeStageNames(homeassistant.ModuleID)
	if request.Operation == "docker.module.upgrade.commit" || request.Operation == "docker.module.upgrade.rollback" {
		current, err := haInspect(ctx, binary, name)
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		previous, err := haInspect(ctx, binary, previousName)
		if err != nil {
			return moduleContainerStatus{}, "", err
		}
		if current.Config.Image != request.Image || previous.Config.Image != request.PreviousImage || current.configDir == previous.configDir || previous.State.Running {
			return moduleContainerStatus{}, "", errors.New("Home Assistant upgrade pair does not match the isolated configuration transaction")
		}
		if request.Operation == "docker.module.upgrade.rollback" {
			status, err := haRestorePrevious(binary, previous, current.ID)
			return status, "previous Home Assistant image AND original configuration restored", err
		}
		if err := haWaitHealthy(ctx, binary, current.ID); err != nil {
			return moduleContainerStatus{}, "", err
		}
		if err := haWriteState("active.json", haState{Image: current.Config.Image, Directory: current.configDir}); err != nil {
			return moduleContainerStatus{}, "", err
		}
		if err := dockerAICommand(ctx, binary, "rm", previous.ID); err != nil {
			return moduleContainerStatus{}, "", err
		}
		status, err := inspectModuleContainer(ctx, binary, current.ID)
		return status, "Home Assistant upgrade committed; previous configuration backup retained", err
	}
	if request.Operation != "docker.module.upgrade" {
		return moduleContainerStatus{}, "", errors.New("unsupported Home Assistant upgrade operation")
	}
	if err := haPreflight(ctx, binary); err != nil {
		return moduleContainerStatus{}, "", err
	}
	old, err := haInspect(ctx, binary, name)
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	state, err := haReadState()
	if err != nil {
		return moduleContainerStatus{}, "", err
	}
	if old.Config.Image != request.PreviousImage || !old.State.Running || state.Image != old.Config.Image || state.Directory != old.configDir {
		return moduleContainerStatus{}, "", errors.New("running Home Assistant image or configuration differs from the committed record")
	}
	if err := ensureModuleUpgradeNamesFree(ctx, binary, candidateName, previousName); err != nil {
		return moduleContainerStatus{}, "", err
	}
	report("download", "")
	if err := haPull(ctx, binary, request.Image, report); err != nil {
		return moduleContainerStatus{}, "", err
	}
	if err := ctx.Err(); err != nil {
		return moduleContainerStatus{}, "", err
	}
	critical, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Minute)
	defer cancel()
	var newID string
	recoverOld := func(cause error) (moduleContainerStatus, string, error) {
		_, recoveryErr := haRestorePrevious(binary, old, newID)
		if recoveryErr != nil {
			return moduleContainerStatus{}, "", errors.Join(cause, fmt.Errorf("Home Assistant rollback needs attention: %w", recoveryErr))
		}
		return moduleContainerStatus{}, "", fmt.Errorf("Home Assistant upgrade failed; previous image and original configuration restored: %w", cause)
	}
	if err := dockerAICommand(critical, binary, "stop", "--time", "60", old.ID); err != nil {
		// A failed stop can have taken effect; reconcile with an idempotent start.
		return recoverOld(err)
	}
	stopped, err := haInspect(critical, binary, old.ID)
	if err != nil {
		return recoverOld(err)
	}
	if stopped.State.Running {
		return recoverOld(errors.New("previous Home Assistant is still writing configuration"))
	}
	report("storage", "copying stopped Home Assistant configuration")
	copyDir, err := haCopyConfig(critical, old.configDir)
	if err != nil {
		return recoverOld(err)
	}
	if err := haWriteState("backup-"+filepath.Base(copyDir)+".json", haState{Image: old.Config.Image, Directory: old.configDir}); err != nil {
		return recoverOld(err)
	}
	if err := haPortAvailable(); err != nil {
		return recoverOld(err)
	}
	report("create", "")
	if err := haCreate(critical, binary, candidateName, request.Image, copyDir); err != nil {
		return recoverOld(err)
	}
	candidate, err := haInspect(critical, binary, candidateName)
	if err != nil {
		return recoverOld(err)
	}
	if candidate.Config.Image != request.Image || candidate.configDir != copyDir || candidate.ID == "" || candidate.ID == old.ID {
		return recoverOld(errors.New("Home Assistant candidate identity does not match"))
	}
	newID = candidate.ID
	report("start", "")
	if err := dockerAICommand(critical, binary, "start", newID); err != nil {
		return recoverOld(err)
	}
	report("verify", "")
	if err := haWaitHealthy(critical, binary, newID); err != nil {
		return recoverOld(err)
	}
	if err := dockerAICommand(critical, binary, "rename", old.ID, previousName); err != nil {
		return recoverOld(err)
	}
	if err := dockerAICommand(critical, binary, "rename", newID, name); err != nil {
		return recoverOld(err)
	}
	status, err := inspectModuleContainer(critical, binary, newID)
	if err != nil {
		return recoverOld(err)
	}
	if status.State != "running" {
		return recoverOld(errors.New("Home Assistant candidate exited after swap"))
	}
	return status, "Home Assistant candidate healthy; old image and unmodified config retained pending registry commit", nil
}
