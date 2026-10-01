# Core 1.0 readiness

This checklist defines when the shared Home-AI-Core foundation may be marked complete and product work can continue without adding unrelated server-platform scope.

## Automated engineering gates

- [x] Core API, identity, RBAC/scoped permissions, audit, jobs, events, WebSocket, modules, hardware inventory and privileged-helper boundaries are implemented.
- [x] Update bundles verify archive SHA-256, bounded extraction, manifest metadata, exact file sizes and per-file SHA-256.
- [x] Negative bundle tests cover payload tampering, unexpected files and archive path traversal.
- [x] CI runs Go tests/vet, Web typecheck/tests/build, integration smoke, Linux amd64/arm64 cross-build and Windows client tests/build.
- [ ] Stable-channel update manifests/assets are authenticated with an offline-managed signing key and a pinned Core trust root.
- [ ] Long-duration update failure tests cover interrupted downloads/restarts and recovery from failed installation attempts.

## Live acceptance gates

These checks require an installed HOME AI server and may interrupt connectivity. They are never performed automatically by CI.

### Update / rollback / re-update

- [x] Install a newer dev update from Web.
- [x] Confirm the new Core version and Web UI start.
- [x] Roll back from Web.
- [x] Confirm the previous Core version starts.
- [x] Confirm login, System page and the existing SQLite state database still work.
- [x] Reinstall the newer version and confirm normal operation.

### Network persistence

- [x] Confirm the active Home-AI-managed network configuration survives reboot and connectivity returns.

Live acceptance was confirmed by the user on 2026-10-01. The exact active mode (DHCP or static IPv4) was not recorded; mode-specific DHCP/static regression coverage remains documented in [CORE_ACCEPTANCE.md](CORE_ACCEPTANCE.md) without being inferred.

### WireGuard persistence

- [ ] Install/use wireguard-tools from Web.
- [ ] Create a test tunnel and peer.
- [ ] Confirm the tunnel starts and peer state is visible.
- [ ] Reboot and confirm the tunnel starts automatically and handshake/RX/TX state is visible.
- [ ] Remove the test tunnel after acceptance.

## Completion rule

Core Foundation may be marked **COMPLETE** when:

1. all live P0 acceptance checks above are recorded as passed;
2. stable release signing/trust-root handling is implemented and tested;
3. no failed acceptance item threatens data safety, authentication, update recovery or persistent networking.

Items such as NAS/SMB acceptance, Windows client UX, Smart Home, NVR, AI and cluster functionality remain product/domain work and do not reopen the Core foundation unless they expose a shared-contract defect.
