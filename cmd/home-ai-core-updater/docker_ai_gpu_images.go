package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Both OCI index digests come from the SAME AI Agent 0.3.0 source commit.
// The CPU image is the catalog default. No untrusted request can choose an
// image URL; the privileged helper selects the exact paired image by digest.
const (
	aiCPU030Image = "ghcr.io/deadsoulf/home-ai-core-ai@sha256:f293ac8deead89561df585b13516b25999bd4a1b103725235c80608919ab86a6"
	aiGPU030Image = "ghcr.io/deadsoulf/home-ai-core-ai@sha256:df8587d727e1e185c916e7f34c21516d08fb0cee272cb71f54d3a4e8c0bc22c6"
)

func aiImageForDeviceSelection(currentImage string, gpuCount int) (string, error) {
	if !imageDigestPattern.MatchString(currentImage) {
		return "", errors.New("existing AI image is not an immutable OCI digest")
	}
	if gpuCount < 0 || gpuCount > 8 {
		return "", errors.New("invalid number of GPUs")
	}
	// Legacy 0.2.0 images included all backends. Keep their previous behavior,
	// but never silently upgrade a legacy container through a GPU operation.
	if currentImage != aiCPU030Image && currentImage != aiGPU030Image {
		return currentImage, nil
	}
	if gpuCount != 0 {
		return aiGPU030Image, nil
	}
	return aiCPU030Image, nil
}

func aiPinnedImageCached(ctx context.Context, dockerPath, image string) (bool, error) {
	if image != aiGPU030Image && image != aiCPU030Image {
		return false, errors.New("image not approved for AI runtime switching")
	}
	output, err := exec.CommandContext(ctx, dockerPath, "image", "inspect", "--format", "{{.Id}}", image).CombinedOutput()
	if err == nil {
		id := strings.TrimSpace(string(output))
		if !localImageIDPattern.MatchString(id) {
			return false, errors.New("invalid cached image ID")
		}
		return true, nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no such image") {
		return false, nil
	}
	return false, fmt.Errorf("cannot inspect pinned AI image: %s", commandError(output, err))
}

func aiOptionalGPUImageReady(ctx context.Context) (bool, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return false, errors.New("Docker unavailable")
	}
	// Reuse strict container ownership/network proof before disclosing state.
	container, err := inspectAIContainer(ctx, dockerPath)
	if err != nil {
		return false, err
	}
	if container.Config.Image != aiCPU030Image && container.Config.Image != aiGPU030Image {
		return false, errors.New("optional GPU package requires installed AI Agent 0.3.0")
	}
	return aiPinnedImageCached(ctx, dockerPath, aiGPU030Image)
}
