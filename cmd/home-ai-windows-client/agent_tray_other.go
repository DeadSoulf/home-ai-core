//go:build !windows

package main

func startAgentTray(logPath, configPath string) (*agentTrayRuntime, error) {
	return &agentTrayRuntime{}, nil
}
