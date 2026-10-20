#!/usr/bin/env bash
# Private keys are supplied by Actions secrets, never by arguments or Git.
set +x
set -euo pipefail
umask 077

if [[ -z "${HOME_AI_UPDATE_SIGNING_KEY:-}" ]]; then
  echo 'Release blocked: HOME_AI_UPDATE_SIGNING_KEY is not configured. No unsigned release will be published.' >&2
  exit 2
fi
MODE=${1:-}
if [[ "$MODE" != '--check-key' && "$MODE" != '--sign' ]]; then
  echo 'Usage: sign-release-assets.sh --check-key | --sign DIST VERSION' >&2
  exit 2
fi
TMP=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/home-ai-sign.XXXXXXXX")
trap 'rm -rf -- "$TMP"' EXIT
printf '%s\n' "$HOME_AI_UPDATE_SIGNING_KEY" > "$TMP/private.pem"
unset HOME_AI_UPDATE_SIGNING_KEY
openssl pkey -passin pass: -in "$TMP/private.pem" -pubout -out "$TMP/public.pem" 2>/dev/null || {
  echo 'Release blocked: signing key is not a readable unencrypted private PEM.' >&2; exit 2;
}
openssl pkey -pubin -in "$TMP/public.pem" -outform DER -out "$TMP/public.der"
python3 - "$TMP/public.der" <<'PY'
import pathlib, sys
raw = pathlib.Path(sys.argv[1]).read_bytes()
if len(raw) != 44 or raw[:12] != bytes.fromhex('302a300506032b6570032100'):
    raise SystemExit('Release blocked: an Ed25519 key is required.')
PY
printf '%s\n' 'Home-AI-Core release signing self-test' > "$TMP/challenge"
openssl pkeyutl -sign -rawin -inkey "$TMP/private.pem" -in "$TMP/challenge" -out "$TMP/signature"
openssl pkeyutl -verify -rawin -pubin -inkey "$TMP/public.pem" -in "$TMP/challenge" -sigfile "$TMP/signature" >/dev/null
if [[ "$MODE" == '--check-key' ]]; then
  echo 'Ed25519 signing self-test passed (no release created).'
  exit 0
fi
[[ $# == 3 ]] || { echo 'DIST and VERSION are required' >&2; exit 2; }
DIST=$(cd -- "$2" && pwd)
VERSION=$3
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]] || { echo 'Invalid release version' >&2; exit 2; }
[[ "${GITHUB_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || { echo 'Exact source commit is required' >&2; exit 2; }
FILE_VERSION=${VERSION//-/.}
files=(
  "home-ai-core-update_${VERSION}_amd64.tar.gz"
  "home-ai-core-update_${VERSION}_arm64.tar.gz"
  "home-ai-core_${FILE_VERSION}_amd64.deb"
  "home-ai-core_${FILE_VERSION}_arm64.deb"
  "home-ai-windows-client_${VERSION}_amd64.exe"
  "HOME-AI-Setup_${VERSION}_x64.exe"
)
# Check the complete set before generating any publication metadata.
for file in "${files[@]}"; do
  [[ -f "$DIST/$file" && ! -L "$DIST/$file" && -s "$DIST/$file" ]] || { echo "Missing/unsafe release artifact: $file" >&2; exit 2; }
done
for file in "$DIST"/*; do
  [[ -f "$file" && ! -L "$file" ]] || { echo 'Unsafe entry in release directory' >&2; exit 2; }
  name=$(basename -- "$file")
  allowed=0
  for wanted in "${files[@]}"; do
    [[ "$name" == "$wanted" || "$name" == "$wanted.sha256" || "$name" == "$wanted.sha256.sig" ]] && allowed=1
  done
  [[ $allowed == 1 ]] || { echo "Unexpected release asset: $name" >&2; exit 2; }
done
cd -- "$DIST"
cp -- "$TMP/public.pem" home-ai-core-update-trusted.pub
python3 - "$VERSION" "$GITHUB_SHA" "$TMP/public.der" <<'PY'
import hashlib, json, pathlib, sys
version, sha, public = sys.argv[1:]
record = {'version': version, 'source_commit': sha,
          'signing_public_key_sha256_spki': hashlib.sha256(pathlib.Path(public).read_bytes()).hexdigest(),
          'helper_protocol': 8, 'default_endpoint': 'https://SERVER-IP:82/',
          'migration': {'from_172_requires_trusted_debian_installer': True,
                        'do_not_bootstrap_using_old_web_updater': True,
                        'guide': f'https://github.com/DeadSoulf/home-ai-core/blob/{sha}/docs/SECURITY_PORT82_MIGRATION.md'}}
pathlib.Path('release-metadata.json').write_text(json.dumps(record, indent=2)+'\n', encoding='utf-8')
PY
sign() {
  openssl pkeyutl -sign -rawin -inkey "$TMP/private.pem" -in "$1" -out "$TMP/signature"
  openssl pkeyutl -verify -rawin -pubin -inkey "$TMP/public.pem" -in "$1" -sigfile "$TMP/signature" >/dev/null
  base64 -w 0 "$TMP/signature" > "$1.sig"
  printf '\n' >> "$1.sig"
}
for file in "${files[@]}"; do
  sha256sum -- "$file" > "$file.sha256"
  sign "$file.sha256"
done
# Also bind the commit/transition description and exported public key to the release.
sha256sum -- "${files[@]}" release-metadata.json home-ai-core-update-trusted.pub > SHA256SUMS
sign SHA256SUMS
sha256sum --check --strict SHA256SUMS
printf 'Signed and verified %d release artifacts. Public-key SPKI SHA-256: ' "${#files[@]}"
sha256sum "$TMP/public.der" | cut -d ' ' -f1
