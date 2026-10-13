# Home-AI Product Scope

## Mission

Home-AI is a local-first extensible server platform built around one trusted control plane.

## Core scope

Home-AI-Core provides:

- identity, authentication and users;
- permissions/policy;
- audit;
- jobs/events/realtime;
- module registry and lifecycle orchestration;
- host inventory;
- storage/network foundations;
- Core updates;
- Web UI shell.

## Module scope

All functional product domains are external modules. The active Core repository must not contain their implementation, domain state schema or runtime dependencies.

External modules may expose UI and API contributions through versioned contracts.

## Runtime

Docker is the standard runtime for independently installable modules. Generic arbitrary container administration is not the product goal; Core manages only registered Home-AI modules.

## Local-first rule

Essential Core administration remains local and does not require a cloud subscription.

## Non-goals for Core

- embedding product services in the Core binary;
- becoming a generic container-management panel;
- becoming a generic virtualization platform;
- giving modules unrestricted host/root access;
- giving the network-facing Core direct Docker socket access;
- storing external-module domain databases inside Core SQLite.
