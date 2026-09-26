# ADR-0003: Control Plane as a Single Self-hostable Binary

**Status:** Accepted  
**Date:** 2026-09-26  
**Deciders:** Project founders

## Context

The control plane hosts the project registry, fleet state machine, build orchestrator,
release orchestrator, and the observe query facade. We need to decide whether to split
these into separate services or ship them as one binary.

The primary target is a solo builder running Bordo on a single VM before they have a
fleet. Adding operational complexity (multiple processes, service discovery, inter-service
auth) would undermine the "single person can set this up in an afternoon" goal.

## Decision

`bordod` is a single Go binary that embeds all control-plane concerns. Internally, the
code is organized into packages with clean interfaces, so extraction into microservices
is possible later. State is an embedded SQLite database that can be swapped for Postgres
by setting a config flag.

```
bordod
├── server/           gRPC + REST gateway, auth middleware
├── registry/         project catalog, metadata
├── fleet/            region/cluster/VM state machine
├── build/            build job orchestrator
├── release/          release pipeline orchestrator
├── observe/          query facade (proxies to telemetry backends)
└── store/            database layer (SQLite / Postgres via same interface)
```

## Alternatives considered

- **Microservices from day one** — maximally scalable but requires a service mesh,
  distributed tracing from the start, and makes local dev significantly harder.
- **Separate binaries, same repo** — middle ground, but adds deploy complexity with
  no benefit at the current scale.

## Consequences

- A solo builder can run `./bordod serve` and have a fully functional control plane.
- The single binary simplifies upgrades (one version to track).
- Internal package interfaces must be kept clean to enable future extraction.
- SQLite is sufficient for single-node and small teams. The Postgres path (via
  config: `store.driver: postgres`) is planned in EP-11 for HA deployments.
- State migration is the responsibility of `bordod migrate` (planned in BRD-003).
