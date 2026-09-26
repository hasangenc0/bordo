# Bordo

**An open-source, self-hostable software factory for solo builders and small teams.**

Enterprises have internal developer platforms — golden paths, CI/CD, GitOps, multi-region fleets, observability. Solo builders don't, because building one is itself a full-time engineering effort.

Bordo is that platform. You run one binary, point it at VMs in any region, and drive the whole thing through an **AI chat interface** instead of a wall of dashboards.

---

## What you can build and run with Bordo

- **Java backends**: web services, workers, Kafka consumers/producers, background/scheduled jobs
- **Java data layer**: a type-safe database layer connecting to any RDBMS
- **Frontend**: React + TypeScript apps and Node BFFs

## How it works

Three operational layers — each reachable via a sidebar in the console chat:

| Layer | What it does |
|---|---|
| **Build** | Scaffold from golden-path templates → containerize → produce OCI artifacts |
| **Release** | GitOps deploy across regions with progressive rollout (canary / blue-green) |
| **Observe** | Unified metrics, logs, and traces behind a single query API |

The AI agent drives each layer through MCP tools. You talk; it acts. Cards in the console render from tool results — not from pre-built dashboards.

## Multi-region, any VM

Bordo bootstraps lightweight Kubernetes (k3s) onto raw VMs over SSH. One cluster per region. The control plane manages the whole fleet.

```
any VM (ssh) ──► bordo installs k3s
  region us-east-1:  cluster
  region eu-west-1:  cluster
  region ap-south-1: cluster
  ...
fleet controller spans all regions
```

## Tech stack

| Level | Technologies |
|---|---|
| Platform | Go · k3s / Kubernetes · Docker / OCI · gRPC |
| App runtimes | Java · TypeScript / Node / React |
| State | SQLite (single node) → Postgres (HA) |
| Observability | OpenTelemetry · VictoriaMetrics · Loki · Tempo |

---

## Project status

🚧 **Active development — Build layer MVP complete.**

**Done (14 issues):**
- BRD-000 Foundation & repo scaffolding
- BRD-001 Control-plane REST API server (chi, port 7401)
- BRD-002 Project registry & catalog (SQLite)
- BRD-003 Config, structured logging, health/version endpoints
- BRD-004 CLI — config, login, project, build commands
- BRD-005 Template engine + golden-path template contract
- BRD-006 `java-web-service` golden-path template (Spring Boot 3.3, OTel, actuator)
- BRD-007 Containerized build (docker buildx) → OCI image + push to registry
- BRD-008 Build MVP demo: project create → scaffold → docker build → artifact
- BRD-010 k3s bootstrap agent over SSH
- BRD-011 Region + cluster registration & fleet state
- BRD-040 Agent chat loop + MCP tool registry (WebSocket + MCP endpoints)
- BRD-080 RBAC + multi-tenancy model
- BRD-091 `make board` generator for `tracker/BOARD.md`

See [`tracker/BOARD.md`](tracker/BOARD.md) for the live kanban board and [`tracker/EPICS.md`](tracker/EPICS.md) for the full roadmap.

## Quickstart

Full guide: [`docs/guides/getting-started.md`](docs/guides/getting-started.md)

```bash
# 1. Clone and build
git clone https://github.com/bordo-io/bordo.git
cd bordo
make build
export PATH="$PWD/bin:$PATH"

# 2. Start the control plane
bordod serve

# 3. Configure the CLI (in a second terminal)
bordo config set server http://localhost:7401

# 4. Create a project from a template
bordo project create hello-service --template java-web-service

# 5. Scaffold the template locally
bordo template expand java-web-service \
  --var "ProjectName=hello-service" \
  --var "GroupId=com.example" \
  --output /tmp/hello-service

# 6. Trigger a container build
bordo build trigger hello-service --registry localhost:5000 --tag v0.1.0
```

Run the end-to-end demo:

```bash
bash examples/java-web-service-demo/demo.sh
```

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) and the [`tracker/README.md`](tracker/README.md) for how we manage work.

## License

Apache 2.0 — see [`LICENSE`](LICENSE).
