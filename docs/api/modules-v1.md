# Module Registry API v1

The API version remains v1 while the active external module manifest is **Manifest v2**.

## List modules

```text
GET /api/v1/modules
```

Requires `modules.read`.

## Read one module

```text
GET /api/v1/modules/{module_id}
```

Requires `modules.read`.

## Discover capabilities

```text
GET /api/v1/modules/capabilities
```

Requires `modules.read`.

The capability set may include `host.docker` when the Docker Engine socket is present, along with generic host architecture/system capabilities.

## Install a module

```text
POST /api/v1/modules/install
Content-Type: application/json
```

Requires `modules.manage` and CSRF protection for cookie-authenticated requests.

The body is a complete Manifest v2 object. Core validates it and returns `202 Accepted` with a Job record. The Job Engine performs the Docker pull/create/start operation through the privileged helper.

Images must be immutable digest references.

## Control a module

```text
POST /api/v1/modules/{module_id}/control
Content-Type: application/json
```

Body:

```json
{"operation":"enable"}
```

Allowed operations:

- `enable`
- `disable`
- `restart`

The endpoint returns a queued Job.

## Remove a module

```text
DELETE /api/v1/modules/{module_id}
```

Requires `modules.manage` and CSRF protection.

Removal deletes the managed container and registry entry. Persistent module data under `/var/lib/home-ai-core/modules/<module-id>` is intentionally preserved.

## Registry status

Persisted statuses are:

- `registered`
- `enabled`
- `disabled`
- `error`

All lifecycle mutations are auditable and run through the Job Engine.
