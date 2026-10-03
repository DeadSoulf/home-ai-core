package nvr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/DeadSoulf/home-ai-core/internal/updaterhelper"
)

const runtimeHelperSocketPath = "/run/home-ai-core-updater.sock"

func InstallMediaRuntime(ctx context.Context) (string, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "unix", runtimeHelperSocketPath)
	if err != nil {
		return "", fmt.Errorf("connect NVR runtime helper: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(12 * time.Minute)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	_ = conn.SetDeadline(deadline)

	if err := json.NewEncoder(conn).Encode(updaterhelper.Request{
		Operation:       "nvr.runtime.install",
		ProtocolVersion: updaterhelper.ProtocolVersion,
	}); err != nil {
		return "", err
	}

	var response updaterhelper.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return "", err
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "NVR runtime helper rejected installation"
		}
		return "", errors.New(response.Error)
	}
	return response.Message, nil
}
