# Contributing

Home-AI-Core uses architecture-first development.

Before adding a feature:

1. Identify whether it belongs to Core, a Module, an App or an Integration.
2. Document or update the public contract.
3. Record third-party code and licenses before source reuse.
4. Implement on `develop` or an appropriate feature branch.
5. Add automated tests.
6. Update documentation.
7. Merge to `main` only after the phase Definition of Done is satisfied.

Do not add Docker-, KVM-, NAS-, AI- or NVR-specific logic to Core.
