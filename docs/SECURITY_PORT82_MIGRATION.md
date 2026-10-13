# Core 0.1.173-dev: security boundary and HTTPS port 82

## What changes

The default endpoint is **`https://SERVER-IP:82/`**, not HTTP on port 8080.
The network-facing process still runs as `home-ai-core`, not root. Its systemd
unit grants only `CAP_NET_BIND_SERVICE` to bind the low port, and retains
`NoNewPrivileges=yes` and the existing filesystem/device restrictions.

This source change integrates the earlier IPC fixes and addresses the Codex
review findings: TLS by default, out-of-band first-owner proof, independent
signature verification inside the privileged updater, recoverable same-image
module installation, and stop/remove during Docker network degradation.
It is not a claim that every possible defect has been found or that CI replaces
real-server acceptance.

## Existing installations: one-time installer migration

Do **not** use an old updater helper to bootstrap the new trust boundary.
This transition changes helper protocol 7 to 8, systemd capability settings,
configuration-directory ownership and trusted update storage. Apply the reviewed
Debian package from a local server console or trusted SSH session:

```sh
sudo apt install ./home-ai-core_0.1.173.dev_amd64.deb
```

The package filename uses `.dev`; the Core version remains `0.1.173-dev`.
Use the arm64 package on arm64. Back up the existing database, configuration and
module data first. Package configuration migrates the old `HOME_AI_LISTEN=*:8080`
setting to the same host on port 82, saving the previous environment file as
`/etc/home-ai-core/home-ai-core.env.before-port82`. Custom ports are retained.
Stop/restart is part of installation; arrange console access and do not perform
the migration over a connection whose loss would prevent recovery.

The normal Web bundle updater does not replace systemd unit files, which is why
this initial package migration is required. No production server was changed by
the development work. Publishing a new package is a separate release step.

## TLS identity and client trust

On first start, Core creates a persistent ECDSA TLS certificate and private key
under `/var/lib/home-ai-core/tls/`, with private permissions. The certificate
includes localhost and the network addresses visible at generation. It expires
in one year; provision a renewed pair before expiry. An incomplete, mismatched or
expired pair fails closed rather than falling back to HTTP.

**A self-signed certificate is not automatically trusted.** Verify its SHA-256
fingerprint directly on the server before importing the *public certificate*
into each client's trust store, or provision a certificate from your own CA.
Do not distribute `server.key`, and do not disable certificate verification.

```sh
sudo openssl x509 -in /var/lib/home-ai-core/tls/server.crt \
  -noout -fingerprint -sha256
```

Administrator-provisioned certificate/key paths can be set with
`HOME_AI_TLS_CERT` and `HOME_AI_TLS_KEY`. Both are required together. Ensure they
are readable by the service within its sandbox and valid for the hostname/IP
used by clients. A changed server address may require certificate reissuance.

For command-line checks, use the verified certificate as a CA, not `curl -k`:

```sh
curl --cacert ./verified-home-ai-server.crt https://SERVER-IP:82/health
```

The Windows file client now refuses plaintext remote URLs and cross-origin
redirects. Update old saved connection/sync URLs to `https://SERVER-IP:82` and
re-authenticate after migration. It uses the system trust store, with an optional
`HOME_AI_CA_FILE` containing a previously verified public certificate/CA.
HTTP remains available only through the explicit `--allow-insecure-loopback`
server flag on a literal loopback address for isolated development/testing.

## First-owner proof

New installations require the one-time setup code shown only in a private
server-local file:

```sh
sudo cat /var/lib/home-ai-core/bootstrap-token
```

Enter it in the first-owner form over verified HTTPS. The API does not reveal
the code. An absent/incorrect code cannot create an owner; after setup the file
is removed and replay is rejected. Existing owner accounts are not reset.
If the initial token file is damaged, stop Core and regenerate the token from a
trusted local console before restarting; do not reset an initialized database.

## Update signing: mandatory before publishing/installing updates

All updates, **including development releases**, require an Ed25519 signature.
The release workflow deliberately fails when `HOME_AI_UPDATE_SIGNING_KEY` is
missing. Supply that GitHub Actions secret through an authenticated account
administrator's existing signing-key process. No production private key is
created by this change or committed to the repository.

Provision the matching public PEM key using a trusted console/channel:

```sh
sudo install -d -o root -g home-ai-core -m 0750 /etc/home-ai-core
sudo install -o root -g root -m 0644 ./verified-update-trusted.pub \
  /etc/home-ai-core/update-trusted.pub
```

The private signing key must not be installed on the home server. Key rotation
requires a separately authenticated administrative trust-anchor update.

The privileged helper checks the pinned signature over the exact archive
filename/checksum, copies the archive into root-controlled storage, verifies its
SHA-256, and extracts only the private copy using bounded archive validation.
It does not trust Core's mutable `prepared-*` directory or a manifest supplied
without a valid signature. Input paths are opened without following symlinks.
Verified staging and new rollback backups live in
`/var/lib/home-ai-core-updater`, outside the Core-owned state directory.
Old backups under `/var/lib/home-ai-core/update/backup` are **not imported or
trusted**. Result messages are written safely back to Core through an atomic,
no-follow file operation. Root-owned key/storage paths must remain unwritable
by `home-ai-core` and other unprivileged accounts.

## Module recovery and existing data

A `registered` row now represents a durable installation intent, written before
Docker side effects. Retrying the **same manifest and image digest** can recover
a managed container created before Core was interrupted, without deleting it.
A committed identical install is idempotent. An incomplete record is still
addressable for explicit removal. Pending modules do not advertise ready
capabilities. Cancelled jobs are not automatically resurrected; only the now
idempotent module-install job is automatically requeued after interruption.
Other interrupted jobs are marked failed for result inspection before retry.

Stop and remove require a functioning Docker engine, but no longer require the
`home-ai-modules` network to be healthy. Install/start/restart still need their
runtime prerequisites. Lifecycle actions are serialized and no longer wait on
an uncancellable registry mutex. A disconnected module request cancels its
helper context; ambiguous outcomes retain recoverable state.

New module data directories are under the root-controlled parent
`/var/lib/home-ai-modules`. Existing containers keep their current mounts.
Legacy data under `/var/lib/home-ai-core/modules/ID` are not deleted, moved or
replaced with empty data automatically. A fresh install encountering such data
is blocked with a migration-required error. Before relocating legacy data,
stop the affected module, back up its data, verify directory ownership and
contents from the server console, and explicitly plan the mount migration.
This source update does not provide an automatic legacy-data migration tool.

Different-image in-place upgrades remain deliberately blocked until staged
application-level health checks, data compatibility and transactional rollback
are implemented (issue #177). The short Docker-running check is not an
application health check. Do not confuse prevention of destructive replacement
with completion of that larger upgrade feature.

## Verification and acceptance

Automated regressions cover invalid/missing signatures, archive tampering,
symlink escapes, immutable verified staging, setup-code rejection/replay,
verified TLS, Secure/HttpOnly cookies, same-image adoption, durable install
intent, degraded-network cleanup, cancellation/restart handling and client TLS
verification. Go tests/vet and targeted race tests are part of development
validation. CI also builds Web, Debian amd64/arm64 and the Windows client.

Before real-server acceptance, verify a reviewed package, signing-key
provisioning, certificate trust, port availability, preserved users/data,
module reconciliation, and a signed update/rollback cycle on the target host.
A successful source/CI check alone is not a completed deployment.
