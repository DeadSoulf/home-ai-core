# Home-AI Cameras / NVR Architecture

**Status:** accepted foundation for implementation  
**Baseline:** Home-AI-Core 0.1.133-dev  
**Date:** 2026-10-02

## 1. Goal

Home-AI Cameras / NVR is a first-party, local-first video-surveillance module integrated into the existing Home-AI identity, permissions, storage, audit, jobs, events and AI Agent boundaries.

The first usable NVR must remain useful without Internet and without an AI model.

The module must provide:

- IP/RTSP camera sources;
- ONVIF discovery/import where supported;
- live view;
- continuous and event-based recording;
- local archive and timeline;
- basic motion events;
- protected evidence/clips;
- ring retention on storage explicitly assigned to purpose `video`;
- per-camera access permissions;
- camera health/reconnect;
- stable APIs for later object detection, face recognition, LPR, semantic search and AI Agent tools.

## 2. Product principles

### Recording is more important than analytics

Capture/recording must not depend on object detection, face recognition, the AI Agent, Internet access or external AI providers.

If analytics fails, recording continues.

### Local-first

Normal live viewing, recording, archive search, export and retention are local.

External services are optional future integrations and never become the required camera data path.

### One Home-AI identity model

NVR does not create separate users.

Camera access uses the existing Core actor and exact resource-scoped permission model:

```text
camera.live     + camera + cam_x
camera.archive  + camera + cam_x
camera.export   + camera + cam_x
camera.ptz      + camera + cam_x
camera.manage   + camera + cam_x
```

Administrator/global permissions remain possible through the existing Core model.

### Separate control plane and media plane

Core owns authoritative metadata and policy.

High-rate video/audio does not travel through SQLite durable events, generic audit payloads or WebSocket event history.

```text
Browser / Client
      |
      v
Home-AI Core control plane
  - identity
  - camera metadata
  - permissions
  - archive index
  - events
  - jobs
  - audit
      |
      +------------------------+
      |                        |
      v                        v
NVR media runtime          Vision runtime
  - RTSP ingest             - motion
  - live restream           - object detection (later)
  - segment recorder        - face/LPR (later)
  - clip/export             - embeddings (later)
      |
      v
video storage
```

## 3. Module identity

Initial first-party module:

- module ID: `nvr`;
- name: `Cameras / NVR`;
- API namespace: `nvr`;
- Web route: `/modules/nvr`;
- capability provided: `camera.nvr`;
- host requirement: Linux;
- package/runtime requirements for the first implementation: FFmpeg/FFprobe;
- lifecycle: backup / restore;
- module can be disabled independently without disabling Core.

The module contributes a single main navigation entry **Cameras / Камеры** only while enabled.

## 4. Camera entity

A camera is a stable Home-AI resource independent of its current Linux process or RTSP connection.

Minimum persistent camera model:

```text
Camera
  id
  name
  enabled
  source_type           rtsp | onvif
  host/address
  credential_ref
  main_stream
  sub_stream
  transport             tcp | udp (tcp default)
  recording_mode        off | continuous | motion
  audio_enabled
  created_at
  updated_at
```

Stream profile metadata:

```text
StreamProfile
  uri / resolved source
  codec
  width
  height
  fps
  bitrate
  role                   main | sub
```

Passwords/tokens must never be returned by camera APIs or written to audit/event payloads. Persistent camera configuration stores only a credential reference. A dedicated server-side secret persistence mechanism is a prerequisite before camera passwords are stored.

## 5. Camera onboarding

### Manual RTSP

The user can enter:

- display name;
- RTSP/RTSPS endpoint;
- username/password if required;
- optional explicit substream URL.

Before save, Home-AI performs **Test connection** and reports:

- reachable/unreachable;
- authentication failure;
- codec;
- resolution;
- FPS when detectable;
- audio availability;
- estimated bitrate;
- timeout/network/TLS/source error.

### ONVIF discovery

Discovery is explicit and bounded to local interfaces.

Flow:

