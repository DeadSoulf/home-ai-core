//go:build !windows

package main

import (
	"fmt"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
)

func prepareAgentRuntime(logPath string) (func() error, error) {
	return nil, fmt.Errorf("prepare background agent: %w", windowsclient.ErrUserAutostartUnsupported)
}
