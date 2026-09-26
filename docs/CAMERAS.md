# Home AI Core — Cameras

## Scope

Camera Core 0.0.18 introduces the persistent camera inventory and health layer used by the
future NVR stack.

0.0.16 adds ONVIF PTZ controls and the first browser Live View implementation on top of the
automatic ONVIF/RTSP setup. The Live View is an on-demand JPEG preview that refreshes while the
user keeps it enabled. Continuous high-frame-rate video transport, recording, archive and
analytics will share the later recorder pipeline.

## Runtime data

Camera Core stores its runtime files in:

```text
runtime/cameras/
├── cameras.db
└── secret.key
```

The directory is restricted to mode `0700`. The local camera secret key is 32 random bytes
and is kept at mode `0600`.

Camera passwords are encrypted before SQLite storage with AES-256-GCM. The normal camera list
API only reports whether a password exists; it never returns the password itself.

RTSP URLs must use:

```text
rtsp://host[:port]/path
rtsps://host[:port]/path
```

Credentials embedded in the URL (for example `rtsp://user:pass@host/...`) are rejected.
Username and password are stored in their dedicated fields.

## Permissions

Reading the inventory requires:

```text
cameras.view
```

Creating, editing, deleting or manually probing cameras requires:

```text
cameras.manage
```

Mutating API calls also require:

```text
X-HomeAI-Request: 1
```

## API

List cameras:

```text
GET /api/cameras
```

Create a camera:

```text
POST /api/cameras/save
X-HomeAI-Request: 1

name=Front+Door
rtsp_url=rtsp%3A%2F%2F192.168.1.50%3A554%2Fstream1
username=viewer
password=secret
enabled=1
```

Update a camera by adding `id`. If the existing password must be retained, omit
`update_password=1` and send an empty password. To replace or clear the password, send
`update_password=1`.

Delete:

```text
POST /api/cameras/delete
X-HomeAI-Request: 1

id=1
```

Connectivity check:

```text
POST /api/cameras/probe
X-HomeAI-Request: 1

id=1
```

ONVIF discovery:

```text
POST /api/cameras/discover
X-HomeAI-Request: 1

timeout_ms=2000
```

Automatic ONVIF stream discovery:

```text
POST /api/cameras/onvif-streams
X-HomeAI-Request: 1

onvif_xaddr=http%3A%2F%2F192.168.1.50%2Fonvif%2Fdevice_service
username=admin
password=secret
```

The response contains ONVIF Media Profiles, sanitized RTSP URIs and GetDeviceInformation
metadata. The recommended profile is the highest-resolution profile returned by the camera.
Credentials are not included in the returned RTSP URI and are not written to audit details.

Real RTSP media probe:

```text
POST /api/cameras/media-probe
X-HomeAI-Request: 1

id=1
```

JPEG snapshot:

```text
GET /api/cameras/snapshot?id=1
```

## Health checks

Camera Core periodically performs a lightweight bounded TCP reachability check against the
configured RTSP host and port. This remains cheaper than launching ffprobe for every health cycle.

The state is one of:

```text
unknown
online
offline
disabled
```

`online` means the configured RTSP endpoint is reachable at the network layer.

An on-demand media probe uses `ffprobe` and reports the real video codec, resolution, FPS and
audio codec. Snapshot capture uses `ffmpeg` to decode one frame as JPEG.

These runtime tools are optional for Core startup. On Debian install them with:

```bash
sudo apt update
sudo apt install -y ffmpeg
```

Home AI Core launches these tools directly rather than through a shell and does not log camera
credentials. Media-tool error text is redacted before it is returned by the API.

## Web UI

The Cameras page provides:

- total / online / offline / disabled counters
- add/edit form with automatic ONVIF state kept internally
- edit without exposing the stored password
- enable/disable control
- compact camera discovery list showing only the camera IP/address
- ONVIF WS-Discovery plus a bounded local IPv4 camera-port scan
- Hikvision-compatible discovery through the normal camera service ports even when ONVIF is disabled
- technical XAddr, scopes and scanned ports are not shown in the normal Web UI
- manufacturer, model, firmware version and serial number
- persistent device metadata on camera cards
- ONVIF PTZ capability detection from PTZ service + Media Profile
- press-and-hold pan / tilt / zoom controls with Stop on release
- on-demand Live View preview with refreshed JPEG frames
- automatic RTSP discovery from ONVIF Media Profiles
- automatic selection of the highest-resolution stream
- alternate profile selection for substreams
- fast RTSP endpoint check
- real media probe with codec / resolution / FPS
- JPEG snapshot preview
- delete action
- automatic status refresh