```text
Discover
 -> list devices
 -> select camera
 -> credentials
 -> query profiles
 -> identify main/sub streams
 -> test
 -> save
```

Discovery does not auto-enrol unknown cameras.

ONVIF credentials are treated as secrets.

## 6. Main stream / substream policy

Where a camera provides both:

- **main stream** is preferred for archive and evidence;
- **substream** is preferred for grid preview, basic motion and later object detection.

This prevents unnecessary decode/GPU/CPU load.

If only one stream exists, the camera still works with reduced efficiency.

Home-AI must not transcode archive video unless required for compatibility. The preferred recording path is packet copy/remux of the source H.264/H.265 stream.

## 7. Media runtime

### Process model

Each enabled camera has a supervised runtime state:

```text
disabled
connecting
online
degraded
offline
error
```

A camera worker owns reconnect/backoff and source health.

The first implementation may use managed FFmpeg/FFprobe processes behind a typed Go runtime abstraction. External command construction is owned by the NVR module; arbitrary user-supplied command lines are forbidden.

### Reconnect

RTSP loss must not require user action.

Backoff:

- fast retries immediately after a short interruption;
- bounded exponential backoff during longer outages;
- reset after a stable connection.

Camera offline/online transitions become low-rate domain events.

### Crash/restart

After Core/NVR restart:

- enabled cameras are reconstructed from persistent configuration;
- recording resumes automatically;
- incomplete current segments are recovered or discarded safely;
- historical archive remains indexed.

## 8. Live view

NVR v1 supports:

- single camera live view;
- multi-camera grid;
- main/sub quality selection;
- fullscreen;
- online/offline state;
- mute/unmute when audio exists.

Browser live transport is separate from archive recording.

The implementation should use a media gateway abstraction so WebRTC/MSE/HLS/restream technology can be replaced without changing camera/archive APIs.

The camera must not be opened independently by every browser when a shared local restream can avoid excessive connections to the physical device.

## 9. Archive format

### Segment model

Archive is split into immutable time-bounded segments.

Initial target:

- approximately 60-second recording segments;
- source timestamps normalized to UTC in metadata;
- native stream packet-copy/remux whenever possible;
- media files stored outside SQLite;
- SQLite stores metadata/index only.

Archive path is implementation detail and not a public API contract.

A segment record includes:

```text
RecordingSegment
  id
  camera_id
  storage_target_id
  start_at
  end_at
  path
  codec
  width
  height
  size_bytes
  protected
  health/status
```

### Why segments

Segments provide:

- bounded corruption after a crash;
- easy ring deletion;
- timeline indexing;
- efficient clip assembly;
- future relocation across nodes/storage.

## 10. Video storage

The NVR only uses mounted physical/logical storage explicitly assigned:

```text
purpose = video
```

This reuses ADR-0030 and never silently consumes a Files/NAS pool.

### NVR v1 storage target

The first version supports one active archive target selected from ready `video` storage.

The UI shows:

- device/filesystem;
- mountpoint;
- total;
- used by NVR;
- free;
- reserved;
- estimated retention.

Multiple active video targets and distributed archive placement are deferred.

### Reserve

NVR must preserve a configurable free-space reserve and never intentionally fill the filesystem to 100%.

### Ring retention

When the NVR archive limit/reserve boundary is reached:

1. select oldest unprotected segments;
2. delete media file;
3. remove/mark index record transactionally;
4. continue until reserve is restored.

Protected evidence is excluded.

If only protected recordings remain and the reserve cannot be restored, recording enters a visible degraded/error state rather than silently deleting protected evidence.

## 11. Recording modes

Per camera:

- `off`;
- `continuous`;
- `motion`.

### Continuous

Main stream is recorded continuously while camera and target storage are available.

### Motion

Basic motion uses the lower-cost stream when available.

NVR v1 supports:

- pre-record buffer;
- post-record buffer;
- sensitivity/threshold;
- minimum event duration;
- cooldown/merge window.

Target defaults may be tuned during live acceptance rather than becoming an immutable API contract.

