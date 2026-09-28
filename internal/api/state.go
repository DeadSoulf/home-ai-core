package api

import "context"

type State interface {
	Ping(ctx context.Context) error
	SchemaVersion(ctx context.Context) (int, error)
	ClearTerminalJobs(ctx context.Context) (int64, error)
}
