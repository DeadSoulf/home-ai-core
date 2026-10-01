package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

const nasPolicyPath = "/etc/systemd/system/home-ai-core.service.d/30-nas.conf"
const nasPolicyContent = "[Service]\nReadWritePaths=-/mnt/home-ai-core\n"

// Web bundles update the broker, so existing Debian installations receive the
// same NAS namespace policy as newly installed packages.
func ensureNASServicePolicy(ctx context.Context) error {
	info, err := os.Lstat(nasMountRoot)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(nasMountRoot, 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("NAS mount root must be a real directory")
	}
	if data, err := os.ReadFile(nasPolicyPath); err == nil && string(data) == nasPolicyContent {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(nasPolicyPath), 0o755); err != nil {
		return err
	}
	if err := atomicWriteFile(nasPolicyPath, []byte(nasPolicyContent), 0o644); err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, "/usr/bin/systemctl", "daemon-reload").Run(); err != nil {
		return err
	}
	return systemctl(ctx, "try-restart", serviceName)
}
