#!/bin/sh
# Compile the user-mode NSIS wizard from a versioned Windows client EXE.
# Requires NSIS 3 (makensis); Wine or Windows is not needed at build time.
set -eu

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 path/to/versioned-client.exe path/to/HOMEAI-Setup.exe" >&2
  exit 2
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CLIENT="$1"
OUTPUT="$2"
VERSION="$(tr -d '\r\n' < "$ROOT/VERSION")"

case "$VERSION" in
  *[!a-zA-Z0-9.-]*|'') echo 'Invalid VERSION for installer metadata' >&2; exit 2 ;;
esac

test -f "$CLIENT" && test -s "$CLIENT"
command -v makensis >/dev/null 2>&1 || {
  echo 'NSIS makensis is missing: install the nsis package first' >&2
  exit 2
}
CLIENT="$(realpath "$CLIENT")"
mkdir -p "$(dirname "$OUTPUT")"
OUTPUT="$(realpath "$(dirname "$OUTPUT")")/$(basename "$OUTPUT")"

makensis -V3 -WX \
  -DVERSION="$VERSION" \
  -DCLIENT_EXE="$CLIENT" \
  -DOUTFILE="$OUTPUT" \
  "$ROOT/packaging/windows/home-ai-client.nsi"
test -s "$OUTPUT"
echo "Built HOME AI Windows setup: $OUTPUT"
