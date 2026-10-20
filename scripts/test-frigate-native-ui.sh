#!/usr/bin/env bash
# Isolated full-UI acceptance. Never uses existing module containers or data.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
IMAGE=ghcr.io/blakeblackshear/frigate@sha256:9ebab4801be69ab4ebc7f6ab1d582c859ff7412d457a3fa550056894e8444dad
DOCKER=${FRIGATE_TEST_DOCKER:-docker}
PYTHON=${FRIGATE_BROWSER_PYTHON:-python3}
TEST_BINARY=${FRIGATE_API_TEST_BINARY:-/tmp/home-ai-frigate-api.test}
WORK=$(mktemp -d /tmp/home-ai-frigate-native-XXXXXX)
NAME=home-ai-frigate-native-$(basename "$WORK" | tr '[:upper:]' '[:lower:]')
READY=${FRIGATE_NATIVE_READY_FILE:-$ROOT/build/validation/native-ui-ready.json}
FIXTURE_PID=
CONTAINER=
cleanup() {
  [[ -z "$FIXTURE_PID" ]] || { touch "$READY.stop"; kill "$FIXTURE_PID" 2>/dev/null || true; wait "$FIXTURE_PID" 2>/dev/null || true; }
  [[ -z "$CONTAINER" ]] || "$DOCKER" rm -f "$CONTAINER" >/dev/null 2>&1 || true
  rm -f "$READY" "$READY.stop"
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$WORK/config" "$WORK/media" "$(dirname "$READY")"
rm -f "$READY" "$READY.stop"
"$DOCKER" image inspect "$IMAGE" >/dev/null 2>&1 || "$DOCKER" pull "$IMAGE"
# Color bars are generated locally. No actual camera, recording or user data.
"$DOCKER" run --rm --network none --entrypoint /bin/sh --mount "type=bind,src=$WORK/config,dst=/out" "$IMAGE" -ec '
ff=$(find /usr/lib/ffmpeg -type f -name ffmpeg | head -1)
"$ff" -hide_banner -loglevel error -f lavfi -i testsrc2=size=640x360:rate=10 -t 600 -c:v libx264 -preset ultrafast -pix_fmt yuv420p -g 10 -an /out/synthetic.mp4
'
cat > "$WORK/config/config.yml" <<'YAML'
mqtt:
  enabled: false
go2rtc:
  streams:
    synthetic:
      - "ffmpeg:/config/synthetic.mp4#input=file#video=copy"
cameras:
  synthetic:
    ffmpeg:
      inputs:
        - path: rtsp://127.0.0.1:8554/synthetic
          input_args: preset-rtsp-restream
          roles: [detect, record]
    detect:
      enabled: false
      width: 640
      height: 360
      fps: 5
    record:
      enabled: true
      continuous:
        days: 1
YAML
CONTAINER=$("$DOCKER" run -d --name "$NAME" --label home-ai.native-ui-test=true \
  --cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add SETUID --cap-add SETGID --cap-add DAC_OVERRIDE --cap-add KILL \
  --security-opt no-new-privileges --pids-limit 2048 --shm-size 256m \
  --mount "type=bind,src=$WORK/config,dst=/config" --mount "type=bind,src=$WORK/media,dst=/media/frigate" "$IMAGE")
# No container port is published: only Core's authenticated TLS gateway is used.
for attempt in $(seq 1 180); do
  STATUS=$("$DOCKER" inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$CONTAINER")
  [[ "$STATUS" != healthy ]] || break
  if [[ "$STATUS" == unhealthy ]]; then echo 'Synthetic Frigate container failed health check' >&2; exit 1; fi
  sleep 2
done
[[ "$STATUS" == healthy ]]
IP=$("$DOCKER" inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$CONTAINER")
cd "$ROOT/internal/api"
FRIGATE_NATIVE_BROWSER=1 FRIGATE_NATIVE_CONTAINER_IP="$IP" FRIGATE_NATIVE_READY_FILE="$READY" \
  "$TEST_BINARY" -test.run '^TestFrigateNativeBrowserFixture$' -test.v -test.timeout 17m > "$ROOT/build/validation/native-ui-fixture.log" 2>&1 &
FIXTURE_PID=$!
for attempt in $(seq 1 80); do [[ ! -s "$READY" ]] || break; kill -0 "$FIXTURE_PID"; sleep .25; done
[[ -s "$READY" ]]
echo 'READY: original Frigate, synthetic live stream and real Core TLS/session gateway'
if [[ ${FRIGATE_NATIVE_SERVE_ONLY:-0} == 1 ]]; then
  wait "$FIXTURE_PID"
else
  FRIGATE_NATIVE_READY_FILE="$READY" "$PYTHON" "$ROOT/web/tests/frigate_native_live.py"
  touch "$READY.stop"
  wait "$FIXTURE_PID"
fi
FIXTURE_PID=
echo 'PASS: real official Frigate UI through isolated authenticated Core gateway'
