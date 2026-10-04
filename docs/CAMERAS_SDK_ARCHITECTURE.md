# Cameras SDK architecture

> Status: HCNetSDK V40 + server-side HCWebSDK WebSDK V3.3.1 / ISAPI control-plane implemented through `0.1.160-dev`. This is a fresh product module and does not reuse the retired `nvr` module runtime.

## Source SDKs

The implementation is based on the supplied Hikvision SDK packages:

- HCNetSDK V6.1.9.48 Linux 64-bit:
  - `lib/libhcnetsdk.so`
  - `lib/HCNetSDKCom/`
  - `incEn/HCNetSDK.h`
  - console example `GetStream.cpp`
- HCWebSDK WebSDK V3.3.1:
  - its official ISAPI endpoint map and control workflow are implemented server-side in Home-AI Core;
  - `webVideoCtrl.js` shows that the original browser SDK routes HTTP through `JS_SubmitHttpRequest` and video through `JS_Play`;
  - both calls depend on the Windows `HCWebSDKPlugin.exe`, so that executable is not embedded in Home-AI;
  - Core replaces the plugin transport while preserving the documented WebSDK/ISAPI workflow.

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

## Slice 3: HCWebSDK WebSDK V3.3.1 / ISAPI control-plane

`0.1.160-dev` promotes the supplied WebSDK from reference material to the canonical Hikvision Web/control-plane contract.

The original package was inspected directly. Its `webVideoCtrl.js` exposes the expected high-level operations such as device login/info, analog/digital channel queries, PTZ, playback and HTTP requests. In that package, ordinary HTTP is executed through the Windows plugin method `JS_SubmitHttpRequest`, and live playback through `JS_Play`. Therefore Home-AI does not copy the Windows plugin runtime into the product. Instead, Core performs the same ISAPI workflow directly on the server.

The initial server-side WebSDK adapter uses the official paths from WebSDK V3.3.1:

- `/ISAPI/Security/userCheck?format=json`;
- `/ISAPI/System/deviceInfo`;
- `/ISAPI/System/Video/inputs/channels`;
- `/ISAPI/ContentMgmt/InputProxy/channels`;
- `/ISAPI/ContentMgmt/InputProxy/channels/status`;
- `/ISAPI/Security/adminAccesses`;
- `/ISAPI/Streaming/channels` or `/ISAPI/ContentMgmt/StreamingProxy/channels`.

The adapter returns device identity/model/firmware, analog and digital channel data, digital online state, source camera IP/manage port, HTTP/RTSP/private SDK service ports and streaming profile identifiers.

Security boundary:

- only literal private/link-local target IPs;
- no HTTP proxy;
- no redirects;
- bounded response bodies and request timeout;
- HTTP Digest MD5/SHA-256 (including `-sess`) and Basic challenge handling;
- passwords are request-only and are never returned by API or audit.

The Cameras Web page now treats WebSDK/ISAPI as the preferred device/control probe. ONVIF discovery is usable even if HCNetSDK has not yet been installed. The private `dev_manage` port obtained from WebSDK is reused to prepare the HCNetSDK V40 diagnostic path.

HCNetSDK remains the native Linux media backend and will be used for `NET_DVR_RealPlay_V40`; WebSDK/ISAPI supplies device/channel/control metadata around that media path.

## Next slices

1. live acceptance of the WebSDK/ISAPI probe against real Hikvision/HiWatch cameras and recorders;
2. `NET_DVR_RealPlay_V40` server-side live ingest using confirmed channel/stream identifiers;
3. encrypted persistent credentials and automatic WebSDK/HCNetSDK session recovery;
4. browser live transport without `HCWebSDKPlugin.exe`;
5. PTZ, presets and configuration operations through the same official WebSDK/ISAPI endpoint map;
6. recording/archive as a later independent layer.
