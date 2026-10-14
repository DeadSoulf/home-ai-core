# Verified CSRF/console hotfix publication: 0.1.174-dev

Published: **2026-10-14T09:13:32Z**, public **prerelease** (not a stable-release certification).
[Release](https://github.com/DeadSoulf/home-ai-core/releases/tag/v0.1.174-dev); [implementation PR #183](https://github.com/DeadSoulf/home-ai-core/pull/183).
Exact source: `e3647ee1666e3b19c72e9906335de9b1d9c2d13b`.

## Verification

- Post-merge [core-ci 38539935877](https://github.com/DeadSoulf/home-ai-core/actions/runs/38539935877): successful, including 66 Web unit tests, Go tests/vet, HTTPS smoke and builds.
- Post-merge [browser checks 38539935838](https://github.com/DeadSoulf/home-ai-core/actions/runs/38539935838): successful, 16 existing and 5 new CSRF groups in real Chromium, synthetic API/session/WebSocket only.
- [Release 38539935962](https://github.com/DeadSoulf/home-ai-core/actions/runs/38539935962): all builds, signing and uploaded-byte verification successful.
- Downloaded and independently verified **19 assets and 6 Ed25519 signatures** on Asus against the owner's separate local public key. Asset sizes and GitHub SHA-256 digests matched; signed metadata binds source/version/protocol/endpoint.
- Anonymous public release endpoint confirmed after publication. The old 173 release is untouched.

Pinned public-key SPKI SHA-256 (unchanged):

```text
5ade10e4c08bb5422ee7b14bbf8c4beed85eef1f0b9014f13c9a1d217c62cf21
```

## Artifact checksums

| File | SHA-256 |
|---|---|
| `home-ai-core-update_0.1.174-dev_amd64.tar.gz` | `04b51d9a3128d8eff19bb15e29f30d5ffff7283223b797ad639a2d03b351c82a` |
| `home-ai-core-update_0.1.174-dev_arm64.tar.gz` | `a1d4b0ee3a57caa30d67d2438ebaa4249d058919e7f8495d54f125e991087e88` |
| `home-ai-core_0.1.174.dev_amd64.deb` | `78836e19ec61535a07a0d980abc07821edc1c97fd65089aaded82279796bc56f` |
| `home-ai-core_0.1.174.dev_arm64.deb` | `881918e13768d09599a460447de8470482011fdc80486329cdbf3f152dff8eb5` |
| `home-ai-windows-client_0.1.174-dev_amd64.exe` | `352580451aa689210a1dfc739ad74ed1d0edea2dd2df755cd778b41d7b41a8e9` |
| `release-metadata.json` | `baa6a86783dc0811a84c911cfe470b976b1f41d7ceea07aea779da5aca31c04d` |
| `home-ai-core-update-trusted.pub` | `59f6ba919a8c60c86affc83bd42e7ebf8694b75fbcdd284aa4472be760803876` |

## Deployment boundary

The user's reported successful start of 173 is not a direct verification of
174. No live Debian service, database, browser session or private signing key
was accessed by the hotfix work. Deploy matching Core and Web assets together;
then reload and sign in to the existing account when prompted. The server's
CSRF validation remains mandatory, no denied mutation is replayed automatically,
and old server sessions are not falsely described as revoked.

For an already migrated 173 installation, protocol 8, TLS:82 and the public
signing key stay unchanged. When its old Web session cannot authorize an
update, use a verified Debian package via independent console/SSH and take a
fresh consistent backup first. Never use the embedded console to stop its own
Core service. For installations still on 172, the one-time Debian migration
in SECURITY_PORT82_MIGRATION.md remains mandatory.

See CSRF_SESSION_RECOVERY.md for implementation and regression-test limits.
Local verified artifacts are in
`D:\AI\releases\home-ai-core\0.1.174-dev-e3647ee\assets`.
