package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const nvrStorageMigrationConfirm = "MIGRATE_NVR_RECORDINGS"

func callNVRStorageHelper(ctx context.Context, req updaterhelper.Request, timeout time.Duration) (updaterhelper.Response, error) {
	req.ModuleID = "nvr.module"
	req.ProtocolVersion = updaterhelper.ProtocolVersion
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil {
		return updaterhelper.Response{}, fmt.Errorf("connect NVR storage helper: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return updaterhelper.Response{}, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return updaterhelper.Response{}, err
	}
	var resp updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return resp, fmt.Errorf("read NVR storage helper response: %w", err)
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "NVR video storage helper rejected the operation"
		}
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func NVRStorageActivate(ctx context.Context, mountpoint, uuid string) (string, error) {
	resp, err := callNVRStorageHelper(ctx, updaterhelper.Request{
		Operation: "docker.nvr.storage.activate",
		Mountpoint: mountpoint, FilesystemUUID: uuid,
		Confirm: nvrStorageMigrationConfirm,
	}, 31*time.Minute)
	if err != nil {
		return "", err
	}
	if resp.ModuleStorageMount == "" {
		return "", errors.New("NVR storage migration returned no mounted path")
	}
	return resp.ModuleStorageMount, nil
}

func NVRStorageStatus(ctx context.Context, uuid string) (string, error) {
	resp, err := callNVRStorageHelper(ctx, updaterhelper.Request{
		Operation: "docker.nvr.storage.status", FilesystemUUID: uuid,
	}, 12*time.Second)
	if err != nil {
		return "", err
	}
	return resp.ModuleStorageMount, nil
}
