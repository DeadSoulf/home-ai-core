# AI Agent Foundation

**Status:** controlled server tools/public web published; local inference tuning published; Web streaming temporarily disabled after live transport regression and pending compatibility rework  
**Started:** 2026-10-01  
**First implementation slice:** `0.1.115-dev` published

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

Published in `v0.1.115-dev` after full Core/Web, Windows and Debian CI/release checks.

- ✅ first-party `ai.agent` module identity/capability;
- ✅ Core startup registration through the existing Module Registry;
- ✅ typed Tool Registry;
- ✅ permission/sensitivity metadata;
- ✅ exact resource-scope authorization hook;
- ✅ `read / change / sensitive` approval boundary;
- ✅ provider adapter interface;
- ✅ deterministic test provider;
- ✅ contract/regression tests for allow/deny/scope/approval/cancellation;
- ✅ read-only Core tools for system status, jobs and module inventory (`0.1.117-dev`);
- ✅ audit events for AI tool execution (`ai.tool.execute`) without raw input payloads;
- ✅ authenticated AI status/tools/execute API foundation with permission filtering and bounded execution.

### 0.1.117-dev — Read-only Core tools + API

Published in `v0.1.117-dev` after successful PR/main Core/Web, Windows and Debian CI plus release workflow.

- ✅ `core.system.status` returns current Core/schema/system/hardware status;
- ✅ `core.jobs.list` exposes bounded recent job summaries without raw input/result payloads;
- ✅ `core.modules.list` exposes registered modules and capabilities;
- ✅ `/api/v1/ai/status` reports AI Agent readiness;
- ✅ `/api/v1/ai/tools` returns only tools allowed by the current user's effective permissions;
- ✅ `/api/v1/ai/tools/<tool>/execute` executes the permitted tool through the same Tool Registry contract;
- ✅ tool calls are bounded by server-side timeout and audited on success/deny/failure;
- ✅ cookie-session POSTs preserve Core CSRF enforcement;
- ✅ API/service regression tests cover discovery, authorization, execution, audit and payload redaction.

### 0.1.119-dev — Local conversation v1

Published in `v0.1.119-dev` after successful Core/Web, Windows, Debian and release workflows. Live validation with a configured local model remains pending.

- ✅ optional local Ollama provider adapter without mandatory provider dependency;
- ✅ persistent per-user conversations/messages in Core state migration 018;
- ✅ conversation ownership enforced at the state boundary;
- ✅ bounded message/context sizes and provider timeout;
- ✅ AI generation audit metadata without prompt/response text;
- ✅ authenticated conversation API;
- ✅ Web AI Agent chat page with provider/model status and RU/EN UI;
- ✅ chat never auto-executes Home-AI tools in this slice;
- 🚧 streaming transport remains next;
- 🚧 model-driven tool-call orchestration remains next.

### 0.1.120-dev — Module runtime control

- ✅ AI Agent is controllable from the standard **Modules** page.
- ✅ `modules.manage` separates module runtime control from read-only module visibility.
- ✅ `enabled / disabled` state persists across Core restart.
- ✅ Disable cancels active AI work and removes AI capabilities while leaving Core/NAS/other modules running.
- ✅ Restart recreates only the AI runtime context and cancels active AI requests.
- ✅ Runtime operations are audited and emitted as realtime module events.
- ✅ AI chat/tools fail closed while the agent is disabled.
- ✅ published in `v0.1.120-dev` after successful Core/Web, Windows, Debian and release workflows.
- 🧪 live enable/disable/restart acceptance on the installed Home-AI server remains pending.

### 0.1.121-dev — Restart persistence hotfix

- ✅ live failure in `0.1.120-dev` traced to the SQLite `modules.status` CHECK rejecting transient `restarting`;
- ✅ restart now persists only the durable `enabled` state and recreates the AI runtime in memory;
- ✅ persistent Module Registry rejects `restarting`;
- ✅ regression test exercises the restart API against the real SQLite-backed registry;
- ✅ published in `v0.1.121-dev` after successful Core/Web, Windows, Debian and release workflows.
- 🧪 live restart re-test on the installed Home-AI server remains pending.

### 0.1.122-dev — Finished conversation lifecycle

