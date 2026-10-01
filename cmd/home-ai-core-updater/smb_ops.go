package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const (
	smbMainConfig       = "/etc/samba/smb.conf"
	smbManagedConfig    = "/etc/samba/home-ai.conf"
	smbIncludeBegin     = "# BEGIN Home-AI-Core managed include"
	smbIncludeDirective = "include = /etc/samba/home-ai.conf"
	smbIncludeEnd       = "# END Home-AI-Core managed include"
)

func inspectSMB(ctx context.Context, users []string) (bool, bool, string, []string) {
	if !sambaToolsAvailable() {
		return false, false, "Samba is not installed", nil
	}
	active := exec.CommandContext(ctx, "/usr/bin/systemctl", "is-active", "--quiet", "smbd.service").Run() == nil
	configured := make([]string, 0, len(users))
	for _, user := range uniqueSortedStrings(users) {
		if !validSMBUser(user) {
			continue
		}
		if exec.CommandContext(ctx, "pdbedit", "-L", "-u", user).Run() == nil {
			configured = append(configured, user)
		}
	}
	return true, active, "", configured
}

func performSMBOperation(
	ctx context.Context,
	request updaterhelper.Request,
	uid, gid int,
) (string, error) {
	switch request.Operation {
	case "smb.suspend":
		if _, err := os.Stat(smbManagedConfig); errors.Is(err, os.ErrNotExist) {
			return "SMB is not configured", nil
		} else if err != nil {
			return "", err
		}
		// Stop the service first, ending open handles as well as fresh sessions.
		if err := systemctl(ctx, "stop", "smbd.service"); err != nil {
			return "", err
		}
		if err := os.WriteFile(smbManagedConfig, []byte(renderSMBConfig("WORKGROUP", nil)), 0o600); err != nil {
			return "", err
		}
		return "SMB access suspended", nil
	case "smb.install":
		return installSamba(ctx)
	case "smb.user.set_password":
		return setSMBPassword(ctx, request.SMBUser, request.SMBPassword)
	case "smb.apply":
		return applySMBConfig(ctx, request.SMBWorkgroup, request.SMBShares, uid, gid)
	default:
		return "", errors.New("unsupported SMB operation")
	}
}

func sambaToolsAvailable() bool {
	for _, name := range []string{"smbd", "smbpasswd", "pdbedit", "testparm"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
}

func installSamba(ctx context.Context) (string, error) {
	if sambaToolsAvailable() {
		if err := enableSMBService(ctx); err != nil {
			return "", err
		}
		return "Samba is already installed", nil
	}
	if !packageInstallMu.TryLock() {
		return "", errors.New("package installation is already in progress")
	}
	defer packageInstallMu.Unlock()

	apt, err := exec.LookPath("apt-get")
	if err != nil {
		return "", errors.New("apt-get is unavailable")
	}
	systemdRun, err := exec.LookPath("systemd-run")
	if err != nil {
		return "", errors.New("systemd-run is unavailable")
	}

	for _, args := range [][]string{{"update"}, {"install", "-y", "samba"}} {
		commandArgs := transientPackageCommand(apt, args...)
		cmd := exec.CommandContext(ctx, systemdRun, commandArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = err.Error()
			}
			return "", fmt.Errorf("install Samba: %s", message)
		}
	}
	if !sambaToolsAvailable() {
		return "", errors.New("Samba installation completed but required tools are unavailable")
	}
	if err := enableSMBService(ctx); err != nil {
		return "", err
	}
	return "Samba installed", nil
}

