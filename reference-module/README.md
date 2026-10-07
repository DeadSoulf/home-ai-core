# Home-AI Reference Module

This is the first reference workload for Home-AI Docker Module Runtime v2.

It exists only to validate the generic module platform. It is not a product module.

## Contract

- module ID: `reference.module`
- version: `0.1.0`
- HTTP port inside module network: `8080`
- health endpoint: `/health`
- state endpoint: `/state`
- persistent state: `/data/state.json`

Each successful start increments `boot_count`. Recreating the container with the same `/data` directory must keep the counter, which validates persistence across remove/reinstall.

The production image is built for `linux/amd64` and `linux/arm64`. The generated `manifest.json` pins the image to the exact multi-architecture OCI digest.
