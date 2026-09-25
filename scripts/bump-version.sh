#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VERSION_FILE="$ROOT_DIR/VERSION"

if [ ! -f "$VERSION_FILE" ]; then
    echo "VERSION file not found: $VERSION_FILE" >&2
    exit 1
fi

current=$(tr -d '[:space:]' < "$VERSION_FILE")

valid_version()
{
    printf '%s\n' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'
}

if ! valid_version "$current"; then
    echo "Invalid current VERSION: $current" >&2
    exit 1
fi

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [MAJOR.MINOR.PATCH]" >&2
    exit 1
fi

if [ "$#" -eq 1 ]; then
    next=$1

    if ! valid_version "$next"; then
        echo "Invalid target version: $next" >&2
        exit 1
    fi
else
    old_ifs=$IFS
    IFS=.
    set -- $current
    IFS=$old_ifs

    if [ "$#" -ne 3 ]; then
        echo "Invalid current VERSION: $current" >&2
        exit 1
    fi

    major=$1
    minor=$2
    patch=$3
    next="$major.$minor.$((patch + 1))"
fi

if [ "$next" = "$current" ]; then
    echo "Version is already $current" >&2
    exit 1
fi

printf '%s\n' "$next" > "$VERSION_FILE"
printf '%s -> %s\n' "$current" "$next"
