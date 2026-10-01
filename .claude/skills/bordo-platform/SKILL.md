---
name: bordo-platform
description: Operate the Bordo self-hosted platform via the bordo CLI — install, start/stop, check status, read logs, complete first-run setup, upgrade, and troubleshoot. Use when a user asks to deploy, run, configure, restart, or debug a Bordo installation on a server.
---

# Operating the Bordo platform

Bordo is a self-hostable software factory. The `bordo` CLI is the single control
surface for the whole stack; Docker is the runtime underneath. You never run raw
`docker compose` commands — always drive the platform through `bordo platform`.

## Services

Three containers, defined in an embedded compose file written to `~/.bordo/deploy`:

- **bordod** — control-plane API (port 7401), SQLite state in the `bordod-data` volume
- **bordo-agent** — AI agent (port 7402), chat history in the `agent-data` volume
- **bordo-console** — web UI (port 3000), proxies to the agent

`~/.bordo/deploy/.env` holds `BORDO_INTERNAL_TOKEN` (shared secret) and the port
values `BORDOD_PORT`, `AGENT_PORT`, `CONSOLE_PORT`.

## Standard lifecycle

```bash
bordo platform install    # write compose + .env, pull images (needs Docker present)
bordo platform up         # start all services
bordo setup   # first-run config; defaults to localhost:7401 on the server
bordo platform status     # verify health
```

## First-run setup

bordod prints a one-time **setup token** to its logs on first start. Setup also
requires a **DeepSeek API key** (the agent's LLM). Steps:

1. `bordo platform token` — prints the current setup token
2. `bordo setup` — supply the setup token + DeepSeek key (add --server <url> if not on the server)
3. Save the returned **admin token** — it is shown only once
4. `bordo login` with the admin token, then `bordo whoami` to confirm

## Command reference

| Command | Purpose |
|---|---|
| `bordo platform install [--force]` | Write config + pull images. `--force` regenerates `.env` (rotates the token). |
| `bordo platform up` | Start all services in the background. |
| `bordo platform down` | Stop/remove containers; **keeps** data volumes. |
| `bordo platform down --volumes` | Also delete data volumes — **destructive, confirm first**. |
| `bordo platform restart [service]` | Restart all, or one named service. |
| `bordo platform status` | Show service health. |
| `bordo platform token` | Print the one-time setup token (needed for `bordo setup`). |
| `bordo platform logs [-f] [service]` | Show/stream logs, optionally for one service. |
| `bordo platform upgrade` | Pull latest images + restart; **data preserved**. |

## Safety rules

- **Never** run `bordo platform down --volumes` unless the user explicitly asks to
  delete all data — it destroys the control-plane database and chat history.
- Keep services private. Recommend an SSH tunnel
  (`ssh -L 3000:localhost:3000 -L 7401:localhost:7401 root@SERVER`) instead of
  exposing ports 3000/7401/7402 to the public internet.
- To change ports or the token: edit `~/.bordo/deploy/.env`, then `bordo platform up`.
- The `BORDO_INTERNAL_TOKEN` is a secret — never print it, commit it, or send it
  to any external service.

## Troubleshooting

- **`platform not installed`** → run `bordo platform install` first.
- **`Docker is not installed`** → install Docker Engine + Compose plugin; the CLI
  will not install system packages itself.
- **A service won't start / restarts** → `bordo platform logs <service>` to read the
  error. bordod not being configured yet is normal until `bordo setup` is run.
- **Deploying an app fails** → the control plane is up, but running user workloads
  needs a k3s cluster bootstrapped onto a VM first (fleet layer). That is a
  separate step from `bordo platform`.

## Mental model

`bordo platform` manages **Bordo itself** (control plane, agent, console) as Docker
containers. Bordo in turn bootstraps and manages **k3s clusters** on your VMs, where
your deployed applications run. Bordo orchestrates Kubernetes; it does not run on it.
