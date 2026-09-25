# Home AI Core — Cameras

## Scope

Camera Core 0.0.11 introduces the persistent camera inventory and health layer used by the
future NVR stack.

This version intentionally does not decode video in the Web UI yet. Media discovery,
Live View, recording, archive and analytics will build on the same camera IDs and database.

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

## Health checks

Camera Core periodically checks enabled cameras. In 0.0.11 this is a bounded TCP reachability
check against the RTSP host and port with a short timeout.

The state is one of:

```text
unknown
online
offline
disabled
```

`online` in 0.0.11 means the configured RTSP endpoint is reachable at the network layer.
The next media layer will perform RTSP/codec probing and distinguish authentication,
stream-path and codec failures.

## Web UI

The Cameras page provides:

- total / online / offline / disabled counters
- add camera form
- edit without exposing the stored password
- enable/disable control
- manual connectivity check
- delete action
- automatic status refresh

## Next stages

1. ONVIF discovery and media profiles.
2. RTSP/codec probing and snapshots.
3. Live View with main/sub streams.
4. Recorder integrated with the video Storage Pool.
5. Archive/timeline and event metadata.
6. Motion/object analytics and Automation Core integration.
