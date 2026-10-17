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

// The privileged helper is the only process that can inspect Docker mounts
// or recreate an installed container. The API cannot accept a raw host path
// from an HTTP client; the disk purpose must be verified by Core first.
func nvrStorageRequest(ctx context.Context, op, device, mountpoint string) (updaterhelper.Response, error) {
	timeout := 8*time.Second
	if op == "docker.module.storage.attach" { timeout = 3*time.Minute + 10*time.Second }
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil { return updaterhelper.Response{}, fmt.Errorf("connect NVR storage helper: %w", err) }
	defer conn.Close()
	limit := time.Now().Add(timeout)
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(limit) { limit = deadline }
	if err := conn.SetDeadline(limit); err != nil { return updaterhelper.Response{}, err }
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	err = json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation: op, ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID: "nvr.module", Device: device, Mountpoint: mountpoint,
	})
	if err != nil { return updaterhelper.Response{}, err }
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil { return updaterhelper.Response{}, err }
	if !response.OK {
		if response.Error == "" { response.Error = "NVR storage operation failed" }
		return updaterhelper.Response{}, errors.New(response.Error)
	}
	return response, nil
}

func ResolveNVRStorage(ctx context.Context) (string, error) {
	response, err := nvrStorageRequest(ctx, "docker.module.storage.inspect", "", "")
	return response.ModuleStorageRoot, err
}

func AttachNVRStorage(ctx context.Context, device, mountpoint string) error {
	_, err := nvrStorageRequest(ctx, "docker.module.storage.attach", device, mountpoint)
	return err
}
