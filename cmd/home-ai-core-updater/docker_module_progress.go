package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Progress updates are strictly restricted to known phases and sanitized
// Docker layer tokens, so neither registry errors nor credentials are
// forwarded to the Web client via helper IPC/job history.
type moduleProgressFunc func(stage, detail string)

func firstModuleProgress(values []moduleProgressFunc) moduleProgressFunc {
	if len(values) != 0 && values[0] != nil {
		return values[0]
	}
	return func(string, string) {}
}

var (
	dockerLayerLine = regexp.MustCompile(`^([a-f0-9]{12}):\s*(Downloading|Extracting|Download complete|Pull complete|Verifying Checksum|Waiting|Already exists)`)
	dockerLayerSize = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?[kMGT]?B)/([0-9]+(?:\.[0-9]+)?[kMGT]?B)`)
)

// CLI pull output is per-layer, not whole-image progress. It must never be
// misrepresented as a percentage of the overall installation or image.
func sanitizedDockerPullDetail(line string) string {
	line = strings.TrimSpace(line)
	match := dockerLayerLine.FindStringSubmatch(line)
	if match == nil {
		return ""
	}
	switch match[2] {
	case "Downloading", "Extracting":
		if amount := dockerLayerSize.FindStringSubmatch(line); amount != nil {
			return match[1] + " " + match[2] + " " + amount[1] + "/" + amount[2]
		}
		return match[1] + " " + match[2]
	default:
		return match[1] + " " + match[2]
	}
}

// Docker reports compressed download byte counts per layer; other layers may
// not be announced yet. These are *known-layer* totals, not whole-image bytes.
type dockerLayerBytes struct {
	downloaded int64
	total      int64
}

type dockerPullTracker struct {
	layers map[string]dockerLayerBytes
}

func parseDockerByteSize(raw string) (int64, bool) {
	multipliers := []struct {
		suffix string
		factor float64
	}{
		{"TB", 1e12},
		{"GB", 1e9},
		{"MB", 1e6},
		{"kB", 1e3},
		{"B", 1},
	}
	for _, unit := range multipliers {
		if !strings.HasSuffix(raw, unit.suffix) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, unit.suffix), 64)
		bytes := value * unit.factor
		if err != nil || math.IsNaN(bytes) || math.IsInf(bytes, 0) || bytes < 0 || bytes > float64(1<<50) {
			return 0, false
		}
		return int64(bytes), true
	}
	return 0, false
}

func (t *dockerPullTracker) report(line string) string {
	detail := sanitizedDockerPullDetail(line)
	if detail == "" {
		return ""
	}
	match := dockerLayerLine.FindStringSubmatch(strings.TrimSpace(line))
	if match == nil {
		return ""
	}
	if t.layers == nil {
		t.layers = make(map[string]dockerLayerBytes)
	}
	layer, exists := t.layers[match[1]]
	if !exists && len(t.layers) >= 256 {
		return detail
	}
	switch match[2] {
	case "Downloading":
		if amounts := dockerLayerSize.FindStringSubmatch(line); amounts != nil {
			current, validCurrent := parseDockerByteSize(amounts[1])
			total, validTotal := parseDockerByteSize(amounts[2])
			if validCurrent && validTotal && total > 0 {
				if layer.total == 0 || layer.total == total {
					layer.total = total
					if current > total {
						current = total
					}
					if current > layer.downloaded {
						layer.downloaded = current
					}
				}
			}
		}
	case "Download complete", "Pull complete":
		if layer.total > 0 {
			layer.downloaded = layer.total
		}
	}
	t.layers[match[1]] = layer

	var received, knownSize int64
	for _, progress := range t.layers {
		if progress.total == 0 {
			continue
		}
		if knownSize > (1<<60)-progress.total {
			return detail
		}
		knownSize += progress.total
		received += progress.downloaded
	}
	if knownSize == 0 {
		return detail
	}
	return fmt.Sprintf("known:%d/%d %s", received, knownSize, detail)
}

// Split both terminal-style CR updates and regular line-based CLI output.
// bufio.Scanner has a hard cap so malicious/misbehaving Docker output cannot
// allocate unbounded memory in the privileged helper.
func splitDockerPullLines(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == 10 || b == 13 {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

type boundedPullOutput struct{ b bytes.Buffer }

func (w *boundedPullOutput) Write(p []byte) (int, error) {
	original := len(p)
	if len(p) > 4096 {
		p = p[len(p)-4096:]
	}
	remaining := 4096 - w.b.Len()
	if remaining < len(p) {
		old := w.b.Bytes()
		keep := old[len(p)-remaining:]
		w.b.Reset()
		_, _ = w.b.Write(keep)
	}
	_, _ = w.b.Write(p)
	return original, nil
}

func pullDockerImageWithProgress(ctx context.Context, binary string, env []string, image string, progress moduleProgressFunc) error {
	cmd := exec.CommandContext(ctx, binary, "pull", image)
	if env != nil {
		cmd.Env = env
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open Docker image pull: %w", err)
	}
	var stderr boundedPullOutput
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Docker image pull: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(splitDockerPullLines)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	lastDetail := ""
	lastSent := ""
	lastReport := time.Time{}
	tracker := &dockerPullTracker{}
	var stdoutError boundedPullOutput
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = io.WriteString(&stdoutError, line)
		detail := tracker.report(line)
		if detail == "" || detail == lastDetail {
			continue
		}
		lastDetail = detail
		if !lastReport.IsZero() && time.Since(lastReport) < 700*time.Millisecond {
			continue
		}
		progress("download", detail)
		lastSent = detail
		lastReport = time.Now()
	}
	// A short pull can finish within the throttle window; never discard the
	// final observed byte count before marking the image fully downloaded.
	if lastDetail != "" && lastDetail != lastSent {
		progress("download", lastDetail)
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("read Docker image pull progress: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		// Docker may report registry failures on stdout or stderr. Only the
		// existing protected error formatter sees those bounded diagnostics;
		// progress notifications never receive raw Docker output.
		raw := append(append([]byte(nil), stderr.b.Bytes()...), stdoutError.b.Bytes()...)
		return fmt.Errorf("pull module image: %s", commandError(raw, err))
	}
	progress("downloaded", "")
	return nil
}
