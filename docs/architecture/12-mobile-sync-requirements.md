# Mobile Sync Requirements

The future Home-AI mobile client is treated as a first-class platform client rather than a special filesystem hack.

## Required capabilities

- authenticated device registration
- background photo/video upload
- document upload
- resumable/chunked transfers
- retry after connectivity loss
- integrity verification
- duplicate detection
- selective synchronization
- remote and local-network operation
- notification support
- per-user and per-device permissions

## Server-side boundary

Mobile clients communicate with a documented sync/media API.

They must not require SMB/NFS exposure to the public Internet.

## Data model considerations

Uploaded objects should support:

- stable ID
- owner
- original filename
- content hash
- size
- MIME type
- creation/capture timestamp
- device/source identity
- storage location
- metadata
- optional album/folder references

## Security

- TLS is mandatory for remote traffic.
- Device tokens can be revoked independently.
- Credentials are stored using mobile platform secure storage.
- Remote access should prefer a secure tunnel or authenticated gateway.
- Server-side encryption and client-side encryption may be added as separate capabilities, but encryption design must not prevent backup and recovery.
