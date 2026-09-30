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

🚧 **Active development.** Core platform is working — control plane, AI agent, multi-region fleet, build pipeline, and web console are all functional. Contributions welcome.

## Quickstart

**Running a server?** The `bordo` CLI drives the whole platform (Docker under the hood):
see [`docs/guides/running-with-the-cli.md`](docs/guides/running-with-the-cli.md).

**Building from source?** Full guide: [`docs/guides/getting-started.md`](docs/guides/getting-started.md)

```bash
# 1. Clone and build
git clone https://github.com/hasangenc0/bordo.git
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

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) for how to contribute.

## License

Apache 2.0 — see [`LICENSE`](LICENSE).
