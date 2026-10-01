# ADR-0036: AI Agent as a First-Party Module with Typed Tool Contracts

- Status: Accepted
- Date: 2026-10-01

## Context

Home-AI is now ready to begin the AI Agent before Smart Home and NVR are fully implemented so the agent and future domain APIs can evolve together.

Putting model orchestration directly into Core would make the trusted control plane depend on AI-provider behavior and would encourage domain-specific logic to leak into Core. Waiting until every product module is finished would create the opposite problem: the agent would have to adapt to APIs that were designed without a common AI/tool boundary.

## Decision

### Module boundary

The primary agent is a first-party module with stable ID `ai.agent`.

A separate `ai.runtime` capability may later provide local inference/runtime services. The agent may consume that capability or an explicitly configured external provider, but Core does not depend on a model vendor.

### Core remains authoritative

Core remains authoritative for:

- identity and sessions;
- effective permissions/resource scopes;
- jobs/events;
- audit;
- module/capability discovery;
- package/update trust.

The AI Agent does not inherit owner/admin authority and does not receive unrestricted host privileges.

### Typed tool registry

AI-accessible actions are exposed as typed tools. A tool descriptor includes:

- stable module/tool ID;
- input schema;
- required permissions;
- optional resource scope;
- side-effect sensitivity;
- approval requirement;
- bounded execution contract.

Every tool execution re-evaluates the requesting user's current effective permissions.

Domain modules own their tools. The AI module does not reach around module APIs to manipulate their state directly.

### Side-effect classes

Tools use three initial classes:

- `read`;
- `change`;
- `sensitive`.

Read tools may execute automatically for an authorized user. Change tools require approval by default. Sensitive tools always require explicit approval unless a future owner-defined policy contract explicitly allows a narrower behavior without weakening permission checks.

### No arbitrary shell tool

The first-party AI Agent has no generic shell/root tool.

If a future diagnostic or maintenance action is needed, it must be represented by a bounded tool with explicit validation, permissions and audit.

### Provider contract

Model providers are adapters behind a stable interface. Local inference is preferred. External providers remain optional and require explicit configuration plus outbound-context policy.

Provider credentials and raw secrets are never written to audit logs.

## Consequences

### Positive

- AI can start now and shape future domain contracts.
- Core remains small and trusted.
- Smart Home/NAS/NVR can expose one consistent tool model.
- permissions and approval semantics are reusable from the beginning.
- local and external models remain replaceable.

### Negative

- an additional tool-contract API must be designed and tested before rich AI features;
- early AI capability will initially be read-heavy and intentionally limited;
- some approval concepts may later need extraction into a more generic Core policy subsystem.

## Follow-up

The implementation plan is maintained in [../AI_AGENT_FOUNDATION.md](../AI_AGENT_FOUNDATION.md).
