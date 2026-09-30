package windowsclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AgentLock struct {
	file *os.File
}

func AcquireAgentLock(filename string) (*AgentLock, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, errors.New("agent lock path is required")
	}
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, fmt.Errorf("resolve agent lock path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return nil, fmt.Errorf("create agent lock directory: %w", err)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(filename))
	if err != nil {
		return nil, fmt.Errorf("resolve agent lock directory: %w", err)
	}
	filename = filepath.Join(dir, filepath.Base(filename))
	if info, err := os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("agent lock path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect agent lock path: %w", err)
	}
	file, err := lockQueueFile(filename)
	if err != nil {
		return nil, fmt.Errorf("another Home-AI sync agent may already be running: %w", err)
	}
	return &AgentLock{file: file}, nil
}

func (lock *AgentLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	file := lock.file
	lock.file = nil
	return errors.Join(unlockQueueFile(file), file.Close())
}
