#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root: sudo $0 <package.deb>" >&2
  exit 2
fi

PACKAGE=${1:-}
if [ -z "$PACKAGE" ] || [ ! -f "$PACKAGE" ]; then
  echo "usage: sudo $0 <home-ai-core_*.deb>" >&2
  exit 2
fi

case "$(dpkg --print-architecture)" in
  amd64|arm64) ;;
  *)
    echo "unsupported host architecture: $(dpkg --print-architecture)" >&2
    exit 2
    ;;
esac

if [ -r /etc/os-release ]; then
  . /etc/os-release
  if [ "${ID:-}" != "debian" ] || [ "${VERSION_ID:-}" != "13" ]; then
    echo "Home-AI-Core currently supports Debian 13; detected ${PRETTY_NAME:-unknown}" >&2
    exit 2
  fi
fi

apt-get update
apt-get install -y ca-certificates curl python3

dpkg -i "$PACKAGE" || {
  apt-get -f install -y
  dpkg -i "$PACKAGE"
}

systemctl daemon-reload
systemctl enable --now home-ai-core.service
systemctl --no-pager --full status home-ai-core.service || true

echo
echo "Home-AI-Core installed."
echo "Local UI: http://127.0.0.1:8080/"
for address in $(hostname -I 2>/dev/null || true); do
  case "$address" in
    *:*) ;;
    127.*) ;;
    *) echo "LAN UI:   http://$address:8080/" ;;
  esac
done
