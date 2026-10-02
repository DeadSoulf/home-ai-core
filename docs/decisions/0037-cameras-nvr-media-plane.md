# ADR-0037: Cameras / NVR control-plane and media-plane architecture

- Status: Accepted for implementation
- Date: 2026-10-02

## Context

Home-AI product vision requires a local NVR with RTSP/ONVIF cameras, live view, continuous/event recording, local archive/timeline, retention, object/face intelligence and later AI Agent search.

The existing Core architecture already provides:

- one household identity system;
- global and exact resource-scoped permissions;
- persistent jobs;
- durable low-rate domain events;
- audit;
- module lifecycle;
- physical storage purpose assignment including `video`.

The generic durable event bus explicitly is not intended for high-rate media/frame traffic. The storage model also intentionally separates `files` and `video` capacity.

A camera implementation must therefore avoid turning Core/SQLite/WebSocket into a video transport while still reusing Core security and lifecycle.

## Decision

Implement Cameras / NVR as a first-party `nvr` module with a strict control-plane/media-plane split.

### Control plane

Home-AI Core and the NVR domain own:

- camera identity/configuration;
- credential references;
- permissions;
- runtime state;
- archive metadata/index;
- review/motion event metadata;
- storage target policy;
- retention policy;
- evidence protection;
- export jobs;
- audit and low-rate domain events.

SQLite stores metadata only, not camera frames/video blobs.

### Media plane

A dedicated NVR runtime owns:

- RTSP ingest;
- reconnect/backoff;
- live restream;
- recording segmentation;
- clip/snapshot production;
- later frame sampling for vision.

Media files are written directly to storage assigned `purpose=video`.

Media frames and high-rate detection telemetry do not use the generic durable event bus.

### Recording independence

Recording must continue when:

- object detection is unavailable;
- vision accelerators fail;
- AI Agent is disabled;
- Internet/cloud AI is unavailable.

Analytics is an optional downstream consumer of camera frames/events.

### Storage

NVR v1 selects one ready mounted `video` storage target.

Archive data is immutable time-bounded segments. Ring retention deletes the oldest unprotected segments only and preserves a free-space reserve.

Protected evidence is never silently deleted by retention.

### Authorization

Cameras are resource-scoped objects using `resource_type=camera`.

Live, archive, export, PTZ and management endpoints enforce the requesting user's effective Core permissions.

NVR has no separate account database.

### Credentials

Camera/API credentials are secrets.

Persistent camera rows contain only a credential reference. Secrets never appear in API responses, audit metadata, durable event payloads or normal logs.

A server-side secret persistence mechanism must exist before credential-bearing camera configuration is considered production-ready.

### First implementation technology boundary

The first media implementation may use managed FFmpeg/FFprobe processes behind typed Go interfaces.

Arbitrary shell command input is not part of the public/module contract.

Live transport is behind an abstraction so WebRTC/MSE/HLS/restream implementation can evolve independently from camera/archive APIs.

### Future AI

Object detection, face recognition, LPR, embeddings and semantic search are separate vision/enrichment layers.

The base NVR does not depend on a specific accelerator or model vendor.

## Alternatives considered

### Put video into Core HTTP/WebSocket/event storage

Rejected. It would mix high-rate media with the trusted control plane and violate the existing event-system boundary.

### Make AI/object detection mandatory for event recording

Rejected. Recording reliability must not depend on inference.

### Use NAS `files` pools for camera archive

Rejected. ADR-0030 already defines explicit `video` capacity and prevents implicit competition between user files and NVR archive.

### Give the NVR its own users

Rejected. Home-AI has a unified household identity/access model and camera permissions fit the existing resource-scoped design.

### Build archive as large monolithic files

Rejected for the first implementation. Time-bounded segments provide better crash recovery, ring retention, timeline indexing and future relocation.

## Consequences

### Positive

- NVR reuses trusted Core identity/permissions/audit without routing media through Core databases;
- local recording continues independently of AI/cloud;
- archive capacity is isolated from Files/NAS;
- segment metadata is suitable for timeline, retention and future cluster placement;
- per-camera grants naturally fit existing authorization;
- AI Agent can later consume explicit NVR tools without becoming the recording runtime.

### Negative

- the product needs a supervised media runtime in addition to normal Core request handling;
- camera credential secret storage becomes a required prerequisite;
- browser live streaming requires a dedicated media transport;
- retention/index/filesystem consistency requires recovery logic and soak testing.

## Detailed architecture

See [../NVR_ARCHITECTURE.md](../NVR_ARCHITECTURE.md).
