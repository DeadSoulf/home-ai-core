# ADR-0030: Storage purpose assignment for Files and Video

## Status

Accepted for implementation, 2026-09-30.

## Context

Home-AI already distinguishes physical storage devices from logical NAS pools and folders, but the system had no persistent way to say what a non-system partition or logical volume is intended to serve. The Files page therefore had to infer candidates from all mounted filesystems under `/mnt/home-ai-core`, while the future NVR/video subsystem also needs dedicated capacity that must not be mixed implicitly with general files.

Users also need to assign a purpose while creating a partition and to change the purpose later for existing storage.

## Decision

Home-AI stores an explicit application purpose on physical data partitions/logical volumes. The initial allowed purposes are `files` and `video`.

The assignment is stored independently from NAS pools/folders. It records the current device path and, when available, the filesystem UUID. UUID matching takes precedence so an assignment survives Linux device-path renumbering. The path remains as a fallback for new/unformatted partitions that do not yet have a filesystem UUID.

System storage cannot receive an application purpose. Only partition/LVM data devices are assignable.

The Storage Web UI exposes the purpose when creating a partition and on existing non-system partitions/logical volumes. Existing assignments can be changed or cleared.

The Files page uses only `files` assignments as candidate capacity for new file pools. It also shows assigned devices before they are ready, including missing, unformatted, unmounted, ready and already-backed-by-pool states. `video` capacity is intentionally excluded from file-pool selection.

Partition deletion clears the related assignment. Formatting preserves the purpose and refreshes the stored filesystem UUID after the new filesystem appears.

## Consequences

Physical capacity routing becomes explicit and reusable by future modules. NAS pool semantics remain focused on logical file data and permissions instead of becoming a generic media-purpose registry.

A partition created with a purpose can still require formatting and mounting before it becomes usable by a file pool. The UI must make those intermediate states visible rather than silently hiding the device.

## Deferred

- automatic formatting/mounting as one guided storage workflow;
- quotas/reservations shared between purposes;
- multiple simultaneous purposes on one partition;
- NVR retention policies and ring-overwrite behavior for `video` storage.
