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

	CreateNASPool(ctx context.Context, name, rootPath, createdBy string, now time.Time) (state.NASPoolRecord, error)
	ListNASPools(ctx context.Context) ([]state.NASPoolRecord, error)
	CreateNASFolder(
		ctx context.Context,
		poolID, name, kind, ownerUserID, createdBy string,
		now time.Time,
	) (state.NASFolderRecord, error)
	ListNASFolders(ctx context.Context) ([]state.NASFolderRecord, error)
	NASFolder(ctx context.Context, folderID string) (state.NASFolderRecord, error)
	DeleteNASFolder(ctx context.Context, folderID string) error
}
