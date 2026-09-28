# ADR-0003: Privilege Separation

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core exposes a network-accessible API while also needing to perform privileged Linux operations such as:

- package installation and removal
- service management
- storage preparation and mounting
- network configuration
- device access
- module installation and upgrade
- selected filesystem ownership/permission changes

Running the entire Core API as root would turn any API or parser vulnerability into unrestricted host compromise.

At the same time, relying on arbitrary `sudo` or shell commands from Web/API code would create an unsafe and difficult-to-audit execution path.

## Decision

Home-AI-Core uses a split-process privilege model.

```text
Browser / Mobile / Node
          |
          v
   home-ai-core
   unprivileged
          |
     Auth + Policy
          |
     typed local IPC
          |
          v
   home-ai-privd
   privileged helper
          |
          v
        Linux
```

### 1. Public Core process

`home-ai-core`:

- runs as a dedicated unprivileged system user
- owns the public API and realtime endpoints
- performs authentication and authorization
- manages jobs/events/module metadata
- must not have a generic path to execute commands as root
- must not use `sudo`
- must not accept shell command text from clients or modules

### 2. Privileged helper

`home-ai-privd`:

- runs as root only because some platform operations require it
- has no TCP/UDP listening socket
- is reachable only through a local Unix-domain IPC endpoint
- accepts only predefined typed operations
- rejects unknown operations and unvalidated arguments
- does not expose a generic `exec(command_string)` API
- keeps its implementation intentionally small
- returns structured results and error codes

Conceptual operations:

```text
service.start(name)
service.stop(name)
package.install(package_set)
package.remove(package_set)
filesystem.mount(spec)
filesystem.unmount(id)
network.apply(plan)
module.stage(package)
module.activate(id)
```

The real operation set is versioned and narrower than the public Core API.

### 3. IPC boundary

Initial implementation uses a Unix-domain socket under:

```text
/run/home-ai-core/priv.sock
```

The socket is not accessible to normal users or module service accounts.

The helper validates the local peer identity using OS-provided Unix peer credentials in addition to filesystem permissions.

IPC messages are structured and versioned. Arbitrary shell fragments are never part of the protocol.

### 4. Execution rules

When an operation ultimately requires launching an operating-system tool:

- the executable path is selected by trusted code
- arguments are passed as an argument vector, never through a shell
- environment variables are explicitly controlled
- stdin is closed unless the operation contract requires it
- output has size/time limits
- exit status and structured diagnostics are captured
- user-provided values are validated against the operation schema

No equivalent of:

```text
/bin/sh -c "<client supplied text>"
```

is allowed in privileged code.

### 5. Module boundary

Modules do not receive direct access to `home-ai-privd`.

A module requests an operation through its scoped Core/Module API. The Core checks:

- module identity
- granted permissions
- user/actor authorization
- resource scope
- operation policy

Only then can the trusted Core translate the request into a privileged IPC operation.

### 6. AI boundary

AI is never a privileged local process merely because it is part of Home-AI-Core.

AI uses the same authenticated capability APIs as other actors.

Sensitive operations may require explicit human approval even when the AI actor otherwise has access to the capability.

### 7. systemd supervision

Both processes are systemd-managed services.

The unprivileged Core unit should use strong service hardening where compatible, including principles such as:

- no privilege escalation
- dedicated runtime/state directories
- restricted writable paths
- private temporary storage
- restricted device/filesystem access
- explicit network requirements

The privileged helper receives only the privileges required by its operation set. Where Linux capabilities can replace full root authority safely, they should be evaluated per operation rather than granted broadly.

### 8. Auditing

Every privileged request has:

- request/job ID
- actor identity
- module/capability identity where relevant
- operation type
- target resource
- policy decision
- start/end timestamp
- result

Sensitive values and secrets are redacted.

Audit records are generated at the Core policy boundary; privileged execution also emits internal completion records so failures cannot silently disappear.

## Security invariants

The following are architectural invariants:

1. The network-facing Core process does not run as root.
2. The privileged helper does not listen on the network.
3. No generic remote shell/command execution primitive exists in the privileged IPC contract.
4. Modules cannot directly invoke the privileged helper.
5. AI cannot bypass Core authorization.
6. Privileged operations are typed, validated and audited.
7. Unknown IPC protocol versions or operations fail closed.

## Consequences

### Positive

- Remote API compromise does not automatically provide a root shell.
- The root attack surface is much smaller than the full Web/API process.
- Privileged behaviour can be unit/integration tested as a finite operation set.
- Module and AI permissions remain enforceable at one control-plane boundary.
- Audit trails map user/module intent to actual host mutation.

### Negative

- More process and IPC complexity than a single root daemon.
- New privileged operations require explicit protocol additions.
- Some Linux operations may need carefully designed transactional/rollback behaviour.
- Tests must cover both policy decisions and executor-side validation.

## Rejected alternatives

### Run the entire Core as root

Rejected because the public API, parsers, WebSocket handling and module metadata processing do not need root and would unnecessarily expand the privileged attack surface.

### Use sudo for privileged commands

Rejected as the primary architecture because it encourages command-oriented privilege escalation and weakens typed operation contracts.

### Let each module run privileged code directly

Rejected because permissions, auditing and security policy would become fragmented across modules.

### Put privileged operations in the browser/Web UI

Rejected categorically. The frontend is an untrusted API client.
