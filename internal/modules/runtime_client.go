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

const (
	moduleHelperSocketPath = "/run/home-ai-core-updater.sock"
	moduleHelperIOTimeout  = 31 * time.Minute // helper has a 30-minute operation budget
)

type helperModuleResult struct {
	State       string
	ContainerID string
	Message     string
}

func callModuleHelper(ctx context.Context, operation, moduleID, image string) (helperModuleResult, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil {
		return helperModuleResult{}, fmt.Errorf("connect privileged module helper: %w", err)
	}
	cleanup, err := protectModuleHelperConn(ctx, conn, moduleHelperIOTimeout)
	if err != nil {
		return helperModuleResult{}, fmt.Errorf("set privileged module helper timeout: %w", err)
	}
	defer cleanup()

	request := updaterhelper.Request{
		Operation:       operation,
		ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID:        moduleID,
		Image:           image,
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return helperModuleResult{}, err
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return helperModuleResult{}, err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "privileged module helper rejected operation"
		}
		return helperModuleResult{}, errors.New(response.Error)
	}
	return helperModuleResult{
		State:       response.ModuleState,
		ContainerID: response.ContainerID,
		Message:     response.Message,
	}, nil
}

// protectModuleHelperConn enforces a deadline for the full JSON exchange.
// DialContext only constrains connection establishment; it does not bound reads
// once the privileged helper has accepted the connection.
func protectModuleHelperConn(ctx context.Context, conn net.Conn, timeout time.Duration) (func(), error) {
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return nil, err
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	return func() {
		stopCancellation()
		_ = conn.Close()
	}, nil
}

func callModuleUpgradeHelper(ctx context.Context, moduleID, previousImage, nextImage string) (helperModuleResult, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", moduleHelperSocketPath)
	if err != nil {
		return helperModuleResult{}, fmt.Errorf("connect privileged upgrade helper: %w", err)
	}
	cleanup, err := protectModuleHelperConn(ctx, conn, moduleHelperIOTimeout)
	if err != nil {
		return helperModuleResult{}, err
	}
	defer cleanup()
	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation: "docker.module.upgrade", ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID: moduleID, PreviousImage: previousImage, Image: nextImage,
	}); err != nil {
		return helperModuleResult{}, err
	}
	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return helperModuleResult{}, err
	}
	if !response.OK {
		if response.Error == "" { response.Error = "module upgrade helper failed" }
		return helperModuleResult{}, errors.New(response.Error)
	}
	return helperModuleResult{State: response.ModuleState,
		ContainerID: response.ContainerID, Message: response.Message}, nil
}
