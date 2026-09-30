//go:build windows

package windowsclient

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const updateParentWait = 5 * time.Minute

func UserClientUpdateDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate LocalAppData: %w", err)
	}
	return filepath.Join(base, "HomeAI", "updates"), nil
}

func LaunchUserClientUpdate(update DownloadedWindowsClientUpdate, restartAgentConfig string) error {
	if err := validateDownloadedUserUpdate(update); err != nil {
		return err
	}
	target, installed, err := UserClientInstallStatus()
	if err != nil {
		return err
	}
	if !installed {
		return errors.New("Home-AI Windows client is not installed; run client install first")
	}
	helper, err := createUserUpdateHelper()
	if err != nil {
		return err
	}
	args := []string{
		"client", "apply-update",
		"--candidate", update.Path,
		"--version", update.Version,
		"--sha256", update.SHA256,
		"--wait-pid", strconv.Itoa(os.Getpid()),
	}
	restartAgentConfig = strings.TrimSpace(restartAgentConfig)
	if restartAgentConfig != "" {
		restartAgentConfig, err = filepath.Abs(restartAgentConfig)
		if err != nil {
			return fmt.Errorf("resolve agent config for restart: %w", err)
		}
		args = append(args, "--restart-agent-config", restartAgentConfig)
	}
	command := exec.Command(helper, args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Windows client update helper: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release Windows client update helper: %w", err)
	}
	_ = target
	return nil
}

func ApplyUserClientUpdate(update DownloadedWindowsClientUpdate, waitPID int, restartAgentConfig string) error {
	if err := validateDownloadedUserUpdate(update); err != nil {
		return err
	}
	target, err := UserClientInstallPath()
	if err != nil {
		return err
	}
	if waitPID > 0 {
		if err := waitForWindowsProcess(waitPID, updateParentWait); err != nil {
			return err
		}
	}
	var installErr error
	for attempt := 0; attempt < 40; attempt++ {
		installErr = installExecutable(update.Path, target)
		if installErr == nil {
			break
		}
		timer := time.NewTimer(250 * time.Millisecond)
		<-timer.C
	}
	if installErr != nil {
		return fmt.Errorf("replace installed Windows client: %w", installErr)
	}
	_ = os.Remove(update.Path)

	restartAgentConfig = strings.TrimSpace(restartAgentConfig)
	if restartAgentConfig == "" {
		return nil
	}
	restartAgentConfig, err = filepath.Abs(restartAgentConfig)
	if err != nil {
		return fmt.Errorf("resolve agent restart config: %w", err)
	}
	if info, err := os.Lstat(restartAgentConfig); err != nil {
		return fmt.Errorf("inspect agent restart config: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("agent restart config must be a real regular file")
	}
	command := exec.Command(target, "agent", "run", "--config", restartAgentConfig)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return fmt.Errorf("restart Home-AI sync agent: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release restarted Home-AI sync agent: %w", err)
	}
	return nil
}

func validateDownloadedUserUpdate(update DownloadedWindowsClientUpdate) error {
	if _, err := parseDevVersion(update.Version); err != nil {
		return fmt.Errorf("invalid downloaded update version: %w", err)
	}
	checksum := strings.ToLower(strings.TrimSpace(update.SHA256))
	decoded, err := hexDecodeSHA256(checksum)
	if err != nil || len(decoded) != 32 {
		return errors.New("downloaded update has invalid SHA-256")
	}
	root, err := UserClientUpdateDir()
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(strings.TrimSpace(update.Path))
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("downloaded update is outside the Home-AI update directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect downloaded update: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("downloaded update must be a real regular file")
	}
	actual, err := hashRegularFile(path)
	if err != nil {
		return fmt.Errorf("hash downloaded update: %w", err)
	}
	if !strings.EqualFold(actual, checksum) {
		return errors.New("downloaded update SHA-256 changed before handoff")
	}
	return nil
}

func createUserUpdateHelper() (string, error) {
	root, err := UserClientUpdateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create update helper directory: %w", err)
	}
	_ = cleanupOldUpdateHelpers(root)
	current, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate current Windows client: %w", err)
	}
	file, err := os.CreateTemp(root, ".home-ai-update-helper-*.exe")
	if err != nil {
		return "", fmt.Errorf("reserve update helper path: %w", err)
	}
	helper := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(helper); err != nil {
		return "", err
	}
	if err := installExecutable(current, helper); err != nil {
		return "", fmt.Errorf("create update helper: %w", err)
	}
	return helper, nil
}

func cleanupOldUpdateHelpers(root string) error {
	current, _ := os.Executable()
	matches, err := filepath.Glob(filepath.Join(root, ".home-ai-update-helper-*.exe"))
	if err != nil {
		return err
	}
	for _, name := range matches {
		if current != "" && sameCleanPath(name, current) {
			continue
		}
		_ = os.Remove(name)
	}
	return nil
}

func waitForWindowsProcess(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("open parent process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle)
	milliseconds := uint32(timeout / time.Millisecond)
	result, err := windows.WaitForSingleObject(handle, milliseconds)
	if err != nil {
		return fmt.Errorf("wait for parent process %d: %w", pid, err)
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return nil
	case windows.WAIT_TIMEOUT:
		return fmt.Errorf("timed out waiting for parent process %d to exit", pid)
	default:
		return fmt.Errorf("unexpected wait result %d for parent process %d", result, pid)
	}
}

func hexDecodeSHA256(value string) ([]byte, error) {
	return hex.DecodeString(value)
}
