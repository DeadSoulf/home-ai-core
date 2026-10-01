package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"golang.org/x/sys/unix"
)

type smbPoolQuotaPolicy struct {
	RootPath       string
	ReservePercent int
}

type quotaMountInfo struct {
	RootPath   string
	Filesystem string
	Source     string
	Options    string
}

func inspectSMBHardQuota(
	ctx context.Context,
	shares []updaterhelper.SMBShareRequest,
	uid int,
) (bool, string) {
	policies, err := smbPoolQuotaPolicies(shares)
	if err != nil {
		return false, err.Error()
	}
	for _, policy := range policies {
		if policy.ReservePercent == 0 {
			continue
		}
		info, err := inspectQuotaMount(ctx, policy.RootPath)
		if err != nil {
			return false, err.Error()
		}
		if err := quotaMountReady(ctx, info); err != nil {
			return false, err.Error()
		}
		if _, err := quotaHardLimitKiB(info.RootPath, policy.ReservePercent); err != nil {
			return false, err.Error()
		}
	}
	_ = uid
	return true, ""
}

func enforceSMBHardQuota(
	ctx context.Context,
	shares []updaterhelper.SMBShareRequest,
	uid int,
) error {
	policies, err := smbPoolQuotaPolicies(shares)
	if err != nil {
		return err
	}
	for _, policy := range policies {
		if policy.ReservePercent == 0 {
			continue
		}
		info, err := inspectQuotaMount(ctx, policy.RootPath)
		if err != nil {
			return err
		}
		if err := quotaMountReady(ctx, info); err != nil {
			return err
		}
		limitKiB, err := quotaHardLimitKiB(info.RootPath, policy.ReservePercent)
		if err != nil {
			return err
		}
		setquota, err := exec.LookPath("setquota")
		if err != nil {
			return errors.New("quota tools are unavailable; install the quota package")
		}
		args := []string{
			"--always-resolve",
			"-u",
			strconv.Itoa(uid),
			strconv.FormatUint(limitKiB, 10),
			strconv.FormatUint(limitKiB, 10),
			"0",
			"0",
			info.RootPath,
		}
		output, err := exec.CommandContext(ctx, setquota, args...).CombinedOutput()
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message == "" {
				message = err.Error()
			}
			return fmt.Errorf("apply hard quota on %s: %s", info.RootPath, message)
		}
	}
	return nil
}

func smbPoolQuotaPolicies(shares []updaterhelper.SMBShareRequest) ([]smbPoolQuotaPolicy, error) {
	byRoot := map[string]int{}
	for _, share := range shares {
		root := filepath.Clean(strings.TrimSpace(share.PoolRoot))
		path := filepath.Clean(strings.TrimSpace(share.Path))
		if root == "." || !filepath.IsAbs(root) ||
			root == nasMountRoot ||
			!strings.HasPrefix(root, nasMountRoot+string(filepath.Separator)) {
			return nil, fmt.Errorf("share %s has invalid NAS pool root", share.Name)
		}
		if share.ReservePercent < 0 || share.ReservePercent > 50 {
			return nil, fmt.Errorf("share %s has invalid reserve percentage", share.Name)
		}
		managedPrefix := filepath.Join(root, ".home-ai") + string(filepath.Separator)
		if !strings.HasPrefix(path, managedPrefix) {
			return nil, fmt.Errorf("share %s is outside its NAS pool", share.Name)
		}
		if existing, ok := byRoot[root]; ok && existing != share.ReservePercent {
			return nil, fmt.Errorf("NAS pool %s has conflicting reserve policies", root)
		}
		byRoot[root] = share.ReservePercent
	}
	result := make([]smbPoolQuotaPolicy, 0, len(byRoot))
	for root, reserve := range byRoot {
		result = append(result, smbPoolQuotaPolicy{RootPath: root, ReservePercent: reserve})
	}
	return result, nil
}

func inspectQuotaMount(ctx context.Context, root string) (quotaMountInfo, error) {
	validated, err := validateNASRoot(ctx, root)
	if err != nil {
		return quotaMountInfo{}, err
	}
	filesystem, err := findmntValue(ctx, validated, "FSTYPE")
	if err != nil {
		return quotaMountInfo{}, err
	}
	source, err := findmntValue(ctx, validated, "SOURCE")
	if err != nil {
		return quotaMountInfo{}, err
	}
	options, err := findmntValue(ctx, validated, "OPTIONS")
	if err != nil {
		return quotaMountInfo{}, err
	}
	return quotaMountInfo{
		RootPath:   validated,
		Filesystem: strings.ToLower(strings.TrimSpace(filesystem)),
		Source:     strings.TrimSpace(source),
		Options:    strings.ToLower(strings.TrimSpace(options)),
	}, nil
}

func findmntValue(ctx context.Context, root, field string) (string, error) {
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/findmnt",
		"-rn",
		"-T", root,
		"-o", field,
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect %s for %s: %s", strings.ToLower(field), root, strings.TrimSpace(string(output)))
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", fmt.Errorf("inspect %s for %s: empty result", strings.ToLower(field), root)
	}
	return value, nil
}

func quotaMountReady(ctx context.Context, info quotaMountInfo) error {
	if _, err := exec.LookPath("setquota"); err != nil {
		return errors.New("quota tools are unavailable; install the quota package")
	}
	switch info.Filesystem {
	case "xfs":
		if !hasUserQuotaMountOption(info.Options) {
			return errors.New("XFS user quota accounting is not active; remount this Home-AI storage with uquota")
		}
		return nil
	case "ext4":
		enabled, err := ext4EmbeddedUserQuota(ctx, info.Source)
		if err != nil {
			return err
		}
		if !enabled {
			return errors.New("ext4 embedded user quotas are not enabled; controlled migration or reformat is required")
		}
		return nil
	default:
		return fmt.Errorf("hard SMB reserve is unsupported on %s; use ext4 or XFS", info.Filesystem)
	}
}

func hasUserQuotaMountOption(options string) bool {
	for _, option := range strings.Split(strings.ToLower(options), ",") {
		switch strings.TrimSpace(option) {
		case "uquota", "usrquota", "quota":
			return true
		}
	}
	return false
}

func ext4EmbeddedUserQuota(ctx context.Context, source string) (bool, error) {
	output, err := exec.CommandContext(ctx, "/usr/sbin/tune2fs", "-l", source).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect ext4 quota features: %s", strings.TrimSpace(string(output)))
	}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "Filesystem features" {
			continue
		}
		for _, feature := range strings.Fields(value) {
			if feature == "quota" {
				return true, nil
			}
		}
		return false, nil
	}
	return false, errors.New("ext4 filesystem features are unavailable")
}

func quotaHardLimitKiB(root string, reservePercent int) (uint64, error) {
	if reservePercent < 0 || reservePercent > 50 {
		return 0, errors.New("reserve percent must be between 0 and 50")
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		return 0, fmt.Errorf("inspect filesystem capacity: %w", err)
	}
	totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
	usablePercent := uint64(100 - reservePercent)
	usableBytes := (totalBytes/100)*usablePercent + ((totalBytes%100)*usablePercent)/100
	limitKiB := usableBytes / 1024
	if limitKiB == 0 {
		return 0, errors.New("filesystem is too small for quota enforcement")
	}
	return limitKiB, nil
}
