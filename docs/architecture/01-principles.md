# Architecture Principles

1. **Debian remains the operating system.** Home-AI-Core must not replace or hide the underlying Debian installation.
2. **The core stays small.** Product features such as Docker, KVM, NAS, AI and NVR are modules.
3. **API first.** The web frontend never performs privileged system operations directly.
4. **Least privilege.** Every module declares the permissions and resources it requires.
5. **Explicit lifecycle.** Modules support defined install, upgrade, remove, backup and restore operations.
6. **Stable contracts.** Core APIs and module manifests are versioned.
7. **Long operations are jobs.** Formatting disks, pulling images, backups and VM operations run through a job engine.
8. **Events are first-class.** Modules communicate through typed events rather than hidden coupling.
9. **Upstream APIs first.** Prefer Docker Engine API, libvirt, systemd/D-Bus and other upstream interfaces over copying management-panel internals.
10. **Third-party code is tracked.** Every reused source component must have a known license, origin and modification record.
11. **Rollback is designed in.** Core and module updates must have recovery paths.
12. **No feature before contract.** New functionality is implemented only after its interface and ownership are documented.
