package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
	"net"
	"time"
)

type QuotaStatus struct{ LimitBytes, UsedBytes uint64 }

func quotaHelper(ctx context.Context, request updaterhelper.Request) (updaterhelper.Response, error) {
	request.ProtocolVersion = updaterhelper.ProtocolVersion
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", helperSocketPath)
	if err != nil {
		return updaterhelper.Response{}, err
	}
	defer conn.Close()
	deadline := time.Now().Add(45 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	_ = conn.SetDeadline(deadline)
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