Motion does not require AI/object classification.

## 12. Timeline and events

The archive UI is timeline-first, not filesystem-first.

Initial event classes:

- recording available;
- motion;
- camera offline;
- camera online;
- manual bookmark/protected clip.

Timeline requirements:

- day/hour navigation;
- zoom;
- scrub to time;
- visible gaps;
- event markers;
- jump to event;
- open protected clip.

Event metadata is stored in SQLite.

Media frames are never placed in durable Core events.

## 13. Review/event grouping

Raw motion bursts close in time should merge into one review event rather than producing hundreds of alerts.

Initial review record:

```text
ReviewEvent
  id
  camera_id
  start_at
  end_at
  severity
  kind
  thumbnail_ref
  acknowledged_at
```

Object labels/person/vehicle metadata are added later without changing the base review identity.

## 14. Evidence and export

NVR v1 supports:

- protect/unprotect a time range;
- bookmark/note;
- snapshot;
- export clip.

Protection pins all archive segments required by the selected range.

Protected evidence is never deleted by ring retention.

Export jobs use the existing typed Job Engine because clip assembly can outlive one HTTP request.

Future evidence manifest/signature support is reserved but not required for first v1.

## 15. Permissions

Initial permission catalog:

```text
camera.list
camera.live
camera.archive
camera.export
camera.ptz
camera.manage
nvr.storage.manage
nvr.settings.manage
```

Camera operations use exact `resource_type=camera` grants where applicable.

Examples:

- child: live view of one camera;
- parent: live + archive for selected cameras;
- administrator: global camera/NVR permissions.

The Web UI must not display inaccessible cameras.

Direct API access enforces the same authorization server-side.

## 16. Audit and domain events

Audit includes management/security-sensitive operations:

- camera.create/update/delete;
- credentials replaced;
- recording mode changed;
- archive target changed;
- retention changed;
- evidence protected/unprotected;
- clip exported;
- PTZ action when implemented.

Audit never contains passwords, full credential-bearing URLs or media payloads.

Low-rate durable events may include:

```text
nvr.camera.online
nvr.camera.offline
nvr.recording.started
nvr.recording.stopped
nvr.motion.started
nvr.motion.ended
nvr.storage.warning
nvr.storage.full
nvr.evidence.protected
```

High-rate frames/detection telemetry stay outside the generic durable event bus.

## 17. Health and diagnostics

Per camera the UI/API exposes:

- runtime state;
- last frame/packet time;
- codec/resolution;
- FPS when known;
- bitrate when known;
- reconnect count;
- current recording state;
- last recording segment;
- archive bytes;
- latest error code/safe message.

NVR summary exposes:

- cameras online / total;
- cameras recording;
- archive target health;
- NVR archive usage;
- estimated retention;
- active review events.

## 18. AI/vision extension boundary

NVR v1 does not require object AI.

The architecture reserves a separate vision pipeline:

```text
substream/frame sampler
       |
       v
motion gate
       |
       v
object detector
       |
       +--> person / vehicle / animal / package
       |
       v
optional enrichers
  - face
  - LPR
  - embeddings
```

Expensive inference is not run on every full-resolution archive frame.

Future inference backends may run:

- locally on CPU;
- NVIDIA GPU;
- Intel/OpenVINO;
- Coral/NPU;
- another Home-AI cluster node.

The detector contract must not be tied to one vendor.

## 19. Future zones and rules

After the stable v1 archive:

- named polygon zones;
- exclusion masks;
- tripwires;
- direction;
- dwell time;
- per-zone event rules;
- object filters;
- Smart Home correlations.

This follows the useful concepts from mature NVR/VMS products without coupling capture to analytics.

## 20. AI Agent integration

The NVR remains authoritative and fully usable without the AI Agent.

Later typed read tools:

```text
nvr.cameras.list
nvr.camera.status
nvr.events.search
nvr.timeline.search
nvr.snapshot.get
nvr.clip.get
```

