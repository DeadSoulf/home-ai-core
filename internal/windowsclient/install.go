package windowsclient

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrUserClientInstallUnsupported = errors.New("per-user Windows client installation is unavailable on this platform")

type UserClientInstallResult struct {
	Path               string
	Changed            bool
	AgentExitRequested bool
}

func installExecutable(source, destination string) error {
	source = strings.TrimSpace(source)
	destination = strings.TrimSpace(destination)
	if source == "" || destination == "" {
		return errors.New("source and destination executable paths are required")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve source executable: %w", err)
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve destination executable: %w", err)
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect source executable: %w", err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.Mode().IsRegular() {
		return errors.New("source executable must be a real regular file")
	}
	if sameCleanPath(source, destination) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return fmt.Errorf("create user client directory: %w", err)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("resolve user client directory: %w", err)
	}
	destination = filepath.Join(dir, filepath.Base(destination))
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("installed client path must be a real regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect installed client: %w", err)
	}

	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source executable: %w", err)
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(destination), ".home-ai-client-*")
	if err != nil {
		return fmt.Errorf("create client install temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o700); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set installed client permissions: %w", err)
	}
	sourceHash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, sourceHash), input)
	if copyErr != nil {
		_ = temp.Close()
		return fmt.Errorf("copy Windows client: %w", copyErr)
	}
	if written != sourceInfo.Size() {
		_ = temp.Close()
		return fmt.Errorf("copied client size = %d, want %d", written, sourceInfo.Size())
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync installed client: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close installed client: %w", err)
	}
	if err := replaceQueueFile(tempName, destination); err != nil {
		return fmt.Errorf("activate installed client: %w", err)
	}
	installedHash, err := hashRegularFile(destination)
	if err != nil {
		return fmt.Errorf("verify installed client: %w", err)
	}
	if installedHash != fmt.Sprintf("%x", sourceHash.Sum(nil)) {
		return errors.New("installed client SHA-256 does not match source")
	}
	return nil
}

func hashRegularFile(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("path is not a real regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func sameRegularFileContent(left, right string) (bool, error) {
	leftHash, err := hashRegularFile(left)
	if err != nil {
		return false, err
	}
	rightHash, err := hashRegularFile(right)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func sameCleanPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}
