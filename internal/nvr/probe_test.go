package nvr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFFProbeParsesVideoAndAudioMetadata(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffprobe")
	content := `#!/bin/sh
cat <<'EOF'
{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"25/1","bit_rate":"4000000"},{"codec_type":"audio","codec_name":"aac"}],"format":{"bit_rate":"4200000"}}
EOF
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	prober := &FFProbe{path: script, timeout: time.Second}
	result, err := prober.Probe(context.Background(), ProbeRequest{
		Address:   "rtsp://192.0.2.50/stream",
		Transport: "tcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Codec != "h264" || result.Width != 1920 || result.Height != 1080 {
		t.Fatalf("probe result = %#v", result)
	}
	if result.FPS != 25 || result.BitrateBPS != 4_000_000 || !result.HasAudio {
		t.Fatalf("probe metadata = %#v", result)
	}
}

func TestRTSPAddressRejectsEmbeddedCredentials(t *testing.T) {
	_, err := normalizeRTSPAddress("rtsp://user:password@192.0.2.10/stream")
	if err == nil {
		t.Fatal("credential-bearing RTSP URL was accepted")
	}
}

func TestFFProbeUnavailableFailsClosed(t *testing.T) {
	prober := &FFProbe{}
	_, err := prober.Probe(context.Background(), ProbeRequest{Address: "rtsp://192.0.2.10/stream"})
	if !errors.Is(err, ErrMediaRuntimeUnavailable) {
		t.Fatalf("error = %v, want ErrMediaRuntimeUnavailable", err)
	}
}


func TestFFProbeDoesNotExposeCredentialsInProcessArguments(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ffprobe")
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	content := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"cat > " + stdinFile + "\n" +
		"cat <<'EOF'\n" +
		"{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":640,\"height\":360,\"avg_frame_rate\":\"5/1\"}],\"format\":{}}\n" +
		"EOF\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	prober := &FFProbe{path: script, timeout: time.Second}
	_, err := prober.Probe(context.Background(), ProbeRequest{
		Address:   "rtsp://192.0.2.90/private",
		Transport: "tcp",
		Credential: CameraCredential{
			Username: "viewer",
			Password: "super-secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "viewer") || strings.Contains(string(args), "super-secret") {
		t.Fatalf("ffprobe argv leaked credentials: %s", args)
	}
	stdin, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdin), "viewer") || !strings.Contains(string(stdin), "super-secret") {
		t.Fatalf("ffconcat stdin did not contain protected source: %s", stdin)
	}
	if !strings.Contains(string(stdin), "option rtsp_transport tcp") {
		t.Fatalf("ffconcat stdin missing RTSP transport: %s", stdin)
	}
}
