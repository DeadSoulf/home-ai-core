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
	PreviousManifest string
}

func callModuleHelper(ctx context.Context, operation, moduleID, image string) (helperModuleResult, error) {
	return callModuleHelperWithProgress(ctx, operation, moduleID, image, nil)
}

func callModuleHelperWithProgress(ctx context.Context, operation, moduleID, image string, progress func(stage, detail string)) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{Operation: operation, ModuleID: moduleID, Image: image}, progress)
}

// Only the registry can supply old/new pinned images. No browser-provided
// previous manifest or arbitrary Docker target is passed through this path.
func callAIUpgradeHelper(ctx context.Context, operation, oldImage, newImage, oldManifest string, progress func(string, string)) (helperModuleResult, error) {
	return callModuleHelperRequest(ctx, updaterhelper.Request{
		Operation: operation, ModuleID: "ai.agent", Image: newImage,
		PreviousImage: oldImage, PreviousManifest: oldManifest,
	}, progress)
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

	request.ProtocolVersion = updaterhelper.ProtocolVersion
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
			PreviousManifest: response.PreviousManifest,
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