func enableSMBService(ctx context.Context) error {
	output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "enable", "--now", "smbd.service").CombinedOutput()
	if err != nil {
		return fmt.Errorf("enable Samba service: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func setSMBPassword(ctx context.Context, username, password string) (string, error) {
	if !sambaToolsAvailable() {
		return "", errors.New("Samba is not installed")
	}
	username = strings.TrimSpace(username)
	if !validSMBUser(username) {
		return "", errors.New("invalid SMB system username")
	}
	if len(password) < 12 || len(password) > 256 {
		return "", errors.New("SMB password must contain 12 to 256 characters")
	}
	if strings.ContainsAny(password, "\r\n") {
		return "", errors.New("SMB password cannot contain line breaks")
	}

	if exec.CommandContext(ctx, "getent", "passwd", username).Run() != nil {
		if _, err := runHostCommand(
			ctx,
			"",
			"useradd",
			"--system",
			"--no-create-home",
			"--shell", "/usr/sbin/nologin",
			username,
		); err != nil {
			return "", fmt.Errorf("create SMB system identity: %w", err)
		}
	}

	input := password + "\n" + password + "\n"
	if _, err := runHostCommand(ctx, input, "smbpasswd", "-s", "-a", username); err != nil {
		return "", fmt.Errorf("set SMB password: %w", err)
	}
	if _, err := runHostCommand(ctx, "", "smbpasswd", "-e", username); err != nil {
		return "", fmt.Errorf("enable SMB account: %w", err)
	}
	return "SMB password updated", nil
}

func applySMBConfig(
	ctx context.Context,
	workgroup string,
	shares []updaterhelper.SMBShareRequest,
	uid, gid int,
) (string, error) {
	if !sambaToolsAvailable() {
		return "", errors.New("Samba is not installed")
	}
	workgroup = strings.ToUpper(strings.TrimSpace(workgroup))
	if workgroup == "" {
		workgroup = "WORKGROUP"
	}
	if !validSMBWorkgroup(workgroup) {
		return "", errors.New("invalid SMB workgroup")
	}

	normalized := make([]updaterhelper.SMBShareRequest, 0, len(shares))
	seenNames := map[string]struct{}{}
	for _, share := range shares {
		share.Name = strings.TrimSpace(share.Name)
		share.Path = filepath.Clean(strings.TrimSpace(share.Path))
		if !validSMBShareName(share.Name) {
			return "", fmt.Errorf("invalid SMB share name %q", share.Name)
		}
		lower := strings.ToLower(share.Name)
		if _, exists := seenNames[lower]; exists {
			return "", fmt.Errorf("duplicate SMB share name %q", share.Name)
		}
		seenNames[lower] = struct{}{}
		if err := validateSMBSharePath(share.Path, uid, gid); err != nil {
			return "", fmt.Errorf("share %s: %w", share.Name, err)
		}
		share.ReadUsers = uniqueSortedStrings(share.ReadUsers)
		share.WriteUsers = uniqueSortedStrings(share.WriteUsers)
		for _, user := range append(append([]string{}, share.ReadUsers...), share.WriteUsers...) {
			if !validSMBUser(user) {
				return "", fmt.Errorf("share %s contains invalid SMB user", share.Name)
			}
		}
		if len(share.ReadUsers) == 0 && len(share.WriteUsers) == 0 {
			return "", fmt.Errorf("share %s has no authorized users", share.Name)
		}
		normalized = append(normalized, share)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i].Name) < strings.ToLower(normalized[j].Name)
	})

	content := renderSMBConfig(workgroup, normalized)
	if err := validateManagedSMBConfig(ctx, content); err != nil {
		return "", err
	}
	if err := ensureSMBInclude(); err != nil {
		return "", err
	}

	oldManaged, oldMode, oldExists, err := readOptionalFile(smbManagedConfig)
	if err != nil {
		return "", err
	}
	if err := atomicWriteFile(smbManagedConfig, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write managed Samba config: %w", err)
	}

	if _, err := runHostCommand(ctx, "", "testparm", "-s", smbMainConfig); err != nil {
		if oldExists {
			_ = atomicWriteFile(smbManagedConfig, oldManaged, oldMode)
		} else {
			_ = os.Remove(smbManagedConfig)
		}
		return "", fmt.Errorf("validate complete Samba configuration: %w", err)
	}
	if output, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "reload-or-restart", "smbd.service").CombinedOutput(); err != nil {
		return "", fmt.Errorf("reload Samba service: %s", strings.TrimSpace(string(output)))
	}
	return fmt.Sprintf("SMB configuration applied with %d shares", len(normalized)), nil
}

