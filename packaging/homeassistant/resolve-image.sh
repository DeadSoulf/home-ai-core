#!/usr/bin/env bash
# Manual, fail-closed Home Assistant image pinning for the staging deployment.
set -euo pipefail
IMAGE_REPO="ghcr.io/home-assistant/home-assistant"
TAG="${1:-stable}"
case "$TAG" in
  stable|[0-9][0-9][0-9][0-9].[0-9]*.[0-9]*) ;;
  *) echo "Unsupported Home Assistant tag" >&2; exit 2 ;;
esac
command -v docker >/dev/null || { echo "Docker not found" >&2; exit 1; }
case "$(uname -m)" in
  x86_64|aarch64) ;;
  *) echo "Only amd64 and arm64 are supported" >&2; exit 1 ;;
esac
if [ -e /run/dbus/system_bus_socket ]; then
  echo "DBus socket available"
else
  echo "DBus socket unavailable: Bluetooth and other integrations may not work" >&2
fi
if command -v ss >/dev/null && ss -ltn '( sport = :8123 )' | tail -n +2 | grep -q .; then
  echo "Port 8123 is in use; refusing deployment" >&2
  exit 1
fi
docker pull "$IMAGE_REPO:$TAG" >&2
# RepoDigest obtained from actual downloaded image; never trust an unverified tag.
digest="$(docker image inspect "$IMAGE_REPO:$TAG" --format '{{range .RepoDigests}}{{println .}}{{end}}' | grep -E '^ghcr.io/home-assistant/home-assistant@sha256:[a-f0-9]{64}$' | head -n 1)"
test -n "$digest" || { echo "No verified registry digest" >&2; exit 1; }
printf 'HOME_ASSISTANT_IMAGE=%s\n' "$digest"
