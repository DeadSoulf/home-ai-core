# ADR-0012: Automatic Hardware Driver Reconciliation

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core is intended to manage a physical home server. Hardware can be added after installation, including GPUs, network adapters, storage controllers and USB devices.

The current Core inventory is read-only and must remain unprivileged. Installing kernel modules or Debian packages requires privileged host changes and therefore cannot be performed directly by the network-facing Core process.

## Decision

### Hardware identity

Core inventory records stable hardware identity before relying on a loaded driver.

For PCI devices this includes:

- PCI class
- vendor ID
- device ID
- PCI address
- kernel modalias
- currently bound kernel driver, when present

GPU discovery scans PCI display classes directly rather than relying on DRM nodes, so an unbound GPU is still visible.

USB driver management will use the equivalent USB IDs and modalias when that inventory is added.

### Driver reconciliation

Home-AI-Core will maintain a Driver Manager workflow:

1. detect a newly added or newly visible device;
2. determine whether a suitable driver is already bound;
3. match hardware IDs/modalias against a trusted driver profile;
4. validate architecture, Debian/Core compatibility and package signatures;
5. enqueue a persistent `driver.reconcile` job;
6. execute only allowlisted package/module operations through the privileged helper;
7. rescan the device and verify that the expected driver is bound;
8. publish events and write an audit record.

Unambiguous trusted matches may be installed automatically.

If multiple profiles match, a package requires an unaccepted license, Secure Boot/manual enrollment is required, or the change cannot be safely rolled back, Core presents the required action in the UI instead of choosing arbitrarily.

### Trust

Driver profiles and packages use the same signed repository trust model established for Phase 9.

Repository metadata never grants arbitrary shell execution. The privileged helper accepts typed, allowlisted operations only.

### UI

The System page shows:

- detected device model
- vendor
- hardware IDs
- bus address
- active driver
- explicit `driver not installed` state

Driver installation/reconciliation progress is represented as normal persistent jobs.

## Consequences

### Positive

- newly installed hardware is visible even before a driver is loaded;
- supported hardware can become usable without manual SSH package installation;
- all privileged changes remain constrained, observable and auditable;
- driver behavior is reusable for GPU, network, storage and USB devices.

### Negative

- some proprietary or Secure-Boot-sensitive drivers cannot be fully unattended;
- automatic installation depends on a trusted driver profile existing for the device;
- a privileged helper is required before actual package installation can be enabled.
