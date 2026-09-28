# Running Bordo with Docker Compose

The quickest way to run the full Bordo platform on any machine with Docker.

## Prerequisites

- Docker 24+ with Compose v2 (`docker compose version`)
- Git
- A Cloudflare AI Gateway token (`CF_API_TOKEN`) for the AI agent

## Rancher Desktop

Bordo works with [Rancher Desktop](https://rancherdesktop.io/) instead of Docker Desktop.

**Setup:**
1. Install Rancher Desktop
2. Enable **dockerd (moby)** in Preferences → Container Engine (for full `docker compose` support)
   — or use `nerdctl compose` if you prefer containerd
3. Make sure `docker` and `docker compose` are on your PATH

**Build note:** For local builds without a registry, images are loaded into the local daemon
(`--load`). Set `BORDO_REGISTRY` to push to a registry instead.

## Quick start

```bash
# 1. Clone
git clone https://github.com/bordo-io/bordo && cd bordo

# 2. Configure
cp deploy/bordo/.env.example deploy/bordo/.env
# Edit deploy/bordo/.env — set CF_API_TOKEN and BORDO_SECRET_KEY

# 3. Build and start
cd deploy/bordo
docker compose up --build
```

Open the console at **http://localhost:3000**.

## Services

| Service | Port | Description |
|---|---|---|
| `bordod` | 7401 | Control plane REST API |
| `bordo-agent` | 7402 | AI agent + WebSocket chat |
| Console | 3000 | Web UI (React + Node BFF) |

## With observability stack

Adds VictoriaMetrics, Loki, Tempo, and OTel Collector:

```bash
docker compose -f docker-compose.yml -f docker-compose.observability.yml up --build
```

Ports: VictoriaMetrics `:8428`, Loki `:3100`, Tempo `:3200`, OTel OTLP HTTP `:4318`.

## Using the CLI

```bash
# Build the CLI locally
make build   # or: go build -o bin/bordo ./cli/

# Point at the running stack
./bin/bordo config set server http://localhost:7401
./bin/bordo config set token <api-token>

# Use it
./bin/bordo project list
./bin/bordo project create my-service --template java-web-service
```

## Data persistence

SQLite database stored in the `bordod-data` Docker volume.

```bash
# Reset all data
docker compose down -v

# Backup
docker run --rm -v bordo_bordod-data:/data alpine tar czf - /data > bordo-backup.tar.gz
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `CF_API_TOKEN` | — | **Required.** Cloudflare AI Gateway token for the agent. |
| `BORDO_LLM_URL` | CF gateway URL | Override the LLM endpoint (OpenAI-compatible). |
| `BORDO_LLM_MODEL` | `@cf/deepseek-ai/deepseek-v4.1-flash` | Override the LLM model. |
| `BORDO_SECRET_KEY` | `changeme...` | 32+ char key for AES-256-GCM secrets encryption. |
| `BORDO_GITOPS_REPO` | _(empty)_ | URL of desired-state git repo for GitOps reconciliation. |
| `BORDO_REGISTRY` | _(empty)_ | Container registry for built images. Empty = local daemon load. |
| `BORDOD_PORT` | `7401` | Override the control-plane port. |
| `AGENT_PORT` | `7402` | Override the agent port. |
| `CONSOLE_PORT` | `3000` | Override the console port. |

## Adding a region

Once bordod is running, bootstrap a VM with k3s:

```bash
./bin/bordo-fleet region add \
  --name us-east \
  --host 1.2.3.4 \
  --user ubuntu \
  --key ~/.ssh/id_rsa
```

## Healthchecks

```bash
curl http://localhost:7401/healthz   # bordod
curl http://localhost:7401/version   # version info
curl http://localhost:3000/health    # console
```

## Logs

```bash
docker compose logs -f bordod
docker compose logs -f bordo-agent
docker compose logs -f bordo-console
```
