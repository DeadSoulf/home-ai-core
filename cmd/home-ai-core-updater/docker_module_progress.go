package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
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
	lastReport := time.Time{}
	var stdoutError boundedPullOutput
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = io.WriteString(&stdoutError, line)
		detail := sanitizedDockerPullDetail(line)
		if detail == "" || detail == lastDetail || (!lastReport.IsZero() && time.Since(lastReport) < 700*time.Millisecond) {
			continue
		}
		progress("download", detail)
		lastDetail = detail
		lastReport = time.Now()
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
