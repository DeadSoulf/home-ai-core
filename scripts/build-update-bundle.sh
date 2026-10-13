#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ARCH=${1:-$(dpkg --print-architecture 2>/dev/null || echo amd64)}
VERSION=$(tr -d '\r\n' < "$ROOT/VERSION")
OUT_DIR="$ROOT/build/updates"
STAGE="$ROOT/build/update-$ARCH"
BUNDLE="$OUT_DIR/home-ai-core-update_${VERSION}_${ARCH}.tar.gz"

case "$ARCH" in
  amd64) GOARCH=amd64 ;;
  arm64) GOARCH=arm64 ;;
  *)
    echo "unsupported architecture: $ARCH" >&2
    exit 2
    ;;
esac

command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 2; }
command -v npm >/dev/null 2>&1 || { echo "npm is required" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 2; }
command -v tar >/dev/null 2>&1 || { echo "tar is required" >&2; exit 2; }
command -v gzip >/dev/null 2>&1 || { echo "gzip is required" >&2; exit 2; }
command -v sha256sum >/dev/null 2>&1 || { echo "sha256sum is required" >&2; exit 2; }

rm -rf "$STAGE"
mkdir -p "$STAGE/bin" "$STAGE/helper" "$STAGE/web" "$OUT_DIR"

if [ "${HOME_AI_SKIP_WEB_BUILD:-0}" != "1" ]; then
  cd "$ROOT/web"
  npm ci --ignore-scripts --no-audit --no-fund
  npm run build
fi

test -f "$ROOT/web/dist/index.html"
cp -a "$ROOT/web/dist/." "$STAGE/web/"

cd "$ROOT"
GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w -X github.com/DeadSoulf/home-ai-core/internal/version.Version=$VERSION" \
  -o "$STAGE/bin/home-ai-core" \
  ./cmd/home-ai-core

GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w -X github.com/DeadSoulf/home-ai-core/internal/updaterhelper.HelperVersion=$VERSION" \
  -o "$STAGE/helper/home-ai-core-updater" \
  ./cmd/home-ai-core-updater

chmod 0755 "$STAGE/bin/home-ai-core" "$STAGE/helper/home-ai-core-updater"
find "$STAGE/web" -type d -exec chmod 0755 {} +
find "$STAGE/web" -type f -exec chmod 0644 {} +

STAGE="$STAGE" VERSION="$VERSION" ARCH="$ARCH" python3 - <<'PY'
import hashlib
import json
import os
from pathlib import Path

stage = Path(os.environ["STAGE"])
version = os.environ["VERSION"]
arch = os.environ["ARCH"]

files = []
for path in sorted(p for p in stage.rglob("*") if p.is_file()):
    rel = path.relative_to(stage).as_posix()
    if rel == "manifest.json":
        continue
    data = path.read_bytes()
    files.append({
        "path": rel,
        "sha256": hashlib.sha256(data).hexdigest(),
        "size_bytes": len(data),
    })

manifest = {
    "schema_version": 1,
    "product": "home-ai-core",
    "version": version,
    "architecture": arch,
    "helper_protocol": 7,
    "helper_version": version,
    "files": files,
}
(stage / "manifest.json").write_text(
    json.dumps(manifest, indent=2, sort_keys=True) + "\n",
    encoding="utf-8",
)
PY

rm -f "$BUNDLE" "$BUNDLE.sha256"
(
  cd "$STAGE"
  {
    printf '%s\0' manifest.json bin/home-ai-core helper/home-ai-core-updater
    find web -type f -print0
  } | sort -z | tar --null --no-recursion --sort=name --mtime='@0' --owner=0 --group=0 --numeric-owner -cf - -T -
) | gzip -n > "$BUNDLE"

(
  cd "$OUT_DIR"
  sha256sum "$(basename "$BUNDLE")" > "$(basename "$BUNDLE").sha256"
)

echo "$BUNDLE"
echo "$BUNDLE.sha256"
