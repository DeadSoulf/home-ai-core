# Cameras SDK architecture

> Status: HCNetSDK V40 metadata/channel slice implemented in `0.1.159-dev`. This is a fresh product module and does not reuse the retired `nvr` module runtime.

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

The vendor package's `HCNetSDKCom` and bundled runtime dependencies must be installed next to the SDK in a layout supported by Hikvision. Home-AI can import the original Linux64 vendor ZIP from the authenticated Web UI and expands only the bounded `lib/` runtime subtree into the Core state directory.

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

## Supporting slices already implemented

- authenticated Web upload/import of the original HCNetSDK Linux64 ZIP into the Core state directory;
- runtime refresh after SDK installation without relinking Core;
- bounded ONVIF WS-Discovery for local devices;
- persistent camera definitions after successful HCNetSDK authentication; passwords are not persisted yet.

## Slice 2: V40 device metadata and channel model

`0.1.159-dev` moves the primary probe to `NET_DVR_Login_V40`.

The Linux amd64 dynamic wrapper mirrors the ABI of the supplied HCNetSDK V6.1.9.48 structures without including or redistributing the proprietary vendor header in normal Core builds:

- `NET_DVR_USER_LOGIN_INFO`;
- `NET_DVR_DEVICEINFO_V30`;
- `NET_DVR_DEVICEINFO_V40`;
- `NET_DVR_DEVICECFG_V40`.

After successful login Home-AI records safe device metadata from `NET_DVR_DEVICEINFO_V40`. When `NET_DVR_GetDVRConfig` is exported and the device accepts `NET_DVR_GET_DEVICECFG_V40`, the probe also reads:

- device name;
- serial number;
- device/model type name;
- software version and build date;
- analog channel count/start;
- IP channel count/start.

The Web probe renders an explicit analog/IP channel list. This is the initial recorder-aware channel model needed before `NET_DVR_RealPlay_V40`.

The password remains request-only. It is never returned by the API and is not written into the current camera JSON store.

## Next slices

1. live acceptance of the V40 metadata/channel probe against real Hikvision/HiWatch hardware;
2. `NET_DVR_RealPlay_V40` live stream ingestion for a selected channel;
3. encrypted persistent camera credentials and automatic session recovery;
4. browser live transport without the HCWebSDK Windows plugin;
5. richer channel capability/online-state discovery for DVR/NVR devices;
6. recording/archive as a later independent layer.
