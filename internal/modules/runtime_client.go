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
	return callModuleHelperWithProgress(ctx, operation, moduleID, image, nil)
}

func callModuleHelperWithProgress(ctx context.Context, operation, moduleID, image string, progress func(stage, detail string)) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{Operation: operation, ProtocolVersion: updaterhelper.ProtocolVersion, ModuleID: moduleID, Image: image}, progress)
}

// A separate operation carries both immutable digests so the privileged helper
// can refuse to touch a container whose image differs from the recorded one.
func callModuleUpgradeHelper(ctx context.Context, operation, moduleID, image, previousImage string, progress func(string, string)) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{Operation: operation, ProtocolVersion: updaterhelper.ProtocolVersion, ModuleID: moduleID, Image: image, PreviousImage: previousImage}, progress)
}

func callModuleHelperRequest(ctx context.Context, request updaterhelper.Request, progress func(stage, detail string)) (helperModuleResult, error) {
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

	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return helperModuleResult{}, err
	}

	decoder := json.NewDecoder(conn)
	for frames := 0; frames < 4096; frames++ {
		var response updaterhelper.Response
		if err := decoder.Decode(&response); err != nil {
			return helperModuleResult{}, fmt.Errorf("read module helper response: %w", err)
		}
		if response.ProgressStage != "" {
			if progress != nil {
				progress(response.ProgressStage, response.ProgressDetail)
			}
			continue
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
	return helperModuleResult{}, errors.New("too many module progress frames")
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
