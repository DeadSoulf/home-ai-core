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
	aiCPU031Image = "ghcr.io/deadsoulf/home-ai-core-ai@sha256:a1aafe7d847d284f03d85e3d0162f4704e8bec4c97047768d6992c38e9981f40"
	aiGPU031Image = "ghcr.io/deadsoulf/home-ai-core-ai@sha256:42eca8a3e7e2bfe13a704d4dbcd75e1afcaf2d889d59c78514f4030307b6dcff"
)

func aiImageForDeviceSelection(currentImage string, gpuCount int) (string, error) {
	if !imageDigestPattern.MatchString(currentImage) {
		return "", errors.New("existing AI image is not an immutable OCI digest")
	}
	if gpuCount < 0 || gpuCount > 8 {
		return "", errors.New("invalid number of GPUs")
	}
	// Only switch between matching CPU/GPU images from one verified build.
	// Never downgrade 0.3.1 to 0.3.0 when changing device assignment.
	switch currentImage {
	case aiCPU030Image, aiGPU030Image:
		if gpuCount != 0 {
			return aiGPU030Image, nil
		}
		return aiCPU030Image, nil
	case aiCPU031Image, aiGPU031Image:
		if gpuCount != 0 {
			return aiGPU031Image, nil
		}
		return aiCPU031Image, nil
	default:
		// Legacy 0.2.0 images included all backends and use their own image.
		return currentImage, nil
	}
}

func aiGPUImageForInstalled(image string) (string, error) {
	switch image {
	case aiCPU030Image, aiGPU030Image:
		return aiGPU030Image, nil
	case aiCPU031Image, aiGPU031Image:
		return aiGPU031Image, nil
	default:
		return "", errors.New("optional GPU package requires AI Agent 0.3.0 or 0.3.1")
	}
}

func aiPinnedImageCached(ctx context.Context, dockerPath, image string) (bool, error) {
	if image != aiGPU030Image && image != aiCPU030Image && image != aiGPU031Image && image != aiCPU031Image {
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
	target, err := aiGPUImageForInstalled(container.Config.Image)
	if err != nil {
		return false, err
	}
	return aiPinnedImageCached(ctx, dockerPath, target)
}

func prepareOptionalAIGPUImage(ctx context.Context, progress moduleProgressFunc) error {
	if !moduleOperationMu.TryLock() {
		return errors.New("another Docker module operation is running")
	}
	defer moduleOperationMu.Unlock()
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return errors.New("Docker unavailable")
	}
	// Restrict this to a verified installed AI 0.3.0. All subsequent OCI
	// requests are pinned by the helper, never by the API or model.
	container, err := inspectAIContainer(ctx, dockerPath)
	if err != nil {
		return err
	}
	target, err := aiGPUImageForInstalled(container.Config.Image)
	if err != nil {
		return err
	}
	cached, err := aiPinnedImageCached(ctx, dockerPath, target)
	if err != nil {
		return err
	}
	if cached {
		progress("downloaded", "")
		return nil
	}
	progress("download", "")
	if err := pullDockerModuleImage(ctx, dockerPath, target, progress); err != nil {
		return err
	}
	cached, err = aiPinnedImageCached(ctx, dockerPath, target)
	if err != nil {
		return err
	}
	if !cached {
		return errors.New("GPU image pull finished but pinned image was not verified")
	}
	return nil
}
