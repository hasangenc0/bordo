# observe/

The Bordo Observe layer — unified telemetry query facade.

## What it will do (EP-06)

- Deploy the observability stack (VictoriaMetrics + Loki + Tempo + OTel Collector)
  to each regional cluster when a region is added (BRD-030)
- Expose a unified query API that the agent and console call (BRD-031)
- Route metrics queries to VictoriaMetrics, log queries to Loki, trace queries to Tempo
- Support cross-region aggregation (query all regions, merge results)

## Planned issues

- **BRD-030** — Observability bundle (OTel + VictoriaMetrics + Loki + Tempo)
- **BRD-031** — Unified observe query facade API

## Stack

| Signal | Backend | Protocol |
|---|---|---|
| Metrics | VictoriaMetrics | PromQL (remote_read) |
| Logs | Loki | LogQL |
| Traces | Tempo | TraceQL / OTLP |
| Collection | OTel Collector (DaemonSet) | OTLP gRPC + HTTP |

Apps built with Bordo templates are pre-wired for OTel. New apps get an OTel sidecar injected.

## Dependencies

Depends on EP-03 (k3s clusters to deploy the stack into).
