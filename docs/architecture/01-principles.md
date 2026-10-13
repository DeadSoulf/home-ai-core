# Architecture Principles

1. **Debian remains the operating system.**
2. **Core stays small.** Functional workloads are external Docker modules.
3. **API first.** The Web frontend never performs privileged operations directly.
4. **Least privilege.** Every module declares permissions, capabilities and host resources.
5. **No Docker socket in Core.** Container lifecycle crosses the privileged-helper boundary.
6. **Explicit lifecycle.** Install, start, stop, update and remove are controlled operations.
7. **Stable contracts.** Core APIs and module manifests are versioned.
8. **Long operations are jobs.**
9. **Events are first-class.** Modules use typed contracts rather than hidden coupling.
10. **Independent state.** Product-domain state belongs to the module, not the Core database.
11. **Independent releases.** Core and modules can be updated separately.
12. **Rollback is designed in.**
13. **Third-party code is tracked.**
14. **No feature before contract.**