## Next stages

1. Recorder integrated with the video Storage Pool.
2. Continuous browser video transport using the recorder pipeline.
3. Archive/timeline and event metadata.
4. Motion/object analytics and Automation Core integration.


## Automatic RTSP setup

For an ONVIF camera the user does not need to know the RTSP path.

The normal flow is:

```text
Find ONVIF cameras
        ↓
Select a discovered camera
        ↓
Enter camera username/password
        ↓
Detect stream automatically
        ↓
GetCapabilities
        ↓
GetProfiles
        ↓
GetStreamUri
        ↓
Recommended RTSP URL is filled into the form
```

ONVIF authentication uses WS-Security UsernameToken PasswordDigest. Home AI Core does not
send an HTTP Basic Authorization header by default, so the password is not transmitted as
clear-text HTTP Basic credentials. The current automatic Media client supports HTTP ONVIF
XAddr endpoints. Manual RTSP entry remains available as a compatibility fallback.


## Camera device information

During automatic ONVIF setup Home AI Core calls:

```text
GetDeviceInformation
```

When supported by the camera, the following values are stored:

```text
Manufacturer
Model
FirmwareVersion
SerialNumber
HardwareId
```

The metadata is stored separately from credentials and is safe to return through the normal
camera inventory API. Existing cameras can refresh this information by opening Edit and running
automatic stream detection again.


## PTZ and Live View

During automatic ONVIF setup Camera Core now also requests PTZ capabilities. PTZ controls are
enabled only when both a PTZ service XAddr and a PTZ-capable Media Profile are detected.

The Web UI sends these commands:

```text
left
right
up
down
zoom_in
zoom_out
stop
```

Movement uses ONVIF ContinuousMove with a short safety timeout. The Web UI sends Stop when the
user releases the control.

Live View in 0.0.16 is intentionally an on-demand JPEG preview. It repeatedly requests the
existing authenticated snapshot endpoint only while Live is enabled. This keeps the browser
implementation simple and avoids exposing RTSP credentials to the browser.

WebServer client requests are now handled by short worker threads so a camera snapshot or
ffmpeg operation does not block unrelated Web/API requests. Client socket timeouts and shutdown
waiting keep server stop/restart bounded.

A later recorder milestone will replace the repeated-JPEG preview with a persistent continuous
video transport shared with recording and archive playback.


## Simplified discovery UI

Starting with 0.0.17, WS-Discovery results are presented as a compact user-facing list:

```text
10.10.10.13    [Use]
10.10.10.3     [Use]
10.10.10.10    [Use]
```

The ONVIF XAddr, scopes and profile URLs remain available internally to Camera Core but are
not rendered in the discovery list. The XAddr form field is also hidden from the normal camera
form; selecting a camera stores it automatically.


## Discovery without ONVIF

Starting with 0.0.18, ONVIF is no longer required for a camera to appear in discovery.

Camera Core combines:

```text
ONVIF WS-Discovery
        +
local IPv4 camera-port scan
```

The LAN scanner is bounded to at most 254 addresses around the server's active local IPv4
network and probes only common camera/service ports:

```text
554   RTSP
8554  alternate RTSP
8000  Hikvision-compatible service
8899  common alternate ONVIF service
37777 Dahua-compatible service
34567 NetSurveillance-compatible service
80/443 Web service hints
```

A normal Web server exposing only 80/443 is not classified as a camera.

For a Hikvision-compatible device with RTSP on port 554, Camera Core prepares the common main
stream path automatically:

```text
rtsp://CAMERA_IP:554/Streaming/Channels/101
```

This does not require ONVIF. Username and password remain separate and are still protected by
Camera Core. If ONVIF is available, ONVIF Media Profiles remain preferred because they provide
authoritative stream, PTZ and device metadata.
