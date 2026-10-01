package api

import (
	"context"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"math"
)

// A project quota is a physical allocation cap. SMB is writable only when all
// NAS folders sharing the reserve/user budget have finite verified caps. This
// conservatively allocates capacity instead of racing a per-write free-space
// sample in a transport that bypasses the Core API.
func smbWritableFolders(ctx context.Context, folders []state.NASFolderRecord, userQuotas map[string]int64) map[string]bool {
	allowed := map[string]bool{}
	poolBudget := map[string]uint64{}
	poolUsed := map[string]uint64{}
	poolSafe := map[string]bool{}
	userBudget := map[string]int64{}
	userSafe := map[string]bool{}
	for _, folder := range folders {
		if _, seen := poolSafe[folder.PoolID]; !seen {
			poolSafe[folder.PoolID] = true
		}
		if folder.Kind == "private" {
			if _, seen := userSafe[folder.OwnerUserID]; !seen {
				userSafe[folder.OwnerUserID] = true
			}
		}
		if folder.HardQuotaBytes <= 0 || folder.ProjectID == 0 {
			poolSafe[folder.PoolID] = false
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
			poolSafe[folder.PoolID] = false
			userSafe[folder.OwnerUserID] = false
			continue
		}
		if limit > math.MaxUint64-poolBudget[folder.PoolID] || status.UsedBytes > math.MaxUint64-poolUsed[folder.PoolID] {
			poolSafe[folder.PoolID] = false
			continue
		}
		poolBudget[folder.PoolID] += limit
		poolUsed[folder.PoolID] += status.UsedBytes
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
		if folder.PoolReservePercent > 0 {
			capacity, err := readFilePoolCapacity(folder.PoolRoot)
			if err != nil || !poolSafe[folder.PoolID] {
				safe = false
			} else {
				reserve := capacityPercentBytes(capacity.TotalBytes, folder.PoolReservePercent)
				// Leave additional filesystem metadata/inode headroom outside
				// block-quota allocations. Other host writers are outside NAS.
				overhead := max(uint64(16<<20), capacity.TotalBytes/100)
				pending := poolBudget[folder.PoolID] - poolUsed[folder.PoolID]
				if capacity.FreeBytes <= reserve || overhead > capacity.FreeBytes-reserve || pending > capacity.FreeBytes-reserve-overhead {
					safe = false
				}
			}
		}
		if folder.Kind == "private" && userQuotas[folder.OwnerUserID] > 0 && (!userSafe[folder.OwnerUserID] || userBudget[folder.OwnerUserID] > userQuotas[folder.OwnerUserID]) {
			safe = false
		}
		allowed[folder.ID] = safe
	}
	return allowed
}
