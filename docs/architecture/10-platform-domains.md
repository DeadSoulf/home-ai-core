# Platform Domains

Home-AI-Core separates platform control from workload implementation.

## Control plane

Core owns:

- identity;
- authentication;
- authorization;
- module registry;
- jobs;
- durable events;
- audit;
- updates;
- host health/capability discovery;
- storage/network foundations.

## Workload plane

The workload plane is implemented exclusively by external Docker modules.

Each workload owns its domain logic and persistent state and communicates with Core through documented interfaces.

## Architectural rule

A workload may request Core capabilities, permissions and privileged operations, but it cannot bypass Core security boundaries. Product-specific data and dependencies stay outside the Core binary and Core SQLite.
