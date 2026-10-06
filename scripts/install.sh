#!/bin/sh
set -eu

GO_VERSION="${HOME_AI_GO_VERSION:-1.27.1}"
NODE_VERSION="${HOME_AI_NODE_VERSION:-24.21.0}"
REF="${HOME_AI_REF:-main}"
SOURCE_URL="${HOME_AI_SOURCE_URL:-https://github.com/DeadSoulf/home-ai-core.git}"

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root, for example:" >&2
  echo "  curl -fsSL https://raw.githubusercontent.com/DeadSoulf/home-ai-core/main/scripts/install.sh | sudo sh" >&2
  exit 2
fi

if [ ! -r /etc/os-release ]; then
  echo "cannot detect operating system" >&2
  exit 2
fi
. /etc/os-release
if [ "${ID:-}" != "debian" ] || [ "${VERSION_ID:-}" != "13" ]; then
  echo "Home-AI-Core supports Debian 13; detected ${PRETTY_NAME:-unknown}" >&2
  exit 2
fi

ARCH=$(dpkg --print-architecture)
case "$ARCH" in
  amd64)
    GOARCH=amd64
    NODEARCH=x64
    ;;
  arm64)
    GOARCH=arm64
    NODEARCH=arm64
    ;;
  *)
    echo "unsupported architecture: $ARCH" >&2
    exit 2
    ;;
esac

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git xz-utils python3

WORKDIR=$(mktemp -d)
cleanup() {
  rm -rf "$WORKDIR"
}
trap cleanup EXIT HUP INT TERM

echo "Installing Go $GO_VERSION..."
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${GOARCH}.tar.gz" -o "$WORKDIR/go.tar.gz"
rm -rf /usr/local/go
tar -C /usr/local -xzf "$WORKDIR/go.tar.gz"
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

echo "Installing Node.js $NODE_VERSION..."
NODE_DIR="/usr/local/lib/nodejs/node-v${NODE_VERSION}-linux-${NODEARCH}"
mkdir -p /usr/local/lib/nodejs
rm -rf "$NODE_DIR"
curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODEARCH}.tar.xz" -o "$WORKDIR/node.tar.xz"
tar -C /usr/local/lib/nodejs -xJf "$WORKDIR/node.tar.xz"
ln -sf "$NODE_DIR/bin/node" /usr/local/bin/node
ln -sf "$NODE_DIR/bin/npm" /usr/local/bin/npm
ln -sf "$NODE_DIR/bin/npx" /usr/local/bin/npx

export PATH="/usr/local/go/bin:/usr/local/bin:$PATH"

echo "Downloading Home-AI-Core..."
git clone --depth 1 --branch "$REF" "$SOURCE_URL" "$WORKDIR/home-ai-core"
cd "$WORKDIR/home-ai-core"

echo "Building Home-AI-Core for $ARCH..."
sh ./scripts/build-deb.sh "$ARCH"
PACKAGE=$(find build/packages -maxdepth 1 -type f -name "home-ai-core_*_${ARCH}.deb" -print -quit)
if [ -z "$PACKAGE" ]; then
  echo "package build failed: .deb not found" >&2
  exit 1
fi

echo "Installing Home-AI-Core..."
apt-get install -y "./$PACKAGE"
systemctl daemon-reload
systemctl enable --now home-ai-core.service home-ai-core-updater.service

echo "Checking Home-AI-Core..."
i=0
until curl -fsS http://127.0.0.1:8080/health >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 30 ]; then
    echo "Home-AI-Core did not become healthy" >&2
    systemctl --no-pager --full status home-ai-core.service || true
    exit 1
  fi
  sleep 1
done

echo
echo "Home-AI-Core installed successfully."
echo "Open the Web UI from another computer on the same local network:"
found=0
for address in $(hostname -I 2>/dev/null || true); do
  case "$address" in
    *:*) ;;
    127.*) ;;
    *)
      echo "  http://$address:8080/"
      found=1
      ;;
  esac
done
if [ "$found" -eq 0 ]; then
  echo "  http://<SERVER-IP>:8080/"
fi
echo
echo "Create the first owner account in the browser."
