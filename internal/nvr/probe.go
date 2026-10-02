package nvr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMediaRuntimeUnavailable = errors.New("NVR media runtime is unavailable")
	ErrRTSPAuthentication      = errors.New("RTSP authentication failed")
	ErrRTSPConnection          = errors.New("RTSP connection failed")
	ErrRTSPNoVideo             = errors.New("RTSP source has no video stream")
)

type ProbeRequest struct {
	Address    string
	Transport  string
	Credential CameraCredential
}

type ProbeResult struct {
	Codec      string  `json:"codec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`
	BitrateBPS int64   `json:"bitrate_bps"`
	HasAudio   bool    `json:"has_audio"`
}

type CameraProber interface {
	Available() bool
	Probe(context.Context, ProbeRequest) (ProbeResult, error)
}

type FFProbe struct {
	path    string
	timeout time.Duration
}

func NewFFProbe() *FFProbe {
	path, _ := exec.LookPath("ffprobe")
	return &FFProbe{path: path, timeout: 12 * time.Second}
}

func (p *FFProbe) Available() bool {
	return p != nil && p.path != ""
}

func (p *FFProbe) Probe(ctx context.Context, request ProbeRequest) (ProbeResult, error) {
	if !p.Available() {
		return ProbeResult{}, ErrMediaRuntimeUnavailable
	}
	address, err := normalizeRTSPAddress(request.Address)
	if err != nil {
		return ProbeResult{}, err
	}
	transport := strings.ToLower(strings.TrimSpace(request.Transport))
	if transport == "" {
		transport = "tcp"
	}
	if transport != "tcp" && transport != "udp" {
		return ProbeResult{}, errors.New("RTSP transport must be tcp or udp")
	}
	probeURL, err := credentialedRTSPAddress(address, request.Credential)
	if err != nil {
		return ProbeResult{}, err
	}
	timeout := p.timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	command := exec.CommandContext(
		probeCtx,
		p.path,
		"-v", "error",
		"-f", "concat",
		"-safe", "0",
		"-protocol_whitelist", "file,pipe,rtsp,tcp,udp,rtp,tls,http,https,crypto",
		"-i", "pipe:0",
		"-show_streams",
		"-show_format",
		"-of", "json",
	)
	command.Stdin = strings.NewReader(ffconcatRTSPInput(probeURL, transport))
	output, err := command.Output()
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return ProbeResult{}, fmt.Errorf("%w: timeout", ErrRTSPConnection)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			safe := strings.ToLower(string(exitErr.Stderr))
			switch {
			case strings.Contains(safe, "401"), strings.Contains(safe, "unauthorized"), strings.Contains(safe, "authentication"):
				return ProbeResult{}, ErrRTSPAuthentication
			case strings.Contains(safe, "connection refused"),
				strings.Contains(safe, "connection timed out"),
				strings.Contains(safe, "no route to host"),
				strings.Contains(safe, "network is unreachable"),
				strings.Contains(safe, "name or service not known"),
				strings.Contains(safe, "temporary failure in name resolution"):
				return ProbeResult{}, ErrRTSPConnection
			}
		}
		return ProbeResult{}, errors.New("RTSP probe failed")
	}

	var decoded struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			AvgFPS    string `json:"avg_frame_rate"`
			RealFPS   string `json:"r_frame_rate"`
			BitRate   string `json:"bit_rate"`
		} `json:"streams"`
		Format struct {
			BitRate string `json:"bit_rate"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		return ProbeResult{}, errors.New("RTSP probe returned invalid media metadata")
	}
	var result ProbeResult
	for _, stream := range decoded.Streams {
		switch stream.CodecType {
		case "video":
			if result.Codec != "" {
				continue
			}
			result.Codec = strings.ToLower(strings.TrimSpace(stream.CodecName))
			result.Width = stream.Width
			result.Height = stream.Height
			result.FPS = parseFFProbeRate(stream.AvgFPS)
			if result.FPS == 0 {
				result.FPS = parseFFProbeRate(stream.RealFPS)
			}
			result.BitrateBPS, _ = strconv.ParseInt(strings.TrimSpace(stream.BitRate), 10, 64)
		case "audio":
			result.HasAudio = true
		}
	}
	if result.Codec == "" {
		return ProbeResult{}, ErrRTSPNoVideo
	}
	if result.BitrateBPS == 0 {
		result.BitrateBPS, _ = strconv.ParseInt(strings.TrimSpace(decoded.Format.BitRate), 10, 64)
	}
	return result, nil
}


func ffconcatRTSPInput(address, transport string) string {
	escaped := strings.ReplaceAll(address, "'", "'\\''")
	return "ffconcat version 1.0\n" +
		"file '" + escaped + "'\n" +
		"option rtsp_transport " + transport + "\n"
}

func normalizeRTSPAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", errors.New("camera RTSP address must be an absolute URL")
	}
	if parsed.Scheme != "rtsp" && parsed.Scheme != "rtsps" {
		return "", errors.New("camera address must use rtsp or rtsps")
	}
	if parsed.User != nil {
		return "", errors.New("camera credentials must be entered separately from the RTSP URL")
	}
	if parsed.Fragment != "" {
		return "", errors.New("camera RTSP address must not contain a fragment")
	}
	return parsed.String(), nil
}

func credentialedRTSPAddress(address string, credential CameraCredential) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return "", errors.New("camera RTSP address is invalid")
	}
	username := strings.TrimSpace(credential.Username)
	if username == "" && credential.Password == "" {
		return parsed.String(), nil
	}
	if username == "" {
		return "", errors.New("camera username is required when a password is supplied")
	}
	parsed.User = url.UserPassword(username, credential.Password)
	return parsed.String(), nil
}

func parseFFProbeRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	numerator, denominator, ok := strings.Cut(value, "/")
	if !ok {
		result, _ := strconv.ParseFloat(value, 64)
		return result
	}
	n, errN := strconv.ParseFloat(numerator, 64)
	d, errD := strconv.ParseFloat(denominator, 64)
	if errN != nil || errD != nil || d == 0 {
		return 0
	}
	return n / d
}
