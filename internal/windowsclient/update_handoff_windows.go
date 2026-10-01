//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func StartVerifiedClientInstall(executable string) error {
	executable = filepath.Clean(executable)
	info, err := os.Lstat(executable)
	if err != nil {
		return fmt.Errorf("inspect verified Windows client: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("verified Windows client must be a real regular file")
	}
	command := exec.Command(executable, "client", "install")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start verified Windows client installer: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release verified Windows client installer: %w", err)
	}
	return nil
}
