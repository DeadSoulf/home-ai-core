# Module Store and Updates Phase Acceptance Criteria

## Repository metadata and package trust

- [x] repository metadata schema v1 exists
- [x] repository index uses detached Ed25519 signatures
- [x] index signature is verified before metadata is accepted
- [x] unknown repository fields are rejected
- [x] release manifests reuse Module Manifest v1 validation
- [x] duplicate module/version releases are rejected
- [x] package URLs require HTTPS
- [x] package size is verified
- [x] package SHA-256 is verified
- [x] package Ed25519 signature is verified
- [x] untrusted signing keys are rejected
- [x] tampered index/package tests exist

## Repository configuration and key management

- [x] persistent repository sources
- [x] persistent trusted signing keys
- [ ] explicit add/remove repository API
- [ ] explicit trust/key rotation workflow
- [ ] repository refresh job
- [ ] last-known-good repository metadata

## Planning and lifecycle API

- [ ] store catalog API
- [ ] install plan preview
- [ ] update plan preview
- [ ] reverse-dependency-safe remove plan
- [ ] modules.manage authorization for mutations
- [ ] CSRF protection for cookie mutations
- [ ] install/update/remove operations create persistent jobs
- [ ] lifecycle changes are audited

## Hardware and driver reconciliation

- [x] GPU inventory works without a loaded graphics driver
- [x] GPU inventory exposes model, hardware IDs, PCI address, modalias and active driver
- [x] Web UI shows GPU model and missing-driver state
- [ ] generic PCI/USB device change detection
- [ ] signed driver profile format
- [ ] modalias/hardware-ID matching engine
- [ ] automatic driver.reconcile persistent job
- [ ] post-install driver binding verification
- [ ] reboot-required state
- [ ] ambiguous/proprietary/Secure-Boot cases require explicit UI action
- [ ] driver changes are audited

## Package staging and privileged boundary

- [ ] bounded package download
- [ ] verified staging directory
- [ ] archive path traversal protection
- [ ] package layout contract
- [ ] dedicated privileged helper/service boundary
- [ ] allowlisted privileged operations only
- [ ] no arbitrary shell strings from repository metadata
- [ ] install/upgrade/remove execution

## Rollback and recovery

- [ ] pre-upgrade backup hook where supported
- [ ] atomic current-version switch
- [ ] failed install leaves previous version active
- [ ] failed upgrade restores previous version
- [ ] interrupted operation recovery
- [ ] rollback events and audit records

## Core/module updates

- [ ] module update discovery
- [ ] compatible update selection
- [ ] Core update metadata and signature verification
- [ ] Core/module compatibility ordering
- [ ] update status in Web UI

## Validation

- [x] signed repository verification tests
- [x] tampered repository rejection test
- [x] tampered package rejection test
- [ ] API contract tests
- [ ] job lifecycle tests
- [ ] helper authorization tests
- [ ] rollback tests
- [ ] Debian physical-node acceptance
