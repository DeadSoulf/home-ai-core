# ADR-0011: Signed Module Repository and Package Trust

- Status: Accepted
- Date: 2026-09-28

## Context

Phase 9 introduces downloadable module metadata and packages. A module package can eventually contain executable code and request privileged host operations, so transport security alone is not an adequate trust boundary.

Home-AI-Core must reject metadata or package bytes that were modified after publication, and it must not treat a repository as trusted merely because it is reachable over HTTPS.

## Decision

### Repository index

Module repositories publish a versioned JSON index using `module-repository-v1.schema.json`.

The index contains:

- repository ID
- generation timestamp
- one or more module releases
- the complete Module Manifest v1 for each release
- package HTTPS URL
- exact package size
- lowercase SHA-256 digest
- detached Ed25519 package signature metadata

The index itself is distributed with a detached Ed25519 signature.

Core verifies the detached index signature against an explicitly trusted key **before** parsing or using the index.

### Package verification

For a package selected from a verified index, Core verifies:

1. URL is absolute HTTPS.
2. downloaded byte count matches `size_bytes`.
3. SHA-256 matches the digest in the signed repository index.
4. Ed25519 signature over the raw 32-byte SHA-256 digest is valid for the declared trusted key.

A package failing any check is rejected before extraction or lifecycle execution.

### Trust roots

Trust is explicit. A key is identified by `key_id` and must already exist in the local trusted-key set.

Downloading a repository index must never implicitly add the signing key contained by or advertised by that repository.

Persistent repository configuration and key management are implemented in the next Phase 9 slice.

### Manifest validation

Every release manifest from the repository is validated through the existing Module Manifest v1 validator.

Repository metadata cannot weaken:

- Core version compatibility
- architecture constraints
- dependency constraints
- conflicts
- permission declarations
- capability requirements
- lifecycle declarations

### Package format

This ADR defines authenticity and integrity, not the internal archive layout. The package payload remains opaque until the package-layout/privileged-execution slice is accepted.

## Security properties

- HTTPS protects transport but does not establish module trust.
- Repository metadata is authenticated before use.
- Package bytes are authenticated independently of the transport.
- Trust roots are local and explicit.
- No unverified package is extracted or executed.
- Signing private keys never exist on a Home-AI-Core node.

## Consequences

### Positive

- A compromised mirror cannot silently replace module packages.
- Repository metadata and package integrity can be tested independently.
- The existing dependency planner can consume only verified manifests.
- Key rotation can be introduced without changing Module Manifest v1.

### Negative

- Repository operators must manage signing keys securely.
- Key distribution and revocation need an explicit administrative workflow.
- Package installation remains unavailable until the privileged execution boundary is implemented.
