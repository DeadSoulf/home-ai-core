package api

import (
	"context"
	"errors"
	"math"

	"github.com/DeadSoulf/home-ai-core/internal/filedata"
	"github.com/DeadSoulf/home-ai-core/internal/state"
)

// Keep space promised to resumable uploads and filesystem quota allocations.
// Without this, a Core write to an unbounded folder could spend space already
// allocated to an independently writable SMB project on the same pool.
func reservedPoolAllowance(ctx context.Context, folders []state.NASFolderRecord, target state.NASFolderRecord, uploadID string, available int64) (int64, error) {
	reserved := int64(0)
	hasHardQuota := false
	targetCap := int64(math.MaxInt64)
	for _, folder := range folders {
		if folder.PoolID != target.PoolID {
			continue
		}
		if folder.HardQuotaBytes > 0 {
			hasHardQuota = true
			status, err := inspectFolderHardQuota(ctx, folder.PoolRoot, folder.ProjectID+1000000)
			limit := uint64(folder.HardQuotaBytes/1024) * 1024
			if err != nil {
				return 0, err
			}
			if status.LimitBytes != limit || status.UsedBytes > limit {
				return 0, errors.New("filesystem quota does not match the folder allocation")
			}
			remaining := int64(limit - status.UsedBytes)
			if folder.ID == target.ID {
				targetCap = remaining
			} else {
				if remaining > math.MaxInt64-reserved {
					return 0, errors.New("pool reservations overflow")
				}
				reserved += remaining
			}
			// Upload parts in this folder consume its project allocation.
			continue
		}
		root, err := filedata.FolderRoot(folder.PoolRoot, folder.RelativePath)
		if err != nil {
			return 0, err
		}
		uploads, err := filedata.ListUploads(root)
		if err != nil {
			return 0, err
		}
		for _, upload := range uploads {
			if folder.ID == target.ID && upload.ID == uploadID {
				continue
			}
			pending := upload.TotalBytes - upload.ReceivedBytes
			if pending > math.MaxInt64-reserved {
				return 0, errors.New("pool reservations overflow")
			}
			reserved += pending
		}
	}
	if hasHardQuota {
		capacity, err := readFilePoolCapacity(target.PoolRoot)
		if err != nil {
			return 0, err
		}
		headroom := max(uint64(16<<20), capacity.TotalBytes/100)
		if headroom > math.MaxInt64 || int64(headroom) > math.MaxInt64-reserved {
			return 0, nil
		}
		reserved += int64(headroom)
	}
	return min(targetCap, max(int64(0), available-reserved)), nil
}
