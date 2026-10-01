# AI Agent Foundation

**Status:** active foundation plan  
**Started:** 2026-10-01  
**Target first implementation slice:** `0.1.115-dev`

## Goal

Start the Home-AI AI Agent now so it evolves together with NAS, Smart Home, NVR, Voice and cluster capabilities instead of being attached after those domains are already fixed.

The AI Agent is a **first-party Home-AI module**, not privileged logic embedded directly into Core.

Core remains the trusted control plane for identity, effective permissions, jobs, events, audit, module discovery and update trust.

## Architecture

```text
User / later Voice
        |
        v
   AI Agent module
        |
        +--> Model provider adapter
        |      |- local model runtime
        |      \- optional external provider
        |
        +--> Tool Registry
               |
               +--> Core read tools
               +--> Files/NAS tools
               +--> Smart Home tools
               +--> NVR tools
               \--> future cluster tools
```

The agent never receives an unrestricted root shell.

## Core boundary

Core owns:

- user/session identity;
- effective permissions and resource scopes;
- module registry/capabilities;
- Jobs and Events;
- Audit;
- Web/API security boundaries;
- update/package trust.

The AI Agent module owns:

- conversations/sessions;
- model-provider adapters;
- prompt/context assembly;
- tool selection/orchestration;
- approval state and explanation;
- AI-specific memory/preferences;
- redaction rules for provider context.

Domain modules own their own data and actions and expose only explicit AI tools.

## Tool contract

Every AI-callable tool must have a stable descriptor:

- module ID and tool ID;
- human-readable purpose;
- JSON input schema;
- required Core permissions;
- optional resource scope;
- side-effect class: `read`, `change`, `sensitive`;
- whether explicit approval is required;
- bounded timeout;
- auditable execution result.

The agent must re-check the current user's effective permissions immediately before tool execution.

Tools are typed capabilities. Arbitrary shell commands are not tools.

## Approval policy

Initial policy:

- `read`: may run automatically when the user has permission;
- `change`: agent prepares the action and asks for approval by default;
- `sensitive`: always requires explicit approval from an authorized user.

Examples of sensitive actions include deleting files, changing users/security, storage destruction, network exposure and privileged software changes.

An AI response can never grant new permissions to itself.

## Model providers

The model layer must be replaceable.

Initial contract supports:

- local provider adapters as the preferred path;
- optional external providers only when explicitly configured;
- streaming text responses;
- bounded request/response sizes;
- provider health/capability discovery;
- secrets outside logs/audit payloads;
- explicit policy for what context may leave the home.

No single model vendor becomes a Core dependency.

## First implementation slices

### 0.1.115-dev — Agent contracts

- first-party `ai.agent` module identity/capability;
- typed Tool Registry;
- permission/sensitivity metadata;
- provider adapter interface;
- deterministic test provider;
- read-only Core tools for system status, jobs and module inventory;
- audit events for AI session/tool execution;
- API foundation and tests.

### Next slice — Local conversation

- local model-provider adapter;
- persistent AI sessions;
- streaming Web chat;
- context limits and cancellation;
- health/status UI.

### Next slice — Approval and controlled actions

- proposal/approval flow;
- permission re-check at execution time;
- first controlled write tools;
- explicit sensitive-action confirmation;
- complete audit trail.

### Co-development with domains

As new modules arrive they add tools rather than special AI code:

- Smart Home: devices, entities, scenes and automations;
- Files/NAS: authorized search, metadata and file operations;
- NVR: cameras, events and archive search;
- Cluster: node/resource diagnostics and workload proposals.

## Non-goals for the first slice

- autonomous code modification;
- unrestricted terminal/root access;
- silent configuration changes;
- permanent long-term semantic memory;
- mandatory cloud AI;
- Smart Home/NVR business logic inside the AI module.

## Acceptance for the first slice

The foundation is accepted when:

1. AI module can register and expose a provider and tools;
2. a test request can execute a read-only tool through the registry;
3. effective user permissions are enforced;
4. denied tools fail closed;
5. tool calls and outcomes appear in Audit;
6. cancellation/timeouts are bounded;
7. no provider or tool path bypasses Core authorization.
