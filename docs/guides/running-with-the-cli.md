# Running Bordo with the CLI

The `bordo` CLI is the single control surface for the whole platform. Docker is
the runtime under the hood, but you never type a `docker compose` command. This
guide takes you from a fresh server to a running, configured Bordo.

---

## Prerequisites

- **Docker 24+** with the Compose plugin — `docker compose version` should print `v2.x`.
  The CLI checks for Docker and links the install docs if it's missing; it does
  **not** install Docker for you (that touches system packages).
- The `bordo` binary (see [Install the CLI](#install-the-cli)).

---

## Install the CLI

```bash
go install github.com/hasangenc0/bordo/cli/bordo@latest
```

This installs a binary named `bordo` into `$(go env GOPATH)/bin`. Make sure that
directory is on your `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
bordo version
```

Building from source instead:

```bash
git clone https://github.com/hasangenc0/bordo.git
cd bordo
make build            # produces ./bin/bordo
```

---

## Bring up the platform

```bash
bordo platform install    # write config to ~/.bordo/deploy, pull images
bordo platform up         # start bordod + agent + console
```

`install` writes two files to `~/.bordo/deploy`:

- `docker-compose.yml` — embedded in the binary, no git clone needed
- `.env` — generated with a random internal token and default ports

All tunables live in `.env`. Edit it and re-run `bordo platform up` to apply:

```dotenv
BORDO_INTERNAL_TOKEN=<generated>   # shared secret between services — keep private
BORDOD_PORT=7401                   # control-plane API
AGENT_PORT=7402                    # AI agent
CONSOLE_PORT=3000                  # web console
```

---

## Complete first-run setup

On first start bordod prints a one-time **setup token**. Retrieve it, then run the
setup wizard (you'll also need a DeepSeek API key for the agent's LLM):

```bash
bordo platform logs bordod | grep setup_token
bordo setup --server http://localhost:7401
```

Setup returns your **admin token** — save it, it is shown only once.

---

## Access the console

Keep the services private (admin-only). From your laptop, tunnel over SSH rather
than exposing ports publicly:

```bash
ssh -L 3000:localhost:3000 -L 7401:localhost:7401 root@<SERVER_IP>
```

Then open **http://localhost:3000** — the chat-first web console.

Point the CLI at the same tunnel:

```bash
bordo config set server http://localhost:7401
bordo login          # paste your admin token
bordo whoami
```

---

## Day-to-day operations

| Command | What it does |
|---|---|
| `bordo platform status` | Show service health (`docker compose ps`) |
| `bordo platform logs -f` | Stream logs; add a service name to filter (e.g. `logs -f bordod`) |
| `bordo platform restart [service]` | Restart all services, or one |
| `bordo platform upgrade` | Pull the latest images and restart — **data is preserved** |
| `bordo platform down` | Stop and remove containers; volumes are kept |
| `bordo platform down --volumes` | Stop **and delete all data** — destructive |

---

## Upgrades

```bash
bordo platform upgrade
```

Pulls the newest images from GHCR and restarts services in place. Data volumes
(`bordod-data`, `agent-data`) are never touched.

---

## Where things live

| Path | Purpose |
|---|---|
| `~/.bordo/deploy/docker-compose.yml` | Service definitions (rewritten by `install`) |
| `~/.bordo/deploy/.env` | Ports + internal token (yours to edit) |
| `bordod-data` volume | Control-plane SQLite database |
| `agent-data` volume | Agent chat history |

To change ports or rotate the internal token, edit `.env` (or
`bordo platform install --force` to regenerate it) and run `bordo platform up`.
