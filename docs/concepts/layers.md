# Bordo Layers

Bordo organizes platform operations into three layers. Each layer is surfaced as a
sidebar section in the web console and as a group of CLI commands.

## Build

**Goal**: Take source code and produce a deployable OCI image artifact.

Steps:
1. **Scaffold** — expand a golden-path template into a new project skeleton.
2. **Containerize** — run a container build (BuildKit / Kaniko) in the cluster.
3. **Push** — push the resulting OCI image to the configured registry.
4. **SBOM** — generate a software bill of materials and attach it to the image.

Triggered by: `bordo build trigger <project>` or chat: `build my-service`.

## Release

**Goal**: Take a build artifact and deploy it to one or more regions.

Steps:
1. **Desired state** — write the new version to the GitOps state repository.
2. **Reconcile** — the release reconciler detects the change and applies it to the
   target regional cluster(s).
3. **Progressive delivery** — traffic shifts gradually (canary) or all-at-once with
   an old-version standby (blue-green). Automated analysis can promote or rollback.
4. **Multi-region** — regions can be updated sequentially or in parallel; rollout
   policy is per-project.

Triggered by: `bordo deploy <project> <version> --region us-east-1` or chat.

## Observe

**Goal**: Understand what is happening across projects and regions.

Data sources bundled with Bordo:
- **Metrics** — VictoriaMetrics (Prometheus-compatible scrape + push)
- **Logs** — Loki (push via Alloy / Promtail)
- **Traces** — Tempo (OpenTelemetry OTLP)

The observe facade API provides a single query endpoint that the agent and console
call. Apps using the Bordo SDK are pre-instrumented; new apps get an OTel sidecar.

Accessed via: `bordo metrics query ...`, `bordo logs query ...`, or chat.

---

All three layers are managed by the control plane and exposed via the same gRPC/REST
API. The AI agent calls them through MCP tools. The console renders the results as
rich cards rather than static dashboards.