Change/sensitive operations such as PTZ presets, retention changes or deleting evidence remain subject to normal permission/sensitivity/approval rules.

AI tools execute with the requesting user's exact camera permissions.

Semantic search and natural-language video investigation are later enrichment layers over the NVR index, not replacements for it.

## 21. Smart Home integration

The future Smart Home module may correlate domain events:

- door opened -> mark camera timeline;
- alarm triggered -> protect adjacent camera interval;
- person detected at night -> automation may turn on light;
- smoke/alarm -> preserve related recordings.

Neither module directly bypasses Core permissions/policy.

## 22. Cluster readiness

The first implementation is single-node but IDs and archive metadata must not assume that forever.

Future worker placement may split:

- camera ingest;
- archive writer;
- live restream;
- vision inference;
- archive storage.

Future records can gain `node_id` / placement fields without changing camera identity.

No distributed scheduler is part of NVR v1.

## 23. Exact Home-AI NVR v1 scope

A version is considered **NVR v1 usable** only when all of the following work end-to-end:

1. first-party `nvr` module can be enabled/disabled;
2. camera permission catalog is registered;
3. manual RTSP camera add/edit/delete;
4. bounded ONVIF discovery and import;
5. connection test before save;
6. protected camera credentials server-side;
7. main/sub stream profile detection;
8. single live view;
9. multi-camera grid;
10. camera health and automatic reconnect;
11. select one ready `video` storage target;
12. continuous recording using packet-copy/remux where possible;
13. basic motion recording;
14. pre/post event buffer;
15. segmented archive;
16. capacity reserve and ring overwrite;
17. protected recordings never overwritten;
18. timeline with gaps/recording/motion markers;
19. review event grouping for motion bursts;
20. snapshot;
21. protect/unprotect range;
22. export clip as a persistent job;
23. per-camera live/archive/export/manage authorization;
24. management audit without secret/media leakage;
25. restart recovery: cameras and recording resume automatically.

## 24. Explicitly deferred after NVR v1

Not required to call the first stable archive usable:

- object detection;
- person/vehicle classification;
- zones/tripwires;
- face recognition;
- known-person database;
- license plate recognition;
- semantic/embedding search;
- natural-language AI Agent investigation;
- advanced PTZ/presets/tours;
- mobile push notifications;
- incident/case manager;
- multiple archive targets;
- archive replication;
- multi-node capture/recording/failover;
- cloud camera services.

These are planned on top of the stable recording/archive foundation.

## 25. Implementation slices

### NVR-0 — Contracts and persistence

- module manifest/runtime;
- permissions;
- camera/archive/event schema;
- API types;
- secret-reference contract;
- tests.

### NVR-1 — Camera onboarding and live

- RTSP test/probe;
- manual camera CRUD;
- ONVIF discovery/import;
- runtime supervisor;
- main/sub stream;
- single/grid live;
- reconnect/health.

### NVR-2 — Recording and storage

- select `video` target;
- archive directories;
- segment recording;
- startup recovery;
- reserve/ring retention;
- archive health/usage.

### NVR-3 — Timeline and event recording

- basic motion;
- pre/post buffer;
- motion grouping;
- archive timeline;
- protect/bookmark/snapshot;
- clip export job.

### NVR-4 — Hardening and live acceptance

- camera permission UI;
- failure/restart tests;
- storage-full tests;
- long-running soak;
- multi-camera load tests;
- update/rollback compatibility.

### NVR-5 — Vision foundation (after v1)

- detector interface;
- object events;
- zones/tripwires;
- accelerator selection;
- AI Agent read/search tools.

## 26. Acceptance priorities

The order of importance is:

1. never silently lose or overwrite protected evidence;
2. recording recovers automatically after camera/Core interruption;
3. storage cannot be filled uncontrollably;
4. permissions cannot be bypassed through live/archive/export endpoints;
5. credentials never leak through Web/audit/events/logs;
6. timeline accurately represents available archive;
7. live view remains responsive;
8. analytics may fail independently without stopping recording.
