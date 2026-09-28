package modules

import (
	"context"
	"io"
)

type Operation string

const (
	OperationInstall Operation = "install"
	OperationUpgrade Operation = "upgrade"
	OperationRemove  Operation = "remove"
	OperationBackup  Operation = "backup"
	OperationRestore Operation = "restore"
)

type OperationContext struct {
	ModuleID    string
	FromVersion string
	ToVersion   string
	StateDir    string
}

type Progress interface {
	Progress(context.Context, int, string) error
}

type Lifecycle interface {
	Install(context.Context, OperationContext, Progress) error
	Upgrade(context.Context, OperationContext, Progress) error
	Remove(context.Context, OperationContext, Progress) error
	Backup(context.Context, OperationContext, Progress, io.Writer) error
	Restore(context.Context, OperationContext, Progress, io.Reader) error
}

type Module interface {
	Manifest() Manifest
	Lifecycle() Lifecycle
}
