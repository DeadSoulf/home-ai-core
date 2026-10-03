package api

import (
	"context"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/state"
)

type State interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
	ClearTerminalJobs(ctx context.Context) (int64, error)
	DiskNames(ctx context.Context) (map[string]string, error)
	SetDiskName(ctx context.Context, stableID, displayName string) error
	SetStoragePurpose(ctx context.Context, devicePath, filesystemUUID, purpose string, now time.Time) (state.StoragePurposeRecord, error)
	ClearStoragePurpose(ctx context.Context, devicePath, filesystemUUID string) error
	ListStoragePurposes(ctx context.Context) ([]state.StoragePurposeRecord, error)
	SetNVRStorageTarget(
		ctx context.Context,
		devicePath, filesystemUUID, mountpoint string,
		reservePercent int,
		active bool,
		now time.Time,
	) (state.NVRStorageTargetRecord, error)
	ActiveNVRStorageTarget(ctx context.Context) (state.NVRStorageTargetRecord, error)
	NVRArchiveBytes(ctx context.Context, storageTargetID string) (int64, error)

	CreateNASPool(ctx context.Context, name, rootPath, storageDevicePath, storageFilesystemUUID, createdBy string, now time.Time) (state.NASPoolRecord, error)
	NASPool(ctx context.Context, poolID string) (state.NASPoolRecord, error)
	ListNASPools(ctx context.Context) ([]state.NASPoolRecord, error)
	UpdateNASPoolCapacityPolicy(ctx context.Context, poolID string, reservePercent, warningPercent int, now time.Time) (state.NASPoolRecord, error)
	CreateNASFolder(
		ctx context.Context,
		poolID, name, kind, ownerUserID, createdBy string,
		now time.Time,
	) (state.NASFolderRecord, error)
	ListNASFolders(ctx context.Context) ([]state.NASFolderRecord, error)
	NASFolder(ctx context.Context, folderID string) (state.NASFolderRecord, error)
	DeleteNASFolder(ctx context.Context, folderID string) error
}