- ✅ Web chat has an explicit **Finish chat / Завершить чат** action.
- ✅ finished state is persisted as `closed_at` and survives Core restart.
- ✅ finished conversations remain readable in user history.
- ✅ new messages fail closed with `ai_conversation_closed`.
- ✅ close operation is user-scoped and audited without conversation text.
- ✅ Web marks finished chats and offers a direct **New conversation** action.
- ✅ published in `v0.1.122-dev` after successful Core/Web, Windows, Debian and release workflows.
- 🧪 live finish-chat acceptance on the installed Home-AI server remains pending.

### 0.1.123-dev — Controlled server tools + approval loop

- ✅ Ollama receives typed Home-AI tools and returns native tool calls.
- ✅ permitted read tools run automatically inside a bounded multi-round agent loop.
- ✅ change/sensitive tool calls become persistent per-user approval actions instead of executing immediately.
- ✅ current user permissions are re-checked when approval is submitted.
- ✅ Web chat renders action parameters, sensitivity, approve/reject and execution result.
- ✅ first controlled server domains: network profiles/link state and storage inspect/mount/unmount.
- ✅ privileged operations continue through the existing Home-AI helper; no generic shell/root tool exists.
- ✅ action inputs/results are size-bounded and audit does not store raw tool input.
- ✅ published in `v0.1.123-dev` after successful Core/Web, Windows, Debian and release workflows.
- 🧪 live tool-capable local-model acceptance remains pending.

### 0.1.124-dev — Public web access + finish-chat visibility

- ✅ `web.search` exposes bounded public web search to the local model through the existing Tool Registry.
- ✅ `web.fetch` reads bounded public HTTP/HTTPS text while blocking localhost, private/LAN/link-local/CGNAT and non-public targets.
- ✅ DNS is checked at dial time, redirects are bounded, explicit ports are limited to 80/443, and response/model-context size is capped.
- ✅ web content is explicitly treated as untrusted data in the model system prompt.
- ✅ web tools are read-only and inherit the current user's `system.read` permission.
- ✅ **Finish chat / Завершить чат** remains visibly discoverable in the AI chat header; the existing close API/state contract is unchanged.
- ✅ regression tests cover tool discovery/API counts, SSRF boundary and DuckDuckGo result decoding.
- ✅ published in `v0.1.124-dev` after successful PR/main Core/Web, Windows, Debian and release workflows.
- 🧪 live acceptance still required on the installed server: current-info question → `web.search` → optional `web.fetch` → grounded answer.
### 0.1.130-dev — Production Web transport fallback

- ✅ production Web AI chat uses the stable JSON message endpoint after live `Failed to fetch` on the 0.1.129 browser stream.
- ✅ streaming provider/Core endpoint remains implemented for isolated compatibility work and is no longer on the critical user path.
- ✅ Ollama fast-profile tuning from 0.1.129 remains active.
- ✅ server-side provider failures now include request/correlation IDs in logs without chat content.
- ✅ hotfix published in `v0.1.130-dev` after successful PR/main CI and release workflows.
- 🧪 streaming must return only after a dedicated live transport compatibility pass.
### 0.1.129-dev — Streaming + local inference performance

- ✅ Ollama streaming provider path decodes incremental `/api/chat` responses while preserving typed tool calls.
- ✅ Core exposes a CSRF-protected NDJSON chat stream and Web renders assistant deltas immediately.
- ✅ final chat persistence still happens only after successful generation, so streaming does not reintroduce orphan/duplicate history.
- ✅ local Ollama defaults are tuned for interactive use: `keep_alive=30m`, `think:false`, `num_ctx=16384`.
- ✅ Core HTTP write timeout is longer than the AI chat deadline, preventing transport timeout from racing valid generation.
- ✅ non-streaming provider contract remains supported as a fallback.
- ✅ published in `v0.1.129-dev` after successful PR/main Core/Web, Windows, Debian and release workflows.
- 🧪 live acceptance required on the installed GPU node.
### Next slice — Broader controlled server tools

- updater/check/install tools;
- safe service/runtime controls for allow-listed Home-AI dependencies;
- NAS/SMB configuration tools through domain APIs;
- richer post-action model follow-up;
- provider health/reconnect diagnostics.


### Later slice — Approval and controlled actions

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
