# ADR-0002: Core and Web Technology Stack

- Status: Accepted
- Date: 2026-09-28

## Context

Home-AI-Core is a long-running home infrastructure control plane. It must run continuously on Debian 13, expose HTTP and realtime APIs, manage concurrent jobs and events, support x86-64 and ARM64, remain easy to deploy and update, and later coordinate multiple nodes.

The Core is security-sensitive and should have a small operational footprint. AI/ML workloads, video processing and user applications are separate workloads and do not need to use the same implementation language as the Core.

The primary Core candidates considered were Go, Rust, C++ and Python.

## Decision

### Core control plane: Go

The Home-AI-Core daemon and first-party control-plane services will be implemented in Go.

Development starts on the current supported Go 1.27 line. The exact patch version is pinned by project tooling and may be updated through normal dependency/toolchain maintenance.

Go is selected because it provides:

- simple deployment as self-contained executables
- strong support for Linux servers on amd64 and arm64
- a mature standard networking and HTTP stack
- lightweight concurrency suitable for jobs, events, WebSockets and multi-node coordination
- predictable memory management for a general-purpose always-on control plane
- straightforward cross-compilation and CI
- a comparatively small operational dependency surface
- good maintainability for infrastructure code

### Web UI: TypeScript

The browser application will be implemented in TypeScript.

The initial UI stack will use React with a modern build toolchain. Browser code is treated as an untrusted client of the Core API and never performs privileged host actions directly.

The exact frontend package versions are pinned in the lockfile rather than in this ADR.

### API contracts

External and module-facing contracts are language-independent.

Contracts are defined through:

- versioned HTTP/JSON APIs
- WebSocket/realtime event contracts
- versioned JSON Schemas where appropriate

No module is required to be written in Go merely because the Core is written in Go.

### AI and ML workloads

Python is the preferred language for AI/ML services when ecosystem support makes it appropriate.

AI services communicate with the Core through authenticated APIs and capability-scoped tool contracts. Python code is not embedded into the trusted Core process.

### Performance- or safety-critical native components

Rust may be used for isolated components where memory safety, binary-level integration or performance justifies the additional complexity.

Such components must expose a narrow process/API boundary rather than being added casually to the Core.

### C++

C++ is not selected for the new Core. Existing historical C++ code remains archived and may be studied for behaviour, but it is not treated as the foundation of the restarted architecture.

C++ remains acceptable only where a specific external library or hardware integration creates a compelling requirement.

## Candidate comparison

| Criterion | Go | Rust | C++ | Python |
|---|---|---|---|---|
| 24/7 control-plane daemon | Strong | Strong | Strong | Good |
| Memory safety | Good (GC/runtime) | Excellent | Manual/conditional | Good (runtime managed) |
| Deployment simplicity | Excellent | Excellent | Good | Moderate |
| HTTP/network services | Excellent | Strong | Library-dependent | Excellent |
| Concurrency ergonomics | Excellent | Strong but complex | Library-dependent | Good |
| amd64/arm64 Linux | Excellent | Excellent | Excellent | Excellent |
| Build/cross-compile simplicity | Excellent | Good | Moderate | N/A/interpreter |
| AI ecosystem | Limited | Limited | Limited | Excellent |
| Low-level native integration | Good | Excellent | Excellent | Limited |
| Team/maintenance complexity | Low-Moderate | Moderate-High | High | Low |
| Fit for trusted Core | **Selected** | Secondary | Not selected | Not selected |

## Architecture consequence

The platform is intentionally polyglot:

```text
                    Home-AI-Core
                         |
              +----------+----------+
              |                     |
        Go Control Plane       TypeScript Web
              |
        Versioned APIs
              |
   +----------+-----------+----------------+
   |                      |                |
Go Modules          Rust helpers       Python AI
(if appropriate)    (when justified)   services
```

Language boundaries must coincide with documented service/module boundaries.

## Consequences

### Positive

- The Core can be shipped and supervised as a small number of native Linux executables.
- Web/API/concurrency work does not require a large external framework.
- Future multi-node communication can remain within the same control-plane ecosystem.
- AI development can use Python without making the trusted Core dependent on Python packaging.
- Performance-sensitive helpers can use Rust without forcing the entire project into Rust.

### Negative

- The project becomes intentionally polyglot.
- Go's garbage collector must be considered for latency-sensitive workloads; such workloads should normally live outside the Core anyway.
- Direct use of some Linux APIs may require dedicated libraries or small platform-specific adapters.
- TypeScript and Go contracts must be kept synchronized through generated or validated schemas.

## Rejected alternatives

### Rust for the whole Core

Rust provides stronger compile-time memory guarantees and is an excellent systems language. It was not selected as the default because Home-AI-Core's Core is primarily a networked control plane rather than a kernel-adjacent component, and development/maintenance complexity would be higher for the current goals.

Rust remains an approved implementation language for isolated components.

### C++ for the whole Core

C++ offers maximum native control but increases memory-safety and dependency-management risk for a security-sensitive web-connected daemon. The restarted project does not inherit the historical C++ choice.

### Python for the whole Core

Python is highly productive and is the preferred ecosystem for AI work, but the trusted 24/7 control plane should not depend on interpreter environments and application-level Python packaging for its base operation.

Python remains a first-class language for AI and data-processing services.
