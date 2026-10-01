package api

import (
	"context"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"math"
)

// The broker applies the existing pool-wide Core UID quota before granting
// SMB write access. Project quotas add folder caps, and allocate a personal
// user's finite budget across all their private folders.
func smbWritableFolders(ctx context.Context, folders []state.NASFolderRecord, userQuotas map[string]int64) map[string]bool {
	allowed := map[string]bool{}
	userBudget := map[string]int64{}
	userSafe := map[string]bool{}
	for _, folder := range folders {
		if folder.Kind == "private" {
			if _, seen := userSafe[folder.OwnerUserID]; !seen {
				userSafe[folder.OwnerUserID] = true
			}
		}
		if folder.HardQuotaBytes <= 0 || folder.ProjectID == 0 {
			// Unlimited folders retain the prior pool-reserve protection.
			// A finite product limit cannot be bypassed through SMB.
			allowed[folder.ID] = folder.QuotaBytes == 0 && (folder.Kind != "private" || userQuotas[folder.OwnerUserID] == 0)
			if folder.Kind == "private" {
				userSafe[folder.OwnerUserID] = false
			}
			continue
		}
		status, err := inspectFolderHardQuota(ctx, folder.PoolRoot, folder.ProjectID+1000000)
		// Floor-to-KiB is intentional; the kernel never receives an unlimited
		// zero block limit for a positive product quota.
		limit := uint64(folder.HardQuotaBytes/1024) * 1024
		if err != nil || status.LimitBytes != limit || status.UsedBytes > limit {
			userSafe[folder.OwnerUserID] = false
			continue
		}
		if folder.Kind == "private" {
			if folder.HardQuotaBytes > math.MaxInt64-userBudget[folder.OwnerUserID] {
				userSafe[folder.OwnerUserID] = false
			} else {
				userBudget[folder.OwnerUserID] += folder.HardQuotaBytes
			}
		}
		allowed[folder.ID] = true
	}
	for _, folder := range folders {
		safe := allowed[folder.ID]
		if folder.Kind == "private" && userQuotas[folder.OwnerUserID] > 0 && (!userSafe[folder.OwnerUserID] || userBudget[folder.OwnerUserID] > userQuotas[folder.OwnerUserID]) {
			safe = false
		}
		allowed[folder.ID] = safe
	}
	return allowed
}
