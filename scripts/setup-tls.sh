#!/usr/bin/env bash
set -euo pipefail

name="${1:-home-ai-core.local}"
target_dir="${2:-runtime/tls}"

if [[ ! "$name" =~ ^[A-Za-z0-9.-]+$ ]]; then
    echo "Invalid DNS name or IPv4 address: $name" >&2
    exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
    echo "openssl is required. Install it with: sudo apt install openssl" >&2
    exit 1
fi

mkdir -p "$target_dir"
chmod 700 "$target_dir"

certificate="$target_dir/server.crt"
private_key="$target_dir/server.key"

if [[ -e "$certificate" || -e "$private_key" ]]; then
    echo "TLS certificate or key already exists in $target_dir; refusing to overwrite." >&2
    exit 1
fi

san="DNS:$name"
if [[ "$name" =~ ^[0-9]+([.][0-9]+){3}$ ]]; then
    san="IP:$name"
fi

umask 077
openssl req \
    -x509 \
    -newkey rsa:3072 \
    -sha256 \
    -nodes \
    -days 825 \
    -keyout "$private_key" \
    -out "$certificate" \
    -subj "/CN=$name" \
    -addext "subjectAltName=$san"

chmod 600 "$private_key"
chmod 644 "$certificate"

cat <<EOF

TLS files created:
  certificate: $certificate
  private key: $private_key

Enable HTTPS in runtime/home-ai.conf:

web.tls_enabled=true
web.tls_certificate=$certificate
web.tls_private_key=$private_key

Then restart Home AI Core.

This certificate is self-signed. Browsers will warn until the certificate or its issuing
CA is trusted. For remote/production access, use a certificate issued by a trusted internal
or public CA.
EOF
