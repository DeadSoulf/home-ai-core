# Signed release preparation and the 172 to 173 transition

## Status and required administrator input

Source changes and passing CI do not create an installable release. Release
0.1.173-dev was previously blocked because HOME_AI_UPDATE_SIGNING_KEY was absent.
Automatic provisioning of that real secret was not completed. No production
key has been generated or rotated by this preparation, and no server trust
anchor has been changed. Test keys in signing regressions are disposable and
must never be used for releases.

The repository administrator must register the intended Ed25519 private PEM
in Settings > Secrets and variables > Actions > New repository secret, named
HOME_AI_UPDATE_SIGNING_KEY. Use the administrator's trusted key-management
process, retain a protected recovery copy, and separately record the matching
public-key fingerprint. Do not put private material in Git, issue comments,
workflow inputs, command arguments or chat. Existing server trust keys must be
checked before migration, not overwritten silently.

Fingerprint convention (public key only):

```sh
openssl pkey -pubin -in verified-update-trusted.pub -outform DER | sha256sum
```

## Workflow behaviour

`Publish update bundles` now validates the current main commit and the signing
key before starting build jobs. It rejects absent, invalid, encrypted-without-
password or non-Ed25519 keys and tests signing/verification without printing
private material. Manual runs on non-main refs are not release-authorized.

A VERSION push or a manual run defaults to a **draft**. For a fresh version,
a manual workflow run with `publish=true` publishes only after uploaded files
have been downloaded again and verified byte-for-byte, by signed SHA256SUMS
and cryptographic signature. Every run is serialized. Existing drafts and
published releases are never overwritten or clobbered. Inspect/publish an
already verified draft rather than rerunning against it, or use a new version
for new source changes. A failed upload/verification can leave a draft, never
a partially uploaded public release.

Run the updated workflow from the latest main, not the old failed run: the
latter predates the Web UI corrections. CI must be green on that exact source
commit before the administrator authorizes release publication.

## Signed outputs

The complete set must contain both update archives (amd64/arm64), both Debian
installers (amd64/arm64) and the Windows amd64 client. Each artifact receives a
canonical basename-only `.sha256` and Base64 Ed25519 `.sha256.sig`. Missing,
empty, symlinked or unexpected artifacts stop signing.

The public file `home-ai-core-update-trusted.pub` and `release-metadata.json`
are included in signed `SHA256SUMS`. Metadata records the exact source commit,
public-key SPKI SHA-256, helper protocol 8, HTTPS port 82 and the one-time
installer migration requirement. Private material is confined to a temporary
private directory in the runner and removed on exit.

These are detached release signatures, not Windows Authenticode signatures.
Do not trust a public key merely because it was downloaded alongside a binary:
compare its fingerprint with the independent administrator record first.

After that trust check, verify before using any installer:

```sh
base64 --decode SHA256SUMS.sig > release-signature.bin
openssl pkeyutl -verify -rawin -pubin \
  -inkey ./verified-update-trusted.pub \
  -in SHA256SUMS -sigfile release-signature.bin
sha256sum --check --strict SHA256SUMS
```

For a single package, verify its own `.sha256.sig` over its `.sha256` file,
then check that checksum against the package. A checksum without a trusted
signature is not proof of publisher identity.

## Installation is separate

For an installed 0.1.172-dev, follow [the migration guide](SECURITY_PORT82_MIGRATION.md)
from trusted console/SSH with backups. Do not use the old Web updater for this
trust-boundary transition. The Debian hostname/IP, architecture, working
administrative access and existing public trust key must be established before
remote installation. A Windows device named `server` is not proof that it is
the Debian Core host. No target host was restarted or modified by release
preparation, and HTTPS:82 only becomes active on that host after migration.

## Automated tests

`python3 scripts/tests/test_release_signing.py` uses OpenSSL and nine isolated
regressions: missing/invalid/wrong-type key, successful key self-test without
publication, missing artifact, symlink, unexpected artifact, missing source
commit, and full verification plus tamper rejection. It does not use or modify
repository secrets and does not create a GitHub release.
