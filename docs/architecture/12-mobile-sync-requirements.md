# Client File Transfer Requirements

> Historical filename retained for link stability. The clarified priority is Windows file upload first; Android follows later.

## Product requirement

Home-AI needs first-party clients that transfer user-owned files into the user's authorized Home-AI storage.

This is not a full operating-system image-backup system.

## Windows client — first priority

Required direction:

- authenticated device enrollment;
- select files/folders to copy;
- select an allowed destination;
- private and shared Home-AI folders;
- resumable/chunked transfers;
- retry after connectivity loss;
- integrity verification;
- duplicate/conflict handling;
- optional scheduled/automatic copy;
- local-network operation;
- remote operation through the approved secure access path;
- per-device revocation.

The Windows client must not bypass Home-AI user/folder permissions.

## Android — later

The future Android client may add:

- file upload;
- photo/video upload;
- notifications;
- Home-AI status/control;
- voice-related functions;
- selective offline access.

Android implementation should wait until the server APIs used by Windows/file storage are stable.

## Transport and security

- remote transfer uses the normal authenticated Home-AI API through the secure remote-access path;
- SMB/NFS must not be exposed directly to the public Internet;
- device credentials can be revoked independently;
- transfer integrity is verified;
- server-side authorization decides the destination scope.

## Server-side data model

Transferred objects should support enough metadata for future search/sync:

- stable ID;
- owner;
- folder/share;
- original filename;
- content hash;
- size;
- MIME type;
- timestamps;
- device/source identity;
- storage location;
- optional metadata.

The storage implementation must remain compatible with backup/recovery and future AI-authorized search.
