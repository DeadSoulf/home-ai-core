# Docker Module Distribution Acceptance Criteria

The previous package-archive distribution design has been superseded by Docker images pinned to immutable digests.

## Catalog and trust

- [x] repository/catalog metadata foundation exists;
- [x] detached Ed25519 index verification foundation exists;
- [x] untrusted signing keys and tampered metadata are rejected;
- [x] release manifests use current Manifest v2 validation;
- [ ] adapt catalog records to Docker image digests instead of module package archives;
- [ ] explicit repository add/remove API;
- [ ] key rotation workflow;
- [ ] repository refresh job;
- [ ] last-known-good catalog metadata.

## Installation planning

- [x] dependency planner exists;
- [x] Core compatibility validation exists;
- [x] architecture/capability validation exists;
- [ ] enforce the complete install plan immediately before Docker mutation;
- [ ] update-plan preview;
- [ ] reverse-dependency-safe removal.

## Docker delivery boundary

- [x] Docker Engine is managed through the privileged helper;
- [x] Core does not receive `docker.sock`;
- [x] module images require immutable SHA-256 digests;
- [x] image pull is performed by the helper;
- [x] helper creates only managed Home-AI containers;
- [x] no arbitrary shell command comes from catalog or manifest metadata;
- [x] install/start/stop/restart/remove are allowlisted operations;
- [x] module persistent data is isolated per module.

## Update and recovery

- [ ] module update discovery;
- [ ] explicit update job using a new verified digest;
- [ ] health verification before marking update successful;
- [ ] rollback to the previously known image digest;
- [ ] update/rollback audit events;
- [ ] garbage collection of superseded images.

## Validation

- [x] manifest validation tests;
- [x] signed catalog verification tests;
- [x] tampered metadata tests;
- [x] Job Engine integration for lifecycle;
- [x] amd64/arm64 Core builds;
- [ ] helper Docker lifecycle unit/integration tests;
- [ ] Debian physical-node Docker acceptance;
- [ ] rollback tests.
