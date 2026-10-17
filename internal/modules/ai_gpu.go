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
