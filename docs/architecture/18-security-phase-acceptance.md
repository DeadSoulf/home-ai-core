# Identity & Security Acceptance Criteria

## Identity and passwords

- [x] users are persistent Core state
- [x] first owner is created through one-time local bootstrap
- [x] passwords use Argon2id
- [x] password salts are random
- [x] plaintext passwords are never persisted
- [x] username normalization/validation is defined

## Sessions

- [x] session tokens use 256-bit randomness
- [x] only token hashes are stored
- [x] sessions expire
- [x] sessions can be revoked
- [x] browser cookie transport is HttpOnly and SameSite=Strict
- [x] explicit bearer-token mode exists for API/CLI clients
- [x] cookie mutations use a separate CSRF token

## Authorization

- [x] roles and permissions are persistent
- [x] owner permissions are explicit
- [x] API authorization is deny-by-default
- [x] system endpoint requires `system.read`
- [x] realtime endpoint requires `events.read`
- [x] audit endpoint requires `audit.read`

## Audit

- [x] bootstrap is audited
- [x] login success/failure is audited
- [x] logout is audited
- [x] credential/session secrets are excluded from audit metadata

## Bootstrap

- [x] bootstrap token is a local 256-bit secret
- [x] token file is mode 0600
- [x] bootstrap API is loopback-only
- [x] bootstrap is disabled after the first owner exists
- [x] bootstrap token is removed after initialization

## Deferred

- user-management UI/API beyond first owner
- MFA/passkeys
- device identities
- module/app/node/AI credentials
- remote TLS gateway
- distributed identity for multi-node clusters
- login throttling for externally reachable deployments
