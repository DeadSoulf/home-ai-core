# Cameras SDK architecture

> Status: first implementation slice. This is a fresh product module and does not reuse the retired `nvr` module runtime.

## Source SDKs

The implementation is based on the supplied Hikvision SDK packages:

- HCNetSDK V6.1.9.48 Linux 64-bit:
  - `lib/libhcnetsdk.so`
  - `lib/HCNetSDKCom/`
  - `incEn/HCNetSDK.h`
  - console example `GetStream.cpp`
- HCWebSDK WebSDK V3.3.1:
  - used only as an API/ISAPI/Web workflow reference;
  - its browser playback path depends on the Windows HCWebSDK plugin and is not used as the Home-AI browser runtime.

## Product boundary

The new first-party module ID is `cameras`.

It is independent from the dormant `nvr` groundwork. The old NVR database schema and experimental media code remain in Core but are not wired into this module.

The first supported platform is Linux amd64 because the supplied native HCNetSDK package is x86-64. Other architectures expose an explicit unsupported runtime until a matching vendor SDK is provided.

## Runtime loading

Home-AI does not link against HCNetSDK at build time.

The Linux amd64 runtime loads `libhcnetsdk.so` dynamically with `dlopen`. This keeps normal Go builds and non-amd64 builds independent from proprietary SDK binaries.

Library lookup order:

1. `HOME_AI_HCNETSDK_LIB`
2. `HOME_AI_HCNETSDK_DIR/lib/libhcnetsdk.so`
3. `HOME_AI_HCNETSDK_DIR/libhcnetsdk.so`
4. `/opt/home-ai/hikvision/lib/libhcnetsdk.so`
5. `/opt/home-ai/hikvision/HCNetSDK/lib/libhcnetsdk.so`
6. common local system library paths

The vendor package's `HCNetSDKCom` and bundled runtime dependencies must be installed next to the SDK in a layout supported by Hikvision.

## Slice 1: SDK readiness and login probe

The first slice implements:

- `NET_DVR_Init`
- optional `NET_DVR_SetConnectTime`
- `NET_DVR_Login_V30` for a minimal synchronous compatibility/login probe
- `NET_DVR_GetLastError`
- `NET_DVR_Logout`
- `NET_DVR_Cleanup`

`NET_DVR_Login_V30` is deliberately used only for the first login probe because it has a small stable call surface and lets us validate the supplied native runtime before adding larger vendor structures.

Credentials are accepted only by the authenticated POST probe endpoint and are never returned in API responses or audit metadata.

Initial camera targets must be literal private/link-local IP addresses. Port 8000 is the default HCNetSDK private-service port.

## API

- `GET /api/v1/cameras/status`
- `POST /api/v1/cameras/test-login`

The login probe requires `camera.manage`.

## Web

When the `cameras` module is enabled, Home-AI exposes `/modules/cameras`.

The first page shows:

- architecture support;
- HCNetSDK library detection;
- SDK initialization state;
- library/error diagnostics;
- IP / SDK port / username / password login probe.

## Next slices

1. controlled installation of the supplied HCNetSDK runtime into `/opt/home-ai/hikvision`;
2. `NET_DVR_Login_V40` and device metadata;
3. channel enumeration for cameras and recorders;
4. vendor-native device discovery where supported by the supplied SDK/API set;
5. `NET_DVR_RealPlay_V40` live stream ingestion;
6. persistence of camera definitions and encrypted credentials;
7. browser live transport without HCWebSDK Windows plugin;
8. recording/archive as a later independent layer.
