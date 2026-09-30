//go:build !windows

package main

func startAgentTray(logPath, configPath, clientVersion string) (*agentTrayRuntime, error) {
	return &agentTrayRuntime{}, nil
}
