package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/helperipc"
	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const aiGPUOperationTimeout = 162 * time.Second

func aiGPUCall(ctx context.Context, operation string, selected []string) ([]string, error) {
	conn, cleanup, err := helperipc.Dial(ctx, moduleHelperSocketPath, aiGPUOperationTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect AI GPU helper: %w", err)
	}
	defer cleanup()
	request := updaterhelper.Request{
		Operation:       operation,
		ProtocolVersion: updaterhelper.ProtocolVersion,
		ModuleID:        "ai.agent",
	}
	if operation == "docker.ai.gpu.apply" {
		request.Confirm = "ASSIGN_AI_GPU"
		request.AIGPUPCI = selected
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, err
	}
	var result updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		if result.Error == "" {
			result.Error = "AI GPU runtime rejected operation"
		}
		return nil, errors.New(result.Error)
	}
	if result.ProtocolVersion != updaterhelper.ProtocolVersion {
		return nil, errors.New("GPU helper protocol mismatch")
	}
	if result.AIGPUPCI == nil {
		return []string{}, nil
	}
	return result.AIGPUPCI, nil
}

func AssignedAIGPUs(ctx context.Context) ([]string, error) {
	return aiGPUCall(ctx, "docker.ai.gpu.status", nil)
}

// ApplyAIGPUs is an explicit privileged operation. It may briefly interrupt
// the AI module while its GPU device set is staged and health-verified.
func ApplyAIGPUs(ctx context.Context, pciAddresses []string) ([]string, error) {
	return aiGPUCall(ctx, "docker.ai.gpu.apply", pciAddresses)
}

 // AIGPUImageCached checks the exact pinned optional image via the trusted
 // helper, without exposing Docker access or a free-form registry URL.
func AIGPUImageCached(ctx context.Context) (bool, error) {
	result, err := callModuleHelper(ctx, "docker.ai.gpu.image.status", "ai.agent", "")
	if err != nil {
		return false, err
	}
	switch result.State {
	case "downloaded":
		return true, nil
	case "missing":
		return false, nil
	default:
		return false, errors.New("unexpected GPU image status")
	}
}

// PrepareAIGPUImage downloads the pinned acceleration image into Docker's
// cache. It does not stop, replace or reconfigure the running AI container.
func PrepareAIGPUImage(ctx context.Context, progress func(stage, detail string)) error {
	result, err := callModuleHelperWithProgress(ctx, "docker.ai.gpu.prepare", "ai.agent", "", progress)
	if err != nil {
		return err
	}
	if result.State != "downloaded" {
		return errors.New("GPU runtime download did not complete")
	}
	return nil
}
