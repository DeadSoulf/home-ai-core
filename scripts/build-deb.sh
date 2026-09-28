#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ARCH=${1:-$(dpkg --print-architecture 2>/dev/null || echo amd64)}
VERSION=$(tr -d '\r\n' < "$ROOT/VERSION")
PKG_VERSION=$(printf '%s' "$VERSION" | sed 's/-/~/g')
OUT_DIR="$ROOT/build/packages"
STAGE="$ROOT/build/deb-$ARCH"

case "$ARCH" in
  amd64) GOARCH=amd64 ;;
  arm64) GOARCH=arm64 ;;
  *)
    echo "unsupported Debian architecture: $ARCH" >&2
    exit 2
    ;;
esac

command -v go >/dev/null 2>&1 || { echo "go is required" >&2; exit 2; }
command -v npm >/dev/null 2>&1 || { echo "npm is required" >&2; exit 2; }
command -v dpkg-deb >/dev/null 2>&1 || { echo "dpkg-deb is required" >&2; exit 2; }

rm -rf "$STAGE"
mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/bin" "$STAGE/usr/share/home-ai-core/web" \
  "$STAGE/lib/systemd/system" "$STAGE/usr/lib/sysusers.d" "$STAGE/etc/home-ai-core" "$OUT_DIR"

if [ "${HOME_AI_SKIP_WEB_BUILD:-0}" != "1" ]; then
  cd "$ROOT/web"
  npm ci --ignore-scripts --no-audit --no-fund
  npm run build
fi

test -f "$ROOT/web/dist/index.html"
cp -a "$ROOT/web/dist/." "$STAGE/usr/share/home-ai-core/web/"

cd "$ROOT"
GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags "-s -w -X github.com/DeadSoulf/home-ai-core/internal/version.Version=$VERSION" \
  -o "$STAGE/usr/bin/home-ai-core" \
  ./cmd/home-ai-core

install -m 0644 "$ROOT/packaging/debian/home-ai-core.service" "$STAGE/lib/systemd/system/home-ai-core.service"
install -m 0644 "$ROOT/packaging/debian/home-ai-core.sysusers" "$STAGE/usr/lib/sysusers.d/home-ai-core.conf"
install -m 0640 "$ROOT/packaging/debian/home-ai-core.env.example" "$STAGE/etc/home-ai-core/home-ai-core.env"

cat >"$STAGE/DEBIAN/control" <<EOF
Package: home-ai-core
Version: $PKG_VERSION
Section: admin
Priority: optional
Architecture: $ARCH
Maintainer: Home-AI-Core
Depends: systemd, pci.ids
Description: Home-AI-Core private home infrastructure control plane
 Home-AI-Core provides a modular Debian-based control plane for private
 home-server infrastructure, modules, jobs, events and the Web UI.
EOF

cat >"$STAGE/DEBIAN/conffiles" <<'EOF'
/etc/home-ai-core/home-ai-core.env
EOF

cat >"$STAGE/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -eu
if command -v systemd-sysusers >/dev/null 2>&1; then
  systemd-sysusers /usr/lib/sysusers.d/home-ai-core.conf
elif ! getent passwd home-ai-core >/dev/null 2>&1; then
  useradd --system --home-dir /var/lib/home-ai-core --shell /usr/sbin/nologin --user-group home-ai-core
fi
install -d -o home-ai-core -g home-ai-core -m 0700 /var/lib/home-ai-core
install -d -o root -g home-ai-core -m 0750 /etc/home-ai-core
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
  systemctl enable home-ai-core.service >/dev/null 2>&1 || true
  if [ "$1" = "configure" ]; then
    systemctl restart home-ai-core.service || true
  fi
fi
EOF
chmod 0755 "$STAGE/DEBIAN/postinst"

cat >"$STAGE/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -eu
if [ "$1" = "remove" ] && command -v systemctl >/dev/null 2>&1; then
  systemctl stop home-ai-core.service || true
  systemctl disable home-ai-core.service >/dev/null 2>&1 || true
fi
EOF
chmod 0755 "$STAGE/DEBIAN/prerm"

cat >"$STAGE/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -eu
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload || true
fi
# /var/lib/home-ai-core is intentionally preserved.
EOF
chmod 0755 "$STAGE/DEBIAN/postrm"

chmod 0755 "$STAGE/usr/bin/home-ai-core"
find "$STAGE/usr/share/home-ai-core/web" -type d -exec chmod 0755 {} +
find "$STAGE/usr/share/home-ai-core/web" -type f -exec chmod 0644 {} +

PACKAGE="$OUT_DIR/home-ai-core_${PKG_VERSION}_${ARCH}.deb"
dpkg-deb --root-owner-group --build "$STAGE" "$PACKAGE"
echo "$PACKAGE"