func renderSMBConfig(workgroup string, shares []updaterhelper.SMBShareRequest) string {
	var b strings.Builder
	b.WriteString("# Managed by Home-AI-Core. Manual edits will be replaced.\n")
	b.WriteString("[global]\n")
	b.WriteString("    workgroup = " + workgroup + "\n")
	b.WriteString("    server string = Home-AI\n")
	b.WriteString("    map to guest = Never\n")
	b.WriteString("    usershare allow guests = no\n")
	b.WriteString("    server min protocol = SMB2_10\n")
	b.WriteString("\n")

	for _, share := range shares {
		allUsers := uniqueSortedStrings(append(append([]string{}, share.ReadUsers...), share.WriteUsers...))
		b.WriteString("[" + share.Name + "]\n")
		b.WriteString("    path = " + share.Path + "\n")
		b.WriteString("    browseable = yes\n")
		b.WriteString("    guest ok = no\n")
		b.WriteString("    read only = yes\n")
		b.WriteString("    valid users = " + strings.Join(allUsers, " ") + "\n")
		if len(share.ReadUsers) > 0 {
			b.WriteString("    read list = " + strings.Join(share.ReadUsers, " ") + "\n")
		}
		if len(share.WriteUsers) > 0 {
			b.WriteString("    write list = " + strings.Join(share.WriteUsers, " ") + "\n")
		}
		b.WriteString("    force user = home-ai-core\n")
		b.WriteString("    force group = home-ai-core\n")
		b.WriteString("    create mask = 0640\n")
		b.WriteString("    directory mask = 0750\n")
		b.WriteString("    follow symlinks = no\n")
		b.WriteString("    wide links = no\n")
		b.WriteString("    veto files = /.home-ai-trash/.home-ai-upload-*/\n")
		b.WriteString("\n")
	}
	return b.String()
}

func validateManagedSMBConfig(ctx context.Context, content string) error {
	file, err := os.CreateTemp("/etc/samba", ".home-ai-test-*.conf")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := runHostCommand(ctx, "", "testparm", "-s", path); err != nil {
		return fmt.Errorf("validate managed Samba configuration: %w", err)
	}
	return nil
}

func ensureSMBInclude() error {
	data, err := os.ReadFile(smbMainConfig)
	if err != nil {
		return fmt.Errorf("read Samba config: %w", err)
	}
	updated, changed := ensureSMBIncludeContent(string(data))
	if !changed {
		return nil
	}
	info, err := os.Stat(smbMainConfig)
	if err != nil {
		return err
	}
	backup := smbMainConfig + ".home-ai.bak." + time.Now().UTC().Format("20060102T150405Z")
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("backup Samba config: %w", err)
	}
	if err := atomicWriteFile(smbMainConfig, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Errorf("add Home-AI Samba include: %w", err)
	}
	return nil
}

func ensureSMBIncludeContent(content string) (string, bool) {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.EqualFold(line, smbIncludeDirective) {
			return content, false
		}
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed != "" {
		trimmed += "\n\n"
	}
	trimmed += smbIncludeBegin + "\n" +
		smbIncludeDirective + "\n" +
		smbIncludeEnd + "\n"
	return trimmed, true
}

func validateSMBSharePath(path string, uid, gid int) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) ||
		path == nasMountRoot ||
		!strings.HasPrefix(path, nasMountRoot+string(filepath.Separator)) {
		return errors.New("SMB share must be inside Home-AI NAS mount root")
	}
	marker := string(filepath.Separator) + ".home-ai" + string(filepath.Separator)
	if !strings.Contains(path, marker) {
		return errors.New("SMB share must point to a Home-AI managed NAS folder")
	}

	relative, err := filepath.Rel(nasMountRoot, path)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return errors.New("invalid SMB share path")
	}
	current := nasMountRoot
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("invalid SMB share path segment")
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect SMB share path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("SMB share path cannot contain symlinks")
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("SMB share path is not a directory")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != uid || int(stat.Gid) != gid {
			return errors.New("SMB share path is not owned by Home-AI-Core")
		}
	}
	return nil
}

func validSMBUser(value string) bool {
	if len(value) < 3 || len(value) > 31 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validSMBShareName(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validSMBWorkgroup(value string) bool {
	if value == "" || len(value) > 15 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func readOptionalFile(path string) ([]byte, os.FileMode, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode().Perm(), true, nil
}
