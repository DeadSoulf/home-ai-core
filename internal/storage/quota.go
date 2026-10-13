package storage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/helperipc"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

type QuotaStatus struct{ LimitBytes, UsedBytes uint64 }

type NASUsage struct {
	UsedBytes     int64
	ReservedBytes int64
}

func quotaHelper(ctx context.Context, request updaterhelper.Request) (updaterhelper.Response, error) {
	request.ProtocolVersion = updaterhelper.ProtocolVersion
	conn, cleanup, err := helperipc.Dial(ctx, helperSocketPath, 45*time.Second)
	if err != nil {
		return updaterhelper.Response{}, err
	}
	defer cleanup()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return updaterhelper.Response{}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return response, err
	}
	if !response.OK {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func ApplyNASQuota(ctx context.Context, root, relative string, project uint32, quota int64) error {
	_, err := quotaHelper(ctx, updaterhelper.Request{Operation: "storage.nas.quota.set", RootPath: root, RelativePath: relative, ProjectID: project, QuotaBytes: quota})
	return err
}
func InspectNASQuota(ctx context.Context, root string, project uint32) (QuotaStatus, error) {
	response, err := quotaHelper(ctx, updaterhelper.Request{Operation: "storage.nas.quota.inspect", RootPath: root, ProjectID: project})
	return QuotaStatus{LimitBytes: response.QuotaLimitBytes, UsedBytes: response.QuotaUsedBytes}, err
}

func InspectNASUsage(ctx context.Context, root, relative string) (NASUsage, error) {
	response, err := quotaHelper(ctx, updaterhelper.Request{
		Operation:    "storage.nas.usage.inspect",
		RootPath:     root,
		RelativePath: relative,
	})
	if err != nil {
		return NASUsage{}, err
	}
	return NASUsage{UsedBytes: response.UsageUsedBytes, ReservedBytes: response.UsageReservedBytes}, nil
}
