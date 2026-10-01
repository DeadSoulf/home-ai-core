package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
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

type userQuotaStat struct {
	UsedKiB      uint64
	HardLimitKiB uint64
}

func quotaToolsAvailable() bool {
	for _, name := range []string{"setquota", "repquota"} {
		if _, err := exec.LookPath(name); err != nil {
			return false
		}
	}
	return true
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
		info, err := inspectQuotaMount(ctx, policy.RootPath)
		if err != nil {
			if policy.ReservePercent == 0 {
				continue
			}
			return false, err.Error()
		}
		if err := quotaMountReady(ctx, info); err != nil {
			if policy.ReservePercent == 0 {
				continue
			}
			return false, err.Error()
		}
		quota, err := userQuota(ctx, info.RootPath, uid)
		if err != nil {
			return false, err.Error()
		}
		if policy.ReservePercent == 0 {
			if quota.HardLimitKiB != 0 {
				return false, fmt.Sprintf(
					"kernel quota on %s is still limited to %d KiB while the pool reserve is disabled",
					info.RootPath,
					quota.HardLimitKiB,
				)
			}
			continue
		}
		safeLimit, err := quotaSafeHardLimitKiB(
			info.RootPath,
			quota.UsedKiB,
			policy.ReservePercent,
		)
		if err != nil {
			return false, err.Error()
		}
		if quota.HardLimitKiB == 0 || quota.HardLimitKiB > safeLimit {
			return false, fmt.Sprintf(
				"kernel quota on %s is out of sync: hard limit %d KiB, safe maximum %d KiB",
				info.RootPath,
				quota.HardLimitKiB,
				safeLimit,
			)
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
		if err := enforceNASUserQuota(ctx, policy.RootPath, policy.ReservePercent, uid); err != nil {
			return err
		}
	}
	return nil
}

func enforceNASUserQuota(ctx context.Context, root string, reservePercent, uid int) error {
	info, err := inspectQuotaMount(ctx, root)
	if err != nil {
		return err
	}
	if err := quotaMountReady(ctx, info); err != nil {
		if reservePercent == 0 {
			return nil
		}
		return err
	}
	var limitKiB uint64
	if reservePercent > 0 {
		quota, err := userQuota(ctx, info.RootPath, uid)
		if err != nil {
			return err
		}
		limitKiB, err = quotaSafeHardLimitKiB(info.RootPath, quota.UsedKiB, reservePercent)
		if err != nil {
			return err
		}
	}
	setquota, err := exec.LookPath("setquota")
	if err != nil {
		return errors.New("quota tools are unavailable; install the quota package")
	}
	args := []string{
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
	actual, err := userQuota(ctx, info.RootPath, uid)
	if err != nil {
		return err
	}
	if actual.HardLimitKiB != limitKiB {
		return fmt.Errorf(
			"verify hard quota on %s: got %d KiB, want %d KiB",
			info.RootPath,
			actual.HardLimitKiB,
			limitKiB,
		)
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
	sort.Slice(result, func(i, j int) bool { return result[i].RootPath < result[j].RootPath })
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
	if !quotaToolsAvailable() {
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
		if !hasUserQuotaMountOption(info.Options) {
			return errors.New("ext4 user quota accounting is not active; remount this Home-AI storage with usrquota")
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
	return parseExt4QuotaFeature(string(output))
}

func parseExt4QuotaFeature(output string) (bool, error) {
	for _, line := range strings.Split(output, "\n") {
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

func quotaSafeHardLimitKiB(root string, usedKiB uint64, reservePercent int) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(root, &stat); err != nil {
		return 0, fmt.Errorf("inspect filesystem capacity: %w", err)
	}
	totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
	freeBytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	return quotaSafeHardLimitKiBForCapacity(totalBytes, freeBytes, usedKiB, reservePercent)
}

func quotaSafeHardLimitKiBForCapacity(
	totalBytes, freeBytes, usedKiB uint64,
	reservePercent int,
) (uint64, error) {
	if reservePercent < 0 || reservePercent > 50 {
		return 0, errors.New("reserve percent must be between 0 and 50")
	}
	if reservePercent == 0 {
		return 0, nil
	}
	reserve := percentBytes(totalBytes, reservePercent)
	var writableBytes uint64
	if freeBytes > reserve {
		writableBytes = freeBytes - reserve
	}
	writableKiB := writableBytes / 1024
	if ^uint64(0)-usedKiB < writableKiB {
		return 0, errors.New("quota hard limit overflows")
	}
	limit := usedKiB + writableKiB
	// A hard limit of zero means "unlimited" to Linux quota tools. When
	// the configured reserve is already reached on an otherwise empty UID,
	// keep the limit at the smallest enforceable value instead.
	if limit == 0 {
		return 1, nil
	}
	return limit, nil
}

func percentBytes(total uint64, percent int) uint64 {
	if total == 0 || percent <= 0 {
		return 0
	}
	value := uint64(percent)
	return (total/100)*value + ((total%100)*value)/100
}

func userQuota(ctx context.Context, root string, uid int) (userQuotaStat, error) {
	repquota, err := exec.LookPath("repquota")
	if err != nil {
		return userQuotaStat{}, errors.New("repquota is unavailable; install the quota package")
	}
	output, err := exec.CommandContext(
		ctx,
		repquota,
		"-u",
		"-n",
		"-O", "csv",
		root,
	).CombinedOutput()
	if err != nil {
		return userQuotaStat{}, fmt.Errorf("inspect user quota on %s: %s", root, strings.TrimSpace(string(output)))
	}
	stat, found, err := parseUserQuotaCSV(string(output), uid)
	if err != nil {
		return userQuotaStat{}, fmt.Errorf("inspect user quota on %s: %w", root, err)
	}
	if !found {
		return userQuotaStat{}, nil
	}
	return stat, nil
}

func parseUserQuotaCSV(output string, uid int) (userQuotaStat, bool, error) {
	wantID := strconv.Itoa(uid)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ",")
		if len(fields) < 6 || strings.TrimSpace(fields[0]) != wantID {
			continue
		}
		usedValue := strings.TrimSpace(fields[3])
		hardValue := strings.TrimSpace(fields[5])
		used, err := strconv.ParseUint(usedValue, 10, 64)
		if err != nil {
			return userQuotaStat{}, false, fmt.Errorf("invalid used space %q", usedValue)
		}
		hard, err := strconv.ParseUint(hardValue, 10, 64)
		if err != nil {
			return userQuotaStat{}, false, fmt.Errorf("invalid hard limit %q", hardValue)
		}
		return userQuotaStat{UsedKiB: used, HardLimitKiB: hard}, true, nil
	}
	return userQuotaStat{}, false, nil
}
