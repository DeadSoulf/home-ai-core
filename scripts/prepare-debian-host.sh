#!/usr/bin/env bash
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
    echo "Run with sudo: sudo bash scripts/prepare-debian-host.sh" >&2
    exit 1
fi

if [[ ! -r /etc/os-release ]]; then
    echo "Cannot read /etc/os-release." >&2
    exit 1
fi

. /etc/os-release

if [[ "${ID:-}" != "debian" || "${VERSION_ID:-}" != "13" ]]; then
    echo "This preparation script supports Debian 13 only. Found: ${PRETTY_NAME:-unknown}" >&2
    exit 1
fi

if [[ "$(uname -m)" != "x86_64" ]]; then
    echo "Home AI Core physical-server candidate currently targets x86_64." >&2
    exit 1
fi

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    cmake \
    ninja-build \
    g++ \
    pkg-config \
    libssl-dev \
    libsqlite3-dev \
    openssl \
    curl \
    python3 \
    sudo \
    ffmpeg \
    iproute2 \
    util-linux \
    e2fsprogs

echo
echo "Base Home AI Core dependencies are installed."
echo "Hypervisor packages are intentionally separate."
echo "After installing home-ai-core.service, run:"
echo "  sudo bash scripts/setup-hypervisor.sh install"
