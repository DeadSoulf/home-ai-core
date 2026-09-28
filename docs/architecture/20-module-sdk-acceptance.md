# Module SDK Phase Acceptance Criteria

## Manifest contract

- [x] manifest schema version 1 exists
- [x] canonical JSON Schema exists
- [x] unknown fields are rejected
- [x] stable module IDs are validated
- [x] semantic module versions are validated
- [x] Core compatibility constraints are evaluated
- [x] dependency version constraints are evaluated
- [x] duplicate declarations are rejected

## Dependency and conflict planning

- [x] dependency-first install planning
- [x] missing dependencies are rejected
- [x] incompatible dependency versions are rejected
- [x] dependency cycles are rejected
- [x] conflicts are checked in both directions
- [x] unsupported architectures are rejected
- [x] required capabilities are checked before execution
- [x] planning performs no host mutation

## Permissions and capabilities

- [x] module permission requirements are declarative
- [x] permission declarations do not imply grants
- [x] host capability discovery exists
- [x] registered modules can provide capabilities
- [x] disabled/error modules do not provide capabilities
- [x] `modules.read` and `modules.manage` permissions exist

## Lifecycle contract

- [x] typed install contract
- [x] typed upgrade contract
- [x] typed remove contract
- [x] typed backup contract
- [x] typed restore contract
- [x] lifecycle operation support is declared in the manifest
- [x] manifests cannot contain arbitrary lifecycle shell commands
- [x] reference/demo module implements the contract

## API, events and UI

- [x] module API namespace declaration
- [x] module publish events stay in module namespace
- [x] wildcard event subscriptions are supported
- [x] UI navigation registration is declarative
- [x] UI routes stay under the module route namespace
- [x] manifest-provided executable frontend code is not part of v1

## Persistent registry

- [x] module registry state is persisted in SQLite
- [x] registration refreshes manifest/version metadata
- [x] registration does not reset persistent enabled/disabled/error status
- [x] read-only module list API exists
- [x] read-only module detail API exists
- [x] capability discovery API exists

## Security and scope

- [x] module registry API requires `modules.read`
- [x] no package download/install endpoint exists in Phase 7
- [x] no privileged host mutation is introduced in Phase 7
- [x] signed package distribution remains Phase 9
- [x] module permission grants remain explicit future security work

## Validation

- [x] manifest validation tests
- [x] wildcard subscription tests
- [x] publish namespace isolation test
- [x] dependency ordering test
- [x] cycle detection test
- [x] bidirectional conflict test
- [x] capability aggregation test
- [x] persistent registry test
- [x] registry status preservation test
- [x] module API contract test
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] daemon smoke test
- [x] linux/amd64 build
- [x] linux/arm64 build
