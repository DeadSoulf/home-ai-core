#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=$(cat "$ROOT/VERSION")

mkdir -p "$ROOT/build"
cd "$ROOT"

exec go build \
  -trimpath \
  -ldflags "-s -w -X github.com/DeadSoulf/home-ai-core/internal/version.Version=$VERSION" \
  -o "$ROOT/build/home-ai-core" \
  ./cmd/home-ai-core
