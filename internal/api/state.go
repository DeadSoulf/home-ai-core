package api

import "context"

type State interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
	ClearTerminalJobs(ctx context.Context) (int64, error)
	DiskNames(ctx context.Context) (map[string]string, error)
	SetDiskName(ctx context.Context, stableID, displayName string) error
}
