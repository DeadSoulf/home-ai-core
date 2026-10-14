# Verified publication: Home-AI-Core 0.1.173-dev

Published: **2026-10-14T06:40:28Z**. Status: **public prerelease**, not draft and
not a stable-release certification. No installed server was changed.

- [Release](https://github.com/DeadSoulf/home-ai-core/releases/tag/v0.1.173-dev)
- Source commit: `399c49c78dfb5b728e5dc8819041038108bb381d` (includes PR #180 Web UI and #181 signing corrections).
- [Successful release workflow 38523278930](https://github.com/DeadSoulf/home-ai-core/actions/runs/38523278930): key preflight,
  two Debian builds, two update bundles, Windows client and release verification.
- Existing CI on this exact source: [core-ci 38520310088](https://github.com/DeadSoulf/home-ai-core/actions/runs/38520310088)
  and [browser regressions 38520310100](https://github.com/DeadSoulf/home-ai-core/actions/runs/38520310100), successful.

## Independent verification before publication

Downloaded all **19** draft assets to the owner's Asus computer.
Verified the five artifact checksum signatures and aggregate SHA256SUMS signature
(**6 Ed25519 verifications**) against the owner's separately generated,
locally stored **public** key, not just the key accompanying the download.

Public-key SPKI DER SHA-256:

```text
5ade10e4c08bb5422ee7b14bbf8c4beed85eef1f0b9014f13c9a1d217c62cf21
```

The release public key matches this local key. Signed metadata binds version,
exact source commit, helper protocol 8, HTTPS port 82 and the one-time Debian
migration requirement. Checked file sizes and GitHub asset digests too. No
private key or secret value was read, rotated or copied by these verification
tools; the existing release workflow used the secret configured by the owner.

After publication, an **unauthenticated** GitHub API request confirmed the
release in the public release list; both architectures' public checksum and
signature URLs returned the same verified bytes. This verifies the distribution
source, not the behaviour of the installed server's Web UI.

## Artifact checksums

| Artifact | SHA-256 |
|---|---|
| `home-ai-core-update_0.1.173-dev_amd64.tar.gz` | `c0d97f729b123a99c159cfa553fd8cde2de45ad9bf9cc65a7befef738c5c84c1` |
| `home-ai-core-update_0.1.173-dev_arm64.tar.gz` | `32c3eeaae52cd14950863d77e939544738d3ff8e82bd4e20a5e50c8899568ce0` |
| `home-ai-core_0.1.173.dev_amd64.deb` | `d60cc494011071bfdfa685e5f1a0dfad30c33bc5ec78f01007b064f874a7244a` |
| `home-ai-core_0.1.173.dev_arm64.deb` | `4b8800ce50630c28c4931cd02e4eca06c19a365400a99f0fe7150ffd1ed68d1b` |
| `home-ai-windows-client_0.1.173-dev_amd64.exe` | `bbb873067e2ec667312f6143b037b28f7cf373c8490d95886cfaf930688a1665` |
| `release-metadata.json` | `ad31b342967a7274e8b80b0cdc47e5ad3536d86dea4c1865d87af8b8a07cf834` |
| `home-ai-core-update-trusted.pub` | `59f6ba919a8c60c86affc83bd42e7ebf8694b75fbcdd284aa4472be760803876` |

Local verified downloads: `D:\AI\releases\home-ai-core\0.1.173-dev-399c49c\assets`.
Local verification report: the adjacent `verification.json` (public metadata only).

## Installation remains pending

Follow [SECURITY_PORT82_MIGRATION.md](SECURITY_PORT82_MIGRATION.md) from a trusted
Debian console/SSH, with backups and a verified public trust key. Do not bootstrap
protocol 8/systemd/TLS/capability migration through the old 172 Web updater.
No IP/hostname or working administrative SSH login for the actual Debian Core
host has been provided. No service was restarted, no production port switched,
and no data migration was attempted. Different-image module upgrade/rollback
remains in issue #177. Later source fixes require a new version; do not replace
these already published binaries.
