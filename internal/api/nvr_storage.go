package api

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/security"
	"github.com/DeadSoulf/home-ai-core/internal/state"
	"github.com/DeadSoulf/home-ai-core/internal/storage"
	"github.com/DeadSoulf/home-ai-core/internal/systeminfo"
)

// The capacity of a disk assigned to NVR is separate from the bytes of video
// recorded by the running NVR container. A storage-purpose assignment does not
// automatically change the container's /data bind mount.
type nvrStorageDisk struct {
	Device        string `json:"device"`
	Label         string `json:"label,omitempty"`
	Filesystem    string `json:"filesystem,omitempty"`
	Mountpoint    string `json:"mountpoint,omitempty"`
	State         string `json:"state"` // missing, unmounted, mounted
	SizeBytes     uint64 `json:"size_bytes,omitempty"`
	TotalBytes    uint64 `json:"total_bytes,omitempty"`
	FreeBytes     uint64 `json:"free_bytes,omitempty"`
	UsedBytes     uint64 `json:"used_bytes,omitempty"`
	CapacityKnown bool   `json:"capacity_known"`
}

type nvrStorageSnapshot struct {
	Disks         []nvrStorageDisk `json:"disks"`
	AssignedCount int              `json:"assigned_count"`
	MountedCount  int              `json:"mounted_count"`
	CapacityKnown bool             `json:"capacity_known"`
	TotalBytes    uint64           `json:"total_bytes,omitempty"`
	FreeBytes     uint64           `json:"free_bytes,omitempty"`
}

// nvrCapacityFromInventory only trusts the current block-device inventory and
// existing NVR-purpose assignments. Device identity is matched by UUID first
// so the selected disk survives reboots that rename /dev/sdX devices.
func nvrCapacityFromInventory(
	assignments []state.StoragePurposeRecord,
	nodes []systeminfo.BlockNode,
	inspection storage.Inspection,
) nvrStorageSnapshot {
	result := nvrStorageSnapshot{Disks: make([]nvrStorageDisk, 0), CapacityKnown: true}
	for _, assignment := range assignments {
		if assignment.Purpose != state.StoragePurposeNVR {
			continue
		}
		result.AssignedCount++
		disk := nvrStorageDisk{Device: assignment.DevicePath, State: "missing"}
		node, ok := blockNodeForStoragePurpose(nodes, assignment)
		if !ok || node.System || (node.Type != "part" && node.Type != "lvm") {
			result.CapacityKnown = false
			result.Disks = append(result.Disks, disk)
			continue
		}
		disk.Device = node.Path
		disk.Label = node.Label
		disk.Filesystem = node.Filesystem
		disk.SizeBytes = node.SizeBytes
		disk.State = "unmounted"
		for _, mp := range node.Mountpoints {
			mp = strings.TrimSpace(mp)
			if mp == "" || !filepath.IsAbs(mp) {
				continue
			}
			if disk.Mountpoint == "" || strings.HasPrefix(mp, "/mnt/home-ai-core/") {
				disk.Mountpoint = mp
			}
		}
		if disk.Mountpoint == "" {
			result.CapacityKnown = false
			result.Disks = append(result.Disks, disk)
			continue
		}
		disk.State = "mounted"
		result.MountedCount++

		// lsblk --bytes FSAVAIL reports current filesystem free space when
		// mounted. Prefer df/lsblk from the privileged helper when available.
		total, free := node.SizeBytes, node.FreeBytes
		freeKnown := node.FreeKnown
		for _, stat := range inspection.Filesystems {
			if filepath.Clean(strings.TrimSpace(stat.Device)) != filepath.Clean(node.Path) {
				continue
			}
			// Don't pair stale information from a different mountpoint with a
			// newly mounted device that happens to reuse its old path.
			if stat.Mountpoint != "" {
				found := false
				for _, mp := range node.Mountpoints {
					if filepath.Clean(mp) == filepath.Clean(stat.Mountpoint) {
						found = true
						break
					}
				}
				if !found {
					break
				}
			}
			if stat.TotalKnown && stat.TotalBytes > 0 {
				total = stat.TotalBytes
			}
			if stat.FreeKnown {
				free = stat.FreeBytes
				freeKnown = true
			}
			break
		}
		// Unknown is different from full. Never display a misleading "0 GB"
		// when inspection failed, or a negative/overflowed used-space value.
		if total == 0 || !freeKnown || free > total ||
			total > ^uint64(0)-result.TotalBytes || free > ^uint64(0)-result.FreeBytes {
			result.CapacityKnown = false
			result.Disks = append(result.Disks, disk)
			continue
		}
		disk.CapacityKnown = true
		disk.TotalBytes = total
		disk.FreeBytes = free
		disk.UsedBytes = total - free
		result.TotalBytes += total
		result.FreeBytes += free
		result.Disks = append(result.Disks, disk)
	}
	// Aggregates must never silently exclude an unmounted, missing or
	// unmeasurable assigned disk.
	if result.AssignedCount == 0 || !result.CapacityKnown {
		result.CapacityKnown = false
		result.TotalBytes = 0
		result.FreeBytes = 0
	}
	return result
}

// GET /api/v1/nvr/storage is read-only and limited to NVR-assigned disks.
// It exposes no Files pools, arbitrary device metadata or raw Docker data.
func (s *server) nvrStorage(w http.ResponseWriter, r *http.Request, _ security.Actor, _ authSource) {
	if !s.nvrReady(w, r) {
		return
	}
	if s.state == nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"NVR storage assignments are unavailable", nil)
		return
	}
	assignments, err := s.state.ListStoragePurposes(r.Context())
	if err != nil {
		writeAPIError(w, r, http.StatusServiceUnavailable, "nvr_storage_unavailable",
			"NVR storage assignments could not be read", nil)
		return
	}
	// An optional injected inventory keeps this endpoint independently
	// testable without invoking Docker, lsblk or the privileged helper.
	inventory := s.nvrStorageInventory
	if inventory == nil {
		inventory = func(ctx context.Context) ([]systeminfo.BlockNode, storage.Inspection) {
			nodes := systeminfo.Collect(s.nodeID).BlockTree
			scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			inspection, _ := storage.Inspect(scanCtx)
			return nodes, inspection
		}
	}
	nodes, inspection := inventory(r.Context())
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, nvrCapacityFromInventory(assignments, nodes, inspection))
}
