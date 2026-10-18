#!/bin/sh
# Run before building/testing the Windows client. Go links *.syso from the
# package directory automatically; never bundle a sidecar .manifest file.
set -eu
ARCH="${1:-amd64}"
case "$ARCH" in
  amd64|386|arm64) ;;
  *) echo "unsupported Windows client architecture: $ARCH" >&2; exit 2 ;;
esac
ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT/cmd/home-ai-windows-client"
go run github.com/tc-hib/go-winres@v0.3.3 make --arch "$ARCH"
test -s "rsrc_windows_${ARCH}.syso"
