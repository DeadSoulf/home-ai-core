#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "Run this installer with sudo."
    exit 1
fi

PROJECT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TARGET_USER="${1:-${SUDO_USER:-}}"

if [ -z "$TARGET_USER" ]; then
    echo "Unable to determine the service user."
    echo "Usage: sudo ./scripts/install-storage-helper.sh <user>"
    exit 1
fi

case "$TARGET_USER" in
    *[!A-Za-z0-9_.-]*)
        echo "Invalid user name."
        exit 1
        ;;
esac

if ! id "$TARGET_USER" >/dev/null 2>&1; then
    echo "User does not exist: $TARGET_USER"
    exit 1
fi

HELPER="$PROJECT_DIR/build/home-ai-storage-helper"

if [ ! -x "$HELPER" ]; then
    echo "Build home-ai-storage-helper first."
    exit 1
fi

install -d -m 0755 /usr/local/libexec
install -o root -g root -m 0755     "$HELPER"     /usr/local/libexec/home-ai-storage-helper

install -d -m 0750 /mnt/home-ai
install -d -m 0750 /mnt/home-ai/video
install -d -m 0750 /mnt/home-ai/files
install -d -m 0750 /mnt/home-ai/storage

SUDOERS="/etc/sudoers.d/home-ai-storage-helper"

printf '%s ALL=(root) NOPASSWD: /usr/local/libexec/home-ai-storage-helper *\n'     "$TARGET_USER" > "$SUDOERS"

chmod 0440 "$SUDOERS"

if ! visudo -cf "$SUDOERS"; then
    rm -f "$SUDOERS"
    echo "Invalid sudoers configuration; removed."
    exit 1
fi

echo "Storage helper installed for user: $TARGET_USER"
echo "Path: /usr/local/libexec/home-ai-storage-helper"
