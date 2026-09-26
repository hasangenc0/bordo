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

🚧 **Early development** — foundation in place, Build layer in progress.

See [`tracker/BOARD.md`](tracker/BOARD.md) for the live kanban board and [`tracker/EPICS.md`](tracker/EPICS.md) for the roadmap.

## Getting started

> Detailed guide in [`docs/guides/getting-started.md`](docs/guides/getting-started.md) (coming soon).

```bash
# 1. Build the CLI
cd cli && go build -o bordo . && mv bordo /usr/local/bin/

# 2. Start the control plane
cd control-plane && go build -o bordod . && ./bordod serve

# 3. Bootstrap a region (SSH to a VM)
bordo region add --name us-east-1 --host 1.2.3.4 --key ~/.ssh/id_ed25519

# 4. Create your first project
bordo project create my-api --template java-web-service
```

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) and the [`tracker/README.md`](tracker/README.md) for how we manage work.

## License

Apache 2.0 — see [`LICENSE`](LICENSE).
