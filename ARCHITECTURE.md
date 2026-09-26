# Bordo Architecture

> For per-decision rationale see the ADRs in `docs/architecture/`.

## Big picture

Bordo is a self-hostable Internal Developer Platform (IDP) driven by an AI agent.
Users interact primarily through a chat interface; the agent translates intent into
platform operations via MCP tools.

```
┌──────────────────────────────────────────────┐
│   Web Console  (React / TS / Node BFF)        │
│   chat-first · sidebar:                       │
│   Projects · Build · Release · Observe ·      │
│   Metrics                                     │
└──────────────────────┬───────────────────────┘
                       │ REST/gRPC + WebSocket
┌──────────────────────▼───────────────────────┐
│   AI Agent  (Go / TS)                         │
│   chat sessions · MCP tool routing ·          │
│   pluggable model provider (Claude default)   │
└──────────────────────┬───────────────────────┘
                       │ gRPC
┌──────────────────────▼───────────────────────────────────────────┐
│   CONTROL PLANE  (Go  ·  bordod)                                  │
│                                                                    │
│   Project Registry  ·  Fleet Manager  ·  Build Orchestrator       │
│   Release Orchestrator  ·  Observe Facade                         │
│   API server (gRPC + grpc-gateway REST)                           │
│   State: SQLite (embedded)  →  Postgres (HA)                      │
└────┬──────────────┬─────────────────┬──────────────────┬─────────┘
     │              │                 │                  │
┌────▼───┐   ┌──────▼─────┐   ┌──────▼──────┐   ┌──────▼──────┐
│  CLI   │   │  Fleet /   │   │   Build     │   │  Release +  │
│ bordo  │   │ Substrate  │   │   layer     │   │  Observe    │
└────────┘   │ (k3s boot) │   │ (templates, │   │  layers     │
             └──────┬─────┘   │  BuildKit)  │   └──────┬──────┘
                    │         └─────────────┘          │
     ssh + install k3s / docker                        │
                    │          GitOps apply · OTel/metrics
┌───────────────────▼────────────────────────────────────▼──────────┐
│   FLEET                                                            │
│   region us-east-1  ·  region eu-west-1  ·  region ap-south-1 …  │
│   each = a k3s cluster on 1..N VMs                                │
│   running:                                                         │
│     Java web services / workers / Kafka consumers / jobs           │
│     TypeScript / React FE apps, Node BFFs                         │
└────────────────────────────────────────────────────────────────────┘
```

## Components

### Control Plane (`bordod`)

Single self-hostable binary. Responsibilities:
- Project registry & catalog
- Fleet state (regions, clusters, VM membership)
- Build orchestration (delegates to Build layer)
- Release orchestration (delegates to Release layer)
- Observe facade (query proxy to telemetry backends)
- gRPC + REST API (used by CLI, agent, console)
- Embedded state store (SQLite → Postgres for HA)

### CLI (`bordo`)

Go binary using Cobra. Full feature parity with the chat interface — every agent tool
call has a CLI equivalent as an escape hatch. Used for bootstrap, CI, and automation.

### Fleet / Substrate

Bootstrap agent: SSH to a VM → install Docker + k3s → join the regional cluster →
register with the control plane. A fleet controller watches cluster membership and
syncs desired workloads.

Multi-region = multiple k3s clusters, each registered as a named region. The control
plane API provides a single view across all regions.

### Build Layer

1. Template engine: expand a golden-path template into a project skeleton.
2. Container builder: run BuildKit or Kaniko in the cluster to produce an OCI image.
3. Registry: push to a configured OCI registry (local or remote).
4. SBOM: generate a software bill of materials for every image.

### Release Layer

GitOps: desired state lives in a git repository. The release orchestrator reconciles
cluster state to match desired state. Progressive delivery: traffic shifting with
automatic canary analysis and promotion, or blue-green with manual promote.

### Observe Layer

Bundled telemetry stack:
- **Metrics**: VictoriaMetrics (Prometheus-compatible)
- **Logs**: Loki
- **Traces**: Tempo
- **Instrumentation SDK**: OpenTelemetry for app runtimes

The observe facade API lets the agent and console query all three signals with one call.

### AI Agent

Go process (or TypeScript) exposing:
- A WebSocket chat endpoint consumed by the console
- An MCP tool server exposing all platform operations

Model provider is pluggable; default is Claude. The agent never holds business logic —
it routes tool calls to the control plane API.

### Web Console

React + TypeScript frontend + Node BFF. Chat-first: the main surface is a chat window.
The sidebar (Projects / Build / Release / Observe / Metrics) provides contextual
panels that render as rich cards from tool results, not pre-built dashboards.

### App Frameworks (Java + TS)

Opinionated runtime libraries for the apps Bordo manages:
- **Java web service**: Spring Boot + OpenAPI
- **Java worker**: background processing with retries/DLQ
- **Java Kafka**: consumer/producer abstractions
- **Java job**: scheduled tasks with distributed locking
- **Java data layer**: JDBC pool + migrations (any RDBMS)
- **React/TS FE**: Vite + React, pre-wired for Bordo metrics/auth
- **Node BFF**: Express/Fastify BFF template, pre-wired for Bordo

## Layers and their MVP order

```
EP-00: Foundation (done)
  ↓
EP-01: Control plane core    ← current focus
  ↓
EP-02: CLI (bordo)           ← current focus
  ↓
EP-04: Build layer           ← current focus
  ↓ (proves: new java service → scaffold → build → artifact)
EP-03: Fleet / k3s substrate
  ↓
EP-05: Release layer
EP-06: Observe layer
EP-07: AI agent + MCP
EP-08: Web console
EP-09: Java app framework
EP-10: TS/Node app framework
EP-11: Security / multi-tenancy
EP-12: Docs / examples
```

## ADRs

- [ADR-0001](docs/architecture/ADR-0001-record-architecture-decisions.md) — Record architecture decisions
- [ADR-0002](docs/architecture/ADR-0002-monorepo-and-go-workspace.md) — Monorepo + Go workspace
- [ADR-0003](docs/architecture/ADR-0003-control-plane-single-binary.md) — Control plane as a single binary
- [ADR-0004](docs/architecture/ADR-0004-k3s-any-vm-substrate.md) — k3s on any VM as the substrate
- [ADR-0005](docs/architecture/ADR-0005-agent-first-ux-and-mcp.md) — Agent-first UX with MCP
