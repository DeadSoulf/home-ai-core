# Core update signing

Home-AI-Core uses detached Ed25519 signatures to authenticate Core update checksum metadata.

## Trust boundary

- The private signing key is kept outside the repository and outside Home-AI nodes.
- GitHub Actions receives the private key only through the repository secret `HOME_AI_UPDATE_SIGNING_KEY`.
- Managed Home-AI nodes trust only the matching public key installed at `/etc/home-ai-core/update-trusted.pub`.
- The signature covers the exact bytes of `home-ai-core-update_<version>_<arch>.tar.gz.sha256`.
- Core verifies the signature before trusting the SHA-256 value contained in that file.
- Every non-`-dev` release requires a signature. A stable/RC release without one is not accepted.

## Generate a dedicated key

Run this on an offline/admin workstation, not on a Home-AI server:

```sh
openssl genpkey -algorithm ED25519 -out home-ai-update-signing-key.pem
openssl pkey -in home-ai-update-signing-key.pem -pubout -out update-trusted.pub
```

Store `home-ai-update-signing-key.pem` in the GitHub Actions secret named `HOME_AI_UPDATE_SIGNING_KEY`.

Install only the public key on Home-AI nodes:

```sh
install -o root -g root -m 0644 update-trusted.pub /etc/home-ai-core/update-trusted.pub
```

Never commit the private key.

## Release behavior

The release workflow signs every Core `.sha256` asset when the secret is configured. Development releases may be emitted unsigned while signing is being provisioned. Non-development releases fail the workflow when the signing secret is missing.

Core release discovery requires the `.sha256.sig` asset for every non-development release. Download then verifies the detached signature, archive checksum, manifest, file sizes and per-file SHA-256 values before the privileged helper may install the bundle.

## Rotation

Key rotation must be staged: distribute the new public trust root before releases are signed only by the new private key. A future multi-key trust file may make overlap/rotation automatic; until then rotation is an explicit maintenance operation.
