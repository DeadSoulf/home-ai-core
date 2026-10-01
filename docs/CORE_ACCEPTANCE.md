# Core Acceptance Checklist

> This file is the live-server acceptance subset of [Core 1.0 readiness](CORE_1_0_READINESS.md). Automated CI gates and the final completion rule are tracked there.

This checklist tracks live checks that require an installed Home-AI server. CI cannot replace these tests.

## Update / rollback

- [x] Install a newer dev update from Web.
- [x] Confirm the new Core version is running.
- [x] Run Web rollback.
- [x] Confirm the previous version starts successfully.
- [x] Confirm login, System page and state database still work after rollback.
- [x] Reinstall the newer version after the rollback test.

### Recorded live run — 2026-10-01

- update to `0.1.109-dev`: passed;
- Web rollback to the previous installed version: passed;
- Core/Web/login/state remained operational after rollback: passed by user acceptance;
- re-update to `0.1.109-dev`: passed; normal operation confirmed by user;
- network after reboot was also confirmed working by the user; exact DHCP/static profile type and backend were not recorded for this run.

## Network persistence

- [x] Confirm the active Home-AI network configuration survives reboot and connectivity returns.

Recorded on 2026-10-01: the user confirmed the server network was already checked after reboot and is working. The exact active profile mode (DHCP or static) was not recorded, so the mode-specific matrix below remains regression evidence rather than an inferred result.

Use a non-critical physical interface when possible.

### Mode-specific regression coverage

#### DHCP

- [ ] Configure the interface through Home-AI for DHCP.
- [ ] Confirm an IPv4 address is received.
- [ ] Reboot the server.
- [ ] Confirm the same profile is still configured.
- [ ] Confirm the interface receives an address again after boot.

#### Static IPv4

- [ ] Configure a test static IPv4/CIDR.
- [ ] Configure gateway/DNS if needed.
- [ ] Confirm connectivity.
- [ ] Reboot.
- [ ] Confirm the static profile survives and connectivity returns.

## WireGuard

- [x] Install `wireguard-tools` from Web.
- [x] Create a test `wg0` tunnel.
- [x] Add a peer.
- [x] Confirm the tunnel starts.
- [x] Reboot.
- [x] Confirm `wg0` starts automatically.
- [x] Confirm peer state/handshake data is visible in Web.
- [x] Remove the test tunnel when finished.

### WireGuard live acceptance — 2026-10-01

- WireGuard tunnel/peer workflow, reboot persistence, automatic start and peer/handshake visibility were confirmed working by the user.
- The exact test peer/address values are intentionally not recorded in project documentation.

## Result recording

For each live acceptance run, record:

- Home-AI version;
- Debian version;
- network backend;
- date;
- result;
- any observed error/log excerpt.

A failed item becomes an engineering task before stable release, but it does not automatically block unrelated product-module development unless it affects data safety or update recovery.
